import { describe, expect, test } from "vitest";
import type { LogEvent, TaskDetail, Turn } from "@/api/types";
import { ActivityItem, activityOf, inTurn, sections, shown } from "./activity";

// A row as activityOf makes it, with only what sections reads filled in.
function row(key: string, kind: ActivityItem["kind"], by: string, turn = ""): ActivityItem {
  return { key, kind, at: "", live: false, turn, by, title: key, tone: "default" };
}

const band = (key: string, turn: string, agent: string) => row(key, "turn", agent, turn);

// What sections gives, as text: each section's band, then its rows.
function outline(items: ActivityItem[], show: Parameters<typeof sections>[1] = "all"): string[] {
  return sections(items, show).map(
    (section) =>
      `${section.by}${section.turn ? ` ${section.turn.turn}` : ""}${section.continued ? " continued" : ""}: ${section.rows.map((r) => r.key).join(" ")}`,
  );
}

describe("sections", () => {
  test("puts each turn under its agent's band, newest first, its rows newest first", () => {
    const items = [
      band("1", "turn-1", "coordinator"),
      row("2", "note", "Agentry", "turn-1"),
      row("3", "ended", "Agentry", "turn-1"),
      band("4", "turn-2", "engineer"),
      row("5", "action", "engineer", "turn-2"),
      row("6", "action", "engineer", "turn-2"),
    ];
    expect(outline(items)).toEqual(["engineer turn-2: 6 5", "coordinator turn-1: 3 2"]);
    expect(sections(items)[0].key).toBe("4");
    expect(sections(items)[0].turn).toBe(items[3]);
  });

  test("groups rows between turns by who wrote them: you, Agentry, another team's coordinator", () => {
    const items = [
      row("1", "message", "you"),
      row("2", "note", "you"),
      row("3", "note", "Agentry"),
      row("4", "note", "Agentry"),
      row("5", "note", "designer-lead"),
      row("6", "message", "you"),
    ];
    expect(outline(items)).toEqual(["you: 6", "designer-lead: 5", "Agentry: 4 3", "you: 2 1"]);
  });

  test("keeps the coordinator's handoff after its turn in its turn's section, under the band", () => {
    const items = [
      band("1", "turn-1", "coordinator"),
      row("2", "ended", "Agentry", "turn-1"),
      row("3", "message", "coordinator"),
      row("4", "note", "Agentry"),
      band("5", "turn-2", "engineer"),
    ];
    expect(outline(items)).toEqual(["engineer turn-2: ", "Agentry: 4", "coordinator turn-1: 3 2"]);
  });

  test("splits a running turn at your message: the later rows go under a continued band", () => {
    const items = [
      band("1", "turn-6", "reviewer"),
      row("2", "note", "Agentry", "turn-6"),
      row("3", "message", "you"),
      row("4", "action", "reviewer", "turn-6"),
      row("5", "action", "reviewer", "turn-6"),
    ];
    expect(outline(items)).toEqual(["reviewer turn-6 continued: 5 4", "you: 3", "reviewer turn-6: 2"]);
    const [continued] = sections(items);
    expect(continued.turn).toBe(items[0]);
    expect(continued.key).toBe("4");
  });

  test("narrowed to the turn, the turn is one section again", () => {
    const items = [
      band("1", "turn-6", "reviewer"),
      row("2", "note", "Agentry", "turn-6"),
      row("3", "message", "you"),
      row("4", "action", "reviewer", "turn-6"),
    ];
    expect(outline(inTurn(items, "turn-6"))).toEqual(["reviewer turn-6: 4 2"]);
  });

  describe("with a filter", () => {
    const items = [
      row("1", "message", "you"),
      band("2", "turn-1", "coordinator"),
      row("3", "note", "Agentry", "turn-1"),
      row("4", "note", "Agentry"),
      band("5", "turn-2", "engineer"),
      row("6", "action", "engineer", "turn-2"),
      row("7", "message", "you"),
      row("8", "action", "engineer", "turn-2"),
    ];

    test("drops a section the filter empties, with its band, and joins up what was on either side", () => {
      expect(outline(shown(items, "messages"), "messages")).toEqual(["you: 7 1"]);
    });

    test("joins rows by a turn's agent to its turn once a filter drops the turn between", () => {
      const handoffs = [
        band("1", "turn-1", "coordinator"),
        row("2", "message", "coordinator"),
        band("3", "turn-2", "engineer"),
        row("4", "message", "coordinator"),
      ];
      expect(outline(handoffs, "messages")).toEqual(["coordinator turn-1: 4 2"]);
      expect(outline(handoffs)).toEqual(["coordinator: 4", "engineer turn-2: ", "coordinator turn-1: 2"]);
    });

    test("never drops a turn's band in Actions", () => {
      expect(outline(shown(items, "actions"), "actions")).toEqual(["engineer turn-2: 8 6", "coordinator turn-1: "]);
    });

    test("keeps a turn with no rows yet in Everything", () => {
      expect(outline([band("1", "turn-1", "engineer")])).toEqual(["engineer turn-1: "]);
    });
  });

  test("gives nothing for no rows", () => {
    expect(sections([])).toEqual([]);
  });
});

describe("activityOf's authors", () => {
  const turn = { id: "turn-1", agent: "engineer", worker: "local", ended: true, issue: "", tokens: 0 } as Turn;
  const event = (position: number, type: string, actor: string, data: Record<string, unknown> = {}, correlation?: string): LogEvent => ({
    position,
    at: "2026-10-19T06:00:00Z",
    type,
    actor,
    data,
    correlation,
  });
  const task = {
    coordinator: "coordinator",
    turnList: [turn],
    actions: [],
    events: [
      event(1, "task.created", "human:you", { title: "Make it readable" }),
      event(2, "task.assigned", "agent:coordinator", { agent: "engineer", instructions: "Build it" }),
      event(3, "worktree.ready", "worker:local", { branch: "agentry/task-1" }),
      event(4, "turn.started", "worker:local", { agent: "engineer" }, "turn-1"),
      event(5, "prompt.acknowledged", "worker:local", {}, "turn-1"),
      event(6, "task.needs_you", "system:coordinator-loop", { question: "Raise the budget?" }),
    ],
  } as unknown as TaskDetail;

  test("names you, the agent, and Agentry for the loop's and the workers' own rows; a turn is its agent's", () => {
    expect(activityOf(task).map((item) => item.by)).toEqual(["you", "coordinator", "Agentry", "engineer", "Agentry", "Agentry"]);
  });

  test("says a message from the loop is from Agentry", () => {
    expect(activityOf(task).at(-1)!.from).toBe("Agentry");
  });
});

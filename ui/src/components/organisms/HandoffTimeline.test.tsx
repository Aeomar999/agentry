import { describe, expect, test, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import type { Step, Turn } from "@/api/types";
import { clockMinutes } from "@/lib/time";
import { HandoffTimeline } from "./HandoffTimeline";

// EntryText stands in for itself here, so the test reads what each card
// asks of it: how many lines its text folds at, and as what.
const entries: { text: string; lines: number; kind: string }[] = [];
vi.mock("./EntryText", () => ({
  EntryText: (props: { text: string; lines: number; kind: string }) => {
    entries.push(props);
    return <div>{props.text}</div>;
  },
}));

const at = (minute: number) => `2026-10-19T06:${String(minute).padStart(2, "0")}:00Z`;

function turn(id: string, agent: string, fields: Partial<Turn> = {}): Turn {
  return { id, agent, worker: "local", startedAt: at(0), lastEventAt: at(48), ended: true, issue: "", tokens: 0, ...fields } as Turn;
}

const TURNS = [
  turn("turn-4", "engineer", { tokens: 210_000 }),
  turn("turn-5", "coordinator"),
  turn("turn-6", "reviewer", { ended: false }),
];

// Oldest first, as the task has them.
const STEPS: Step[] = [
  { at: at(1), from: "you", to: "coordinator", text: "Make **Handoffs** readable.", turn: "" },
  { at: at(2), from: "coordinator", to: "engineer", text: "# Build the cards\n\nEach card is an article.", turn: "turn-4" },
  { at: at(50), from: "engineer", to: "coordinator", text: "Done: the cards are articles.", turn: "turn-5" },
  { at: at(51), from: "coordinator", to: "reviewer", text: "", turn: "turn-6" },
];

function timeline(onOpen = vi.fn(), steps = STEPS) {
  entries.length = 0;
  render(
    <HandoffTimeline
      steps={steps}
      turns={TURNS}
      coordinator="coordinator"
      links={{}}
      actionsIn={(id) => (id === "turn-4" ? 14 : 1)}
      working=""
      onOpen={onOpen}
      onWatch={() => {}}
    />,
  );
  return onOpen;
}

const card = (label: string) =>
  screen.getByRole("article", { name: `${label}, ${clockMinutes(STEPS.find((s) => label === `${s.from} to ${s.to}`)!.at)}` });

describe("HandoffTimeline", () => {
  test("draws each step as an article named by who passed it to whom, and when", () => {
    timeline();
    const names = screen.getAllByRole("article").map((article) => article.getAttribute("aria-label"));
    expect(names).toEqual([...STEPS].reverse().map((step) => `${step.from} to ${step.to}, ${clockMinutes(step.at)}`));
  });

  test("makes the footer the card's only button, and no button wraps the card", () => {
    timeline();
    for (const article of screen.getAllByRole("article")) {
      expect(article.closest("button")).toBeNull();
      expect(within(article).queryAllByRole("button").length).toBeLessThanOrEqual(1);
    }
    expect(within(card("you to coordinator")).queryByRole("button")).toBeNull(); // no turn, nothing to open
  });

  test("opens the turn a footer names", () => {
    const onOpen = timeline();
    fireEvent.click(within(card("coordinator to engineer")).getByRole("button", { name: "Led to turn-4, engineer. Open its activity" }));
    expect(onOpen).toHaveBeenCalledWith("turn-4");
  });

  test("says Led to with the facts of an ended turn", () => {
    timeline();
    const footer = within(card("coordinator to engineer")).getByRole("button");
    expect(footer.textContent).toBe("Led to turn-4·engineer·48m 00s·14 actions·210k tokensActivity");
  });

  test("puts the running badge in a running turn's footer, and in its name", () => {
    timeline();
    const footer = within(card("coordinator to reviewer")).getByRole("button", {
      name: "Led to turn-6, reviewer, running. Open its activity",
    });
    expect(footer.textContent).toContain("reviewer·running·1 action");
    expect(footer.textContent).not.toContain("48m");
  });

  test("folds a brief at 8 lines, and reports and your messages at 10", () => {
    timeline();
    const entry = (text: string) => entries.find((e) => e.text === text);
    expect(entry("Each card is an article.")).toMatchObject({ lines: 8, kind: "brief" });
    expect(entry("Done: the cards are articles.")).toMatchObject({ lines: 10, kind: "report" });
    expect(entry("Make **Handoffs** readable.")).toMatchObject({ lines: 10, kind: "message" });
  });

  test("takes a brief's opening heading out as the card's title", () => {
    timeline();
    const article = card("coordinator to engineer");
    expect(within(article).getByRole("heading", { level: 3 }).textContent).toBe("Build the cards");
    expect(entries.some((e) => e.text.includes("# Build"))).toBe(false);
  });

  test("draws a title's inline Markdown, not its markers", () => {
    const brief = { ...STEPS[1], text: "# Fix `foo` in **bar** and [baz](https://example.com)\n\nThe rest." };
    timeline(vi.fn(), [brief]);
    const title = screen.getByRole("heading", { level: 3 });
    expect(title.textContent).toBe("Fix foo in bar and baz (opens in a new tab)");
    expect(within(title).getByText("foo").tagName).toBe("CODE");
    expect(within(title).getByText("bar").tagName).toBe("STRONG");
    expect(within(title).getByRole("link", { name: "baz (opens in a new tab)" }).getAttribute("href")).toBe("https://example.com/");
  });

  test("draws a card with no brief as its header and footer only", () => {
    timeline();
    expect(entries).toHaveLength(3);
    expect(within(card("coordinator to reviewer")).queryByRole("heading")).toBeNull();
  });

  test("names both ends in text, with the arrow said as 'to'", () => {
    timeline();
    expect(card("engineer to coordinator").textContent).toContain("engineer to ");
  });
});

// A task's activity for the tests of the feed, as activityOf makes it,
// oldest first: a coordinator's turn and its handoff, Agentry's rows, the
// engineer's turn on another machine with a report, and the reviewer's
// running turn with your message in the middle of it.

import type { Action, Agent, Turn } from "@/api/types";
import type { ActivityItem } from "@/domains/tasks/activity";

export const AGENTS = [
  { name: "coordinator", coordinates: true, column: "", team: "" },
  { name: "engineer", coordinates: false, column: "", team: "" },
  { name: "reviewer", coordinates: false, column: "review", team: "" },
  { name: "product-lead", coordinates: true, column: "", team: "Design" },
] as Agent[];

const at = (minute: number) => `2026-10-19T06:${String(minute).padStart(2, "0")}:00Z`;

function turn(key: string, id: string, agent: string, fields: Partial<Turn> = {}): ActivityItem {
  const info = { id, agent, worker: "local", ended: true, issue: "", tokens: 0, stopped: false, ...fields } as Turn;
  return {
    key,
    kind: "turn",
    at: at(Number(key)),
    until: info.ended ? at(Number(key) + 5) : undefined,
    live: !info.ended,
    turn: id,
    by: agent,
    title: agent,
    tone: "info",
    turnInfo: info,
  };
}

function row(key: string, kind: ActivityItem["kind"], by: string, fields: Partial<ActivityItem> = {}): ActivityItem {
  return { key, kind, at: at(Number(key)), live: false, turn: "", by, title: "", tone: "default", ...fields };
}

function bash(key: string, turnId: string, by: string, command: string): ActivityItem {
  const action = {
    id: `a${key}`,
    turn: turnId,
    agent: by,
    tool: "Bash",
    command,
    decision: "approved",
    finished: true,
    reason: "",
  } as Action;
  return row(key, "action", by, { turn: turnId, title: "Bash", body: command, action, until: at(Number(key) + 1) });
}

export const ITEMS: ActivityItem[] = [
  row("1", "message", "you", { from: "you", to: "coordinator", body: "Make **Activity** readable." }),
  turn("2", "turn-1", "coordinator", { tokens: 8000 }),
  row("3", "ended", "Agentry", { turn: "turn-1", title: "Finished", tone: "success" }),
  row("4", "message", "coordinator", { from: "coordinator", to: "engineer on studio", body: "Build the bands.", leadsTo: "turn-2" }),
  row("5", "note", "Agentry", { title: "Worktree ready on studio: agentry/task-40" }),
  turn("6", "turn-2", "engineer", { worker: "studio", tokens: 41000 }),
  row("7", "note", "Agentry", { turn: "turn-2", title: "Split into stories", body: "one\ntwo" }),
  bash("8", "turn-2", "engineer", "npm test"),
  row("9", "ended", "Agentry", { turn: "turn-2", title: "Finished, 41k tokens", body: "## Done\n\nAll **green**.", tone: "success" }),
  row("10", "note", "product-lead", { title: "task-41 is done, from product-lead" }),
  turn("11", "turn-3", "reviewer", { ended: false }),
  row("12", "note", "Agentry", { turn: "turn-3", title: "Read its brief" }),
  row("13", "message", "you", { from: "you", to: "reviewer", body: "Check it at 390 too." }),
  bash("14", "turn-3", "reviewer", "go test ./..."),
];

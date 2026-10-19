import { onMachine } from "./machines";
import type { Action, LogEvent, TaskDetail, Turn } from "@/api/types";
import { readPlan } from "./plan";
import { limitText } from "./time";
import { tokens } from "./tokens";

// A task's activity: its event log, or the newest part of it, read as rows
// a person follows, oldest first (sections groups them by author, newest
// first, for the page).
// An action's request, decision and result are one row, and each turn opens
// with a row of its own. The Events tab keeps the raw log.

export type ActivityKind = "turn" | "action" | "message" | "waiting" | "issue" | "ended" | "note";
export type ActivityTone = "default" | "success" | "warning" | "danger" | "info";

export interface ActivityItem {
  key: string;
  kind: ActivityKind;
  at: string;
  until?: string; // when it finished; none while it is still happening
  live: boolean; // still happening
  turn: string; // the turn it happened in, "" between turns
  by: string; // who wrote it: an agent, you, or Agentry; a turn's row is its agent's
  leadsTo?: string; // a handoff: the turn it started
  from?: string; // who said it, for a message
  to?: string;
  title: string;
  body?: string; // longer text: instructions, a report, a question
  tone: ActivityTone;
  action?: Action;
  turnInfo?: Turn;
}

// Kinds you can narrow the activity to; "all" is every row.
export const SHOWS = ["all", "actions", "messages"] as const;
export type Show = (typeof SHOWS)[number];

export function activityOf(task: TaskDetail, log: LogEvent[]): ActivityItem[] {
  const actions = new Map((task.actions ?? []).map((a) => [a.id, a]));
  const turns = new Map((task.turnList ?? []).map((t) => [t.id, t]));

  // When each action was settled: its result, or its denial.
  const settledAt = new Map<string, string>();
  for (const e of log) {
    if ((e.type === "action.result" || e.type === "action.denied") && e.correlation) settledAt.set(e.correlation, e.at);
  }

  const items: ActivityItem[] = [];
  let handoff: ActivityItem | undefined; // the last handoff to an agent, until its turn starts
  for (const e of log) {
    const turn = turnOfEvent(e, turns, actions);
    const item = rowOf(e, turn, turns, actions, settledAt, task.coordinator);
    if (!item) continue;
    if (item.kind === "message" && item.to && item.to !== "you") handoff = item;
    if (item.kind === "turn" && handoff && handoff.to === item.turnInfo?.agent) {
      handoff.leadsTo = item.turn;
      handoff = undefined;
    }
    items.push(item);
  }
  return items;
}

// Only the rows of one turn, with the handoff that started it.
export function inTurn(items: ActivityItem[], turn: string): ActivityItem[] {
  return items.filter((item) => item.turn === turn || item.leadsTo === turn);
}

export function shown(items: ActivityItem[], show: Show): ActivityItem[] {
  if (show === "actions") return items.filter((item) => item.kind === "action" || item.kind === "turn");
  if (show === "messages") return items.filter((item) => item.kind !== "action" && item.kind !== "note");
  return items;
}

// rowFor is the row an action, or an event, shows in: the action's own row
// for every event about it. "" when it has none, as for a new priority.
export function rowFor(items: ActivityItem[], target: { action?: string; event?: LogEvent }): string {
  const { action, event } = target;
  const exact = event && items.find((item) => item.key === String(event.position));
  if (exact) return exact.key;
  const id = action ?? event?.correlation;
  return (id && items.find((item) => item.action?.id === id)?.key) || "";
}

// A Section is a run of rows with one author, under one band: a turn (its
// agent's), or rows between turns by you, by Agentry or by another agent.
export interface Section {
  key: string; // its oldest row's, which stays as rows are added
  by: string;
  turn?: ActivityItem; // the turn's own row, for a turn's section
  continued: boolean; // the turn's later rows, after someone else's in the middle of it
  rows: ActivityItem[]; // newest first, the turn's own row left out
}

// sections groups rows, oldest first as activityOf gives them, into author
// sections, newest first. A new turn or a new author starts a section; rows
// between turns by the agent of the turn just before them stay in its
// section (the coordinator's handoff after its turn). Your message in the
// middle of a turn splits it: the turn's later rows go under a continued
// band. A turn left with no rows by the Messages filter goes, and what was
// on either side of it joins up if it has one author; in Actions and
// Everything a turn's band stays, as the turn happened.
export function sections(items: ActivityItem[], show: Show = "all"): Section[] {
  const kept: Section[] = [];
  for (const section of grouped(items)) {
    const last = kept[kept.length - 1];
    if (!section.rows.length && show === "messages") continue;
    if (!section.turn && last?.by === section.by) last.rows.push(...section.rows);
    else kept.push(section);
  }
  for (const section of kept) section.rows.reverse();
  return kept.reverse();
}

// grouped is sections' first pass: every section, oldest first, its rows too.
function grouped(items: ActivityItem[]): Section[] {
  const out: Section[] = [];
  const turns = new Map<string, ActivityItem>();
  for (const item of items) {
    const last = out[out.length - 1];
    if (item.kind === "turn") {
      turns.set(item.turn, item);
      out.push({ key: item.key, by: item.by, turn: item, continued: false, rows: [] });
    } else if (item.turn ? last?.turn?.turn === item.turn : last?.by === item.by) {
      last.rows.push(item);
    } else {
      const turn = item.turn ? turns.get(item.turn) : undefined;
      out.push({ key: item.key, by: turn?.by ?? item.by, turn, continued: Boolean(turn), rows: [item] });
    }
  }
  return out;
}

function turnOfEvent(e: LogEvent, turns: Map<string, Turn>, actions: Map<string, Action>): string {
  if (!e.correlation) return "";
  if (turns.has(e.correlation)) return e.correlation;
  return actions.get(e.correlation)?.turn ?? "";
}

// AGENTRY is the name Agentry's own rows go by: the loop's, the workers'
// and the policy's.
export const AGENTRY = "Agentry";

// who is the name an event's actor goes by in the trail: "agent:dev" is
// dev, a person is you, and anyone else is Agentry.
function who(e: LogEvent): string {
  const [kind, id] = e.actor.split(":", 2);
  return kind === "human" ? "you" : kind === "agent" ? (id ?? kind) : AGENTRY;
}

function text(e: LogEvent, key: string): string {
  const value = e.data?.[key];
  return typeof value === "string" ? value : "";
}

function rowOf(
  e: LogEvent,
  turn: string,
  turns: Map<string, Turn>,
  actions: Map<string, Action>,
  settledAt: Map<string, string>,
  coordinator: string,
): ActivityItem | null {
  const base = { key: String(e.position), at: e.at, turn, by: who(e), live: false, tone: "default" as ActivityTone };
  const message = (from: string, to: string, body: string, tone: ActivityTone = "default"): ActivityItem => ({
    ...base,
    kind: "message",
    from,
    to,
    title: `${from} to ${to}`,
    body,
    tone,
  });

  switch (e.type) {
    case "task.created":
      return message("you", coordinator, text(e, "description") || text(e, "title"));
    case "task.directed":
    case "task.reopened":
      return message("you", coordinator, text(e, "text"));
    case "task.answered":
      return message("you", coordinator, text(e, "answer"));
    case "task.assigned":
      return message(who(e), onMachine(text(e, "agent"), text(e, "machine")), text(e, "instructions"));
    case "task.needs_you":
      return message(who(e), "you", text(e, "question"), "warning");
    case "task.done":
      return message(who(e), "you", text(e, "summary"), "success");

    case "turn.started": {
      const info = turns.get(turn);
      if (!info) return null;
      const ended = info.ended ? info.lastEventAt : undefined;
      return {
        ...base,
        kind: "turn",
        by: info.agent,
        title: info.agent,
        turnInfo: info,
        until: ended,
        live: !info.ended && !info.limited,
        tone: info.issue ? "danger" : info.limited && !info.ended ? "warning" : "info",
      };
    }
    case "turn.ended": {
      const used = Number(e.data?.tokens ?? 0);
      const title = used > 0 ? `Finished, ${tokens(used)} tokens` : "Finished";
      return { ...base, kind: "ended", title, body: text(e, "report"), tone: "success" };
    }
    case "artifact.published":
      return {
        ...base,
        kind: "note",
        title: `Published the artifact "${text(e, "title")}" (${e.correlation})${e.data?.wait ? ", waiting for you" : ""}`,
      };
    case "artifact.commented":
      return { ...base, kind: "note", title: `You commented on ${e.correlation}: ${text(e, "text")}` };
    case "artifact.replied":
      return {
        ...base,
        kind: "note",
        title: `${who(e)} replied on ${e.correlation} ${text(e, "comment")}${e.data?.resolved ? ", resolved" : ""}`,
      };
    case "artifact.acted":
      return {
        ...base,
        kind: "note",
        title: text(e, "action") ? `You chose "${text(e, "action")}" on ${e.correlation}` : `You sent your comments on ${e.correlation}`,
      };
    case "turn.limited":
      return {
        ...base,
        kind: "waiting",
        title: "Waiting for the usage limit",
        body: limitText(text(e, "resetsAt"), e.at),
        tone: "warning",
      };
    case "turn.resumed":
      return {
        ...base,
        kind: "note",
        title: text(e, "reason") === "reset" ? "Went on after the usage limit reset" : "Went on after the usage limit",
      };
    case "turn.stopped":
      return { ...base, kind: "note", title: "You stopped this turn, so the coordinator reads what you said" };
    case "issue.raised":
      return { ...base, kind: "issue", title: "Something went wrong", body: text(e, "reason"), tone: "danger" };
    case "agent.waiting":
      return { ...base, kind: "waiting", title: "Waiting", body: text(e, "question"), tone: "warning" };

    case "action.requested": {
      const action = e.correlation ? actions.get(e.correlation) : undefined;
      if (!action) return null;
      const until = settledAt.get(action.id);
      const waiting = action.asked && !action.decision;
      return {
        ...base,
        kind: "action",
        title: action.tool,
        body: action.command,
        action,
        until,
        live: !until,
        tone: action.decision === "denied" ? "danger" : waiting ? "warning" : "default",
      };
    }
    case "action.asked":
    case "action.approved":
    case "action.denied":
    case "action.result":
    case "task.ranked":
      return null; // folded into the action's row; a new priority is not news

    case "plan.updated": {
      const plan = readPlan(text(e, "plan"));
      return {
        ...base,
        kind: "note",
        title: plan.steps ? `Updated its plan: ${plan.done} of ${plan.steps} steps done` : "Cleared its plan",
      };
    }
    case "prompt.acknowledged":
      return { ...base, kind: "note", title: "Read its brief" };
    case "session.started":
      return { ...base, kind: "note", title: `Engine session ${text(e, "engineSession").slice(0, 8)}` };
    case "task.renamed":
      return { ...base, kind: "note", title: `Renamed to "${text(e, "title")}"` };
    case "task.labeled": {
      const labels = Array.isArray(e.data?.labels) ? (e.data.labels as string[]) : [];
      return { ...base, kind: "note", title: labels.length ? `Labeled ${labels.join(", ")}` : "Labels removed" };
    }
    case "task.budgeted": {
      const budget = Number(e.data?.budget ?? 0);
      return { ...base, kind: "note", title: budget > 0 ? `Budget set to ${tokens(budget)} tokens` : "Budget set to unlimited" };
    }
    case "task.turns_limited": {
      const maxTurns = Number(e.data?.maxTurns ?? 0);
      return { ...base, kind: "note", title: maxTurns > 0 ? `Max turns set to ${maxTurns}` : "Max turns set to unlimited" };
    }
    case "task.held": {
      const place = text(e, "place");
      return { ...base, kind: "note", title: place ? `Held in ${place}` : "Moved on" };
    }
    case "task.transferred":
      return {
        ...base,
        kind: "note",
        title: `Passed to the ${text(e, "team")} team: ${text(e, "coordinator")} coordinates it now`,
        body: text(e, "reason"),
      };
    case "task.requested":
      return { ...base, kind: "note", title: `Asked the ${text(e, "team")} team, as ${text(e, "child")}`, body: text(e, "request") };
    case "task.split":
      return { ...base, kind: "note", title: "Split into stories", body: text(e, "titles").split("; ").join("\n") };
    case "task.epic_waits":
      return { ...base, kind: "note", title: "Waiting for its stories" };
    case "task.story_done":
      return { ...base, kind: "note", title: `${text(e, "story")} is done, from ${text(e, "coordinator")}`, body: text(e, "summary") };
    case "task.request_answered":
      return { ...base, kind: "note", title: `${text(e, "child")} is done, from ${text(e, "coordinator")}`, body: text(e, "summary") };
    case "file.added":
      return { ...base, kind: "note", title: `Shared ${text(e, "name")}${text(e, "by") === "you" ? "" : `, left by ${text(e, "by")}`}` };
    case "file.removed":
      return { ...base, kind: "note", title: `Stopped sharing ${text(e, "name")}` };
    case "file.fetching":
      return { ...base, kind: "note", title: `Fetching ${text(e, "machine")}:${text(e, "path")} into the shared files` };
    case "files.fetched":
      return { ...base, kind: "note", title: `Copied ${text(e, "from")} into the shared files` };
    case "files.collected":
    case "files.delivered":
      return null; // the folder kept in step: what changed shows as file.added and file.removed
    case "task.repo_changed":
      return { ...base, kind: "note", title: `You moved the task to the repo at ${text(e, "repo")}` };
    case "worktree.ready":
      return { ...base, kind: "note", title: `Worktree ready on ${text(e, "worker") || "local"}: ${text(e, "branch")}` };
    case "worktree.moved":
      return { ...base, kind: "note", title: `Work moved to ${text(e, "worker")}` };
    default:
      return { ...base, kind: "note", title: e.type };
  }
}

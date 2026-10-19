import type { LogEvent, TaskDetail } from "@/api/types";
import { onMachine } from "./machines";
import { limitText } from "./time";

// Your chat with a task's coordinator, read from the task's events, oldest
// first: what you said, what it said back, its questions and its summary,
// with the work in between (its handoffs, the agents' reports, a turn you
// stopped) as quieter lines, so you can see what your words led to.

export type ChatKind = "you" | "reply" | "question" | "done" | "handoff" | "report" | "note";

export interface ChatItem {
  key: string;
  at: string;
  kind: ChatKind;
  who: string; // who said it: "you", the coordinator, or an agent
  to?: string; // a handoff's agent
  text: string;
  tone?: "warning" | "danger";
  viaClaude?: boolean; // you asked Claude, in the Workspace, to send it
  viaArtifact?: boolean; // you acted on an artifact, when Agentry had them
}

// The types of event chatOf reads, so the chat reads only those: a type
// chatOf reads must be here too, or a test fails.
export const CHAT_TYPES = [
  "task.created",
  "task.transferred",
  "task.requested",
  "task.split",
  "task.epic_waits",
  "file.fetching",
  "files.fetched",
  "task.story_done",
  "task.request_answered",
  "task.directed",
  "task.reopened",
  "task.answered",
  "task.assigned",
  "task.needs_you",
  "task.done",
  "turn.ended",
  "turn.stopped",
  "issue.raised",
] as const;

export function chatOf(task: TaskDetail, events: LogEvent[]): ChatItem[] {
  const items: ChatItem[] = [];
  const agentOf = new Map((task.turnList ?? []).map((t) => [t.id, t.agent]));
  for (const e of events) {
    const key = String(e.position);
    const said = (kind: ChatKind, text: string, extra: Partial<ChatItem> = {}) =>
      text && items.push({ key: `${key}-${kind}`, at: e.at, kind, who: task.coordinator, text, ...extra });
    const reply = () => said("reply", text(e, "toYou"));
    switch (e.type) {
      case "task.created":
        if (text(e, "parent"))
          said("report", text(e, "description"), { who: askedBy(e, task.coordinator) }); // another team asked
        else said("you", text(e, "description") || text(e, "title"), { who: "you" });
        break;
      case "task.transferred":
        reply();
        said("handoff", `This is the ${text(e, "team")} team's work: ${text(e, "reason")}`, {
          who: askedBy(e, task.coordinator),
          to: text(e, "coordinator"),
        });
        break;
      case "task.requested":
        reply();
        said("handoff", `${text(e, "child")}: ${text(e, "request")}`, {
          who: askedBy(e, task.coordinator),
          to: `the ${text(e, "team")} team`,
        });
        break;
      case "task.split":
        reply();
        said("handoff", text(e, "titles").split("; ").join("\n"), { who: askedBy(e, task.coordinator), to: "its stories" });
        break;
      case "task.epic_waits":
        reply();
        break;
      case "file.fetching":
        said("note", `Fetching ${text(e, "machine")}:${text(e, "path")} into the task's shared files.`, { who: "agentry" });
        break;
      case "files.fetched":
        said("note", `Copied ${text(e, "from")} into the task's shared files.`, { who: "agentry" });
        break;
      case "task.story_done":
        said("report", `${text(e, "story")} is done: ${text(e, "summary")}`, { who: text(e, "coordinator") });
        break;
      case "task.request_answered":
        said("report", `${text(e, "child")} is done: ${text(e, "summary")}`, { who: text(e, "coordinator") });
        break;
      case "task.directed":
      case "task.reopened":
        said("you", yourWords(e, "text"), {
          who: "you",
          viaClaude: text(e, "via") === "claude",
          viaArtifact: text(e, "via") === "artifact",
        });
        break;
      case "task.answered":
        said("you", yourWords(e, "answer"), {
          who: "you",
          viaClaude: text(e, "via") === "claude",
          viaArtifact: text(e, "via") === "artifact",
        });
        break;
      case "task.assigned":
        reply();
        said("handoff", text(e, "instructions"), { to: onMachine(text(e, "agent"), text(e, "machine")) });
        break;
      case "task.needs_you":
        reply();
        said("question", text(e, "question"), { who: askedBy(e, task.coordinator) });
        break;
      case "task.done":
        reply();
        said("done", text(e, "summary"));
        break;
      case "turn.ended": {
        const agent = agentOf.get(e.correlation ?? "") ?? "";
        if (agent && agent !== task.coordinator) said("report", text(e, "report") || "(no report)", { who: agent });
        break;
      }
      case "turn.limited": {
        const agent = agentOf.get(e.correlation ?? "") ?? "an agent";
        said("note", `${agent}'s ${e.correlation} hit a ${limitText(text(e, "resetsAt"), e.at)}`, { who: "agentry", tone: "warning" });
        break;
      }
      case "turn.resumed": {
        const agent = agentOf.get(e.correlation ?? "") ?? "an agent";
        said("note", `${agent}'s ${e.correlation} resumed after its usage limit`, { who: "agentry" });
        break;
      }
      case "turn.stopped": {
        const agent = agentOf.get(e.correlation ?? "") ?? "an agent";
        said("note", `You stopped ${agent}'s ${e.correlation} so the coordinator reads this now.`, { who: "you" });
        break;
      }
      case "issue.raised":
        said("note", text(e, "reason"), { who: "agentry", tone: "danger" });
        break;
    }
  }
  return items;
}

// askedBy is who asked you: the coordinator, or Agentry itself, as when a
// task reaches its budget or its limit of turns.
function askedBy(e: LogEvent, coordinator: string): string {
  return e.actor.startsWith("agent:") ? e.actor.slice("agent:".length) : e.actor.startsWith("system:") ? "agentry" : coordinator;
}

// yourWords is what you said; an artifact's message to the coordinator is
// cut to what you chose and wrote, with a count of the comments sent with it.
function yourWords(e: LogEvent, key: string): string {
  const words = text(e, key);
  if (text(e, "via") !== "artifact") return words;
  const [said, comments = ""] = words.split("\n\nMy comments on it:");
  const count = (comments.match(/\n- /g) ?? []).length;
  return count ? `${said}\n\n(${count} ${count === 1 ? "comment" : "comments"} sent with it)` : said;
}

function text(e: LogEvent, key: string): string {
  const value = e.data?.[key];
  return typeof value === "string" ? value.trim() : "";
}

// How the chat lays its items out: messages, a line for each new day, and
// the work between two messages folded into one entry. A message opens a
// run when someone else spoke before it, or a while has passed.

export type ChatEntry =
  | { type: "day"; key: string; label: string }
  | { type: "message"; key: string; item: ChatItem; opensRun: boolean }
  | { type: "work"; key: string; items: ChatItem[] };

const WORK: ChatKind[] = ["handoff", "report", "note"];
const RUN_GAP = 5 * 60_000;

export function threadOf(items: ChatItem[], now = new Date()): ChatEntry[] {
  const entries: ChatEntry[] = [];
  let day = "";
  let last: ChatItem | undefined; // the last message, not work
  for (const item of items) {
    const label = dayLabel(new Date(item.at), now);
    if (label !== day) {
      day = label;
      entries.push({ type: "day", key: `day-${item.key}`, label });
      last = undefined;
    }
    if (WORK.includes(item.kind)) {
      const previous = entries[entries.length - 1];
      if (previous?.type === "work") previous.items.push(item);
      else entries.push({ type: "work", key: `work-${item.key}`, items: [item] });
      continue;
    }
    const previous = entries[entries.length - 1];
    const opensRun = !last || last.who !== item.who || previous?.type !== "message" || Date.parse(item.at) - Date.parse(last.at) > RUN_GAP;
    entries.push({ type: "message", key: item.key, item, opensRun });
    last = item;
  }
  return entries;
}

function dayLabel(date: Date, now: Date): string {
  const days = Math.round((startOfDay(now) - startOfDay(date)) / 86_400_000);
  if (days === 0) return "Today";
  if (days === 1) return "Yesterday";
  return date.toLocaleDateString([], { weekday: "long", month: "short", day: "numeric" });
}

function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}

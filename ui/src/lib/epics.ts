import type { TaskSummary } from "@/api/types";

// Epics and their stories, as the list and the board show them. An epic is
// a goal split into stories; each story is a task of its own.

// DONE_DAYS is how long finished work stays on the board; the list keeps it all.
export const DONE_DAYS = 7;

const DAY = 24 * 60 * 60 * 1000;

// onBoard is whether a task belongs on the board: not finished long ago,
// and not a story of an epic that is done (the epic's card stands for it).
export function onBoard(task: TaskSummary, byId: Map<string, TaskSummary>, now = Date.now()): boolean {
  if (task.status !== "done") return true;
  if (now - new Date(task.lastActiveAt).getTime() > DONE_DAYS * DAY) return false;
  return byId.get(task.epic)?.status !== "done";
}

export interface Row {
  task: TaskSummary;
  depth: 0 | 1; // 1 for a story under its epic
}

// grouped puts each epic's stories just under it, in the order given (your priority order), and
// leaves out the stories of folded epics. A story whose epic isn't in the
// list stands on its own.
export function grouped(tasks: TaskSummary[], folded: (epic: string) => boolean): Row[] {
  const shown = new Set(tasks.map((task) => task.id));
  const rows: Row[] = [];
  for (const task of tasks) {
    if (task.epic && shown.has(task.epic)) continue;
    rows.push({ task, depth: 0 });
    if (task.kind === "epic" && !folded(task.id)) {
      for (const story of tasks) if (story.epic === task.id) rows.push({ task: story, depth: 1 });
    }
  }
  return rows;
}

// waitingLine says what a story or an epic is waiting on, or "".
export function waitingLine(task: TaskSummary): string {
  if (task.blockedBy.length) return `Starts when ${task.blockedBy.join(" and ")} ${task.blockedBy.length > 1 ? "are" : "is"} done`;
  if (task.kind === "epic" && task.stories && !task.currentAgent && task.status === "working")
    return `${task.storiesDone} of ${task.stories} stories done`;
  return "";
}

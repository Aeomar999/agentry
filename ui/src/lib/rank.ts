import type { TaskSummary } from "@/api/types";

// rankAt is the rank for a task moved to position at among tasks (counted
// without the task itself): halfway between its new neighbours, so only the
// moved task changes. null when it lands where it was.
export function rankAt(tasks: TaskSummary[], moving: TaskSummary, at: number): number | null {
  if (at === tasks.indexOf(moving)) return null;
  const others = tasks.filter((task) => task.id !== moving.id);
  const before = others[at - 1];
  const after = others[at];
  if (!before && !after) return null;
  return before && after ? (before.rank + after.rank) / 2 : after ? after.rank - 1000 : before.rank + 1000;
}

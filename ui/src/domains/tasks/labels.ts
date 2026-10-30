import type { TaskSummary } from "@/api/types";

// Every label in use, most used first: what a label field suggests.
export function knownLabels(tasks: TaskSummary[]): string[] {
  const counts = new Map<string, number>();
  for (const task of tasks) for (const label of task.labels) counts.set(label, (counts.get(label) ?? 0) + 1);
  return [...counts].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([label]) => label);
}

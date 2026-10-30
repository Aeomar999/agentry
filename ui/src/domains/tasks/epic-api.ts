import { api } from "@/api/client";

// fileUnder makes the task a story of the epic, out of any it was in; ""
// takes it out of its epic.
export function fileUnder(task: string, epic: string): Promise<unknown> {
  return api("PUT", `/api/tasks/${encodeURIComponent(task)}/epic`, { epic });
}

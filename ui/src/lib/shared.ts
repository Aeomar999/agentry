import { send } from "@/api/client";

// A task's shared files: what its agents, on any machine, and you hand each
// other that isn't in git. They are in .agentry/files of whichever worktree
// works on the task next.

export interface SharedFile {
  name: string; // a path in the folder, such as data/sales.csv
  size: number;
  by: string; // the agent whose turn left it, or "you"
  at: string;
  from?: string; // machine:path, when it was fetched from a machine
}

// share uploads a file to the task, as name (its own name if not given).
export async function share(task: string, file: File, name = file.name): Promise<void> {
  await send(`/api/tasks/${encodeURIComponent(task)}/shared?name=${encodeURIComponent(name)}`, { method: "POST", body: file });
}

// downloadURL is where a shared file downloads from; the page's sign-in
// cookie lets it in, so it can be a plain link.
export function downloadURL(task: string, name: string): string {
  return `/api/tasks/${encodeURIComponent(task)}/shared/file?name=${encodeURIComponent(name)}`;
}

export function sizeOf(bytes: number): string {
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GB`;
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`;
  if (bytes >= 1 << 10) return `${(bytes / (1 << 10)).toFixed(1)} KB`;
  return `${bytes} B`;
}

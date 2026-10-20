import { api } from "./client";
import type { Erasure, Redaction } from "./types";

// Erasing a task and taking a secret out of one (docs/design/log-growth.md
// section 5). Nothing undoes either, so the page asks before each; a
// refusal throws the server's reason as an ApiError.

const taskPath = (task: string) => `/api/tasks/${encodeURIComponent(task)}`;

export function eraseTask(task: string, why: string): Promise<Erasure> {
  return api("POST", `${taskPath(task)}/erase`, { why });
}

// previewRedaction is how many of the task's rows hold text: it writes
// nothing. A text no row holds, or one the server won't redact, such as a
// number in the JSON or a text too short, is refused with the reason.
export async function previewRedaction(task: string, text: string): Promise<number> {
  const preview = await api<{ rows: number }>("POST", `${taskPath(task)}/redact/preview`, { text });
  return preview.rows;
}

export function redact(task: string, text: string, why: string): Promise<Redaction> {
  return api("POST", `${taskPath(task)}/redact`, { text, why });
}

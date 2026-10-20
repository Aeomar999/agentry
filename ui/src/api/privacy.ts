import { api } from "./client";
import type { Erasure, LogPreview, LogRedaction, Redaction } from "./types";

// Erasing a task and taking a secret out of one, out of what the agents
// learned, or out of the whole log (docs/design/log-growth.md section 5). Nothing undoes either, so the page asks before each; a
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

// redact takes text out of the task's rows, refused unless as many rows
// hold it as the preview counted, which you confirmed.
export function redact(task: string, text: string, why: string, rows: number): Promise<Redaction> {
  return api("POST", `${taskPath(task)}/redact`, { text, why, rows });
}

// previewAgentsRedaction is previewRedaction for the agents' roster and
// what they learned.
export async function previewAgentsRedaction(text: string): Promise<number> {
  const preview = await api<{ rows: number }>("POST", "/api/agents/redact/preview", { text });
  return preview.rows;
}

// redactAgents is redact for the agents' roster and what they learned.
export function redactAgents(text: string, why: string, rows: number): Promise<Redaction> {
  return api("POST", "/api/agents/redact", { text, why, rows });
}

// previewLogRedaction lists each stream of the log that holds text, with
// its rows, or why it can't be redacted: it writes nothing.
export function previewLogRedaction(text: string): Promise<LogPreview> {
  return api("POST", "/api/redact/preview", { text });
}

// redactLog takes text out of each stream counted, each refused unless as
// many of its rows hold it as the preview counted; a stream that holds it
// but wasn't counted is left, as you didn't see its rows.
export function redactLog(text: string, why: string, counts: Record<string, number>): Promise<LogRedaction> {
  return api("POST", "/api/redact", { text, why, counts });
}

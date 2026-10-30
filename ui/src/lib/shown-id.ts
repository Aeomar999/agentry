// An epic shows as epic-N wherever the page names it, so you can tell it
// from a story or a task on its own. Only what you read changes: links,
// keys and the API keep task-N, as in internal/domain/tasks/shown.go.

const TASK = "task-";
const EPIC = "epic-";

// shownId is a task's id as you read it: epic-N for an epic, its own id otherwise.
export function shownId(id: string, kind?: string): string {
  return kind === "epic" ? shownEpic(id) : id;
}

// shownEpic is an epic's id as you read it, such as a story's epic: "" stays "".
export function shownEpic(id: string): string {
  return id.startsWith(TASK) ? EPIC + id.slice(TASK.length) : id;
}

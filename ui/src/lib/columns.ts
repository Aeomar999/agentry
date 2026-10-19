import type { Agent, BoardColumn, TaskSummary } from "@/api/types";

// The board's columns, as every page names them. The server says which
// column a task is in (columnOf in internal/web/views.go).
export const COLUMN_TITLES: Record<BoardColumn, string> = {
  backlog: "Backlog",
  todo: "To do",
  "in-progress": "In progress",
  review: "Review",
  shipping: "Shipping",
  done: "Done",
};

// stageOf names the stage a task's work is in when it is past building:
// Review or Shipping. "" otherwise, so it shows only where it says more
// than the task's status.
export function stageOf(task: TaskSummary): string {
  return task.column === "review" || task.column === "shipping" ? COLUMN_TITLES[task.column] : "";
}

// columnOfAgent is the board column an agent's work shows in, or "" for
// building, which is In progress.
export function columnOfAgent(agent: Agent): BoardColumn | "" {
  return agent.column === "review" ? "review" : agent.column === "ship" ? "shipping" : "";
}

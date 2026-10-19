import { describe, expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { BoardColumn, TaskSummary } from "@/api/types";
import { TaskBoard } from "./TaskBoard";

const task = (id: string, column: BoardColumn): TaskSummary => ({
  id,
  title: `Title of ${id}`,
  repo: "/src/app",
  status: "working",
  currentAgent: "",
  stage: "",
  column,
  canShip: true,
  pushed: false,
  doneByYou: false,
  turns: 1,
  needsYou: "",
  hasQuestion: false,
  labels: [],
  budget: 0,
  tokensUsed: 0,
  overBudget: false,
  maxTurns: 0,
  turnsSinceAnswer: 0,
  atTurnLimit: false,
  stoppedAt: "",
  rank: 1,
  parked: "",
  parent: "",
  waitingOn: "",
  kind: "",
  epic: "",
  madeFrom: "",
  after: [],
  blockedBy: [],
  queued: false,
  stories: 0,
  storiesDone: 0,
  closing: "",
  createdAt: new Date().toISOString(),
  lastActiveAt: new Date().toISOString(),
});

function board(columns: BoardColumn[], tasks: TaskSummary[] = []) {
  render(
    <MemoryRouter>
      <TaskBoard
        columns={columns}
        tasks={tasks}
        everyTask={tasks}
        agents={[]}
        showBacklog={false}
        onOpen={() => {}}
        onPickLabel={() => {}}
        onPickEpic={() => {}}
      />
    </MemoryRouter>,
  );
  return screen.getAllByRole("region").map((section) => section.getAttribute("aria-label"));
}

describe("TaskBoard", () => {
  test("shows the columns it is given, in order, Backlog only when asked", () => {
    expect(board(["backlog", "todo", "in-progress", "review", "shipping", "done"])).toEqual([
      "To do",
      "In progress",
      "Review",
      "Shipping",
      "Done",
    ]);
  });

  test("has no Shipping column when the board lists none", () => {
    expect(board(["backlog", "todo", "in-progress", "review", "done"], [task("task-1", "review")])).toEqual([
      "To do",
      "In progress",
      "Review",
      "Done",
    ]);
  });

  test("puts a task in Shipping under that column", () => {
    board(["todo", "in-progress", "review", "shipping", "done"], [task("task-1", "shipping")]);
    expect(screen.getByRole("region", { name: "Shipping" }).textContent).toContain("task-1");
  });
});

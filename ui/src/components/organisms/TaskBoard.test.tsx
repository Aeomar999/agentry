import { describe, expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { BoardColumn, TaskSummary } from "@/api/types";
import { task } from "@/api/tasks.fixtures";
import { TaskBoard } from "./TaskBoard";

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

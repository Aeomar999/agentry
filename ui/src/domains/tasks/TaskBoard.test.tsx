import { describe, expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { BoardColumn, TaskSummary } from "@/api/types";
import { task } from "@/api/tasks.fixtures";
import { TaskBoard } from "./TaskBoard";
import { TaskCard } from "@/components/molecules/TaskCard";

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

  test("names a story's epic as epic-N, and the story and a task on its own as task-N", () => {
    const epic: TaskSummary = { ...task("task-21", "in-progress"), kind: "epic", title: "Ship v2" };
    const story: TaskSummary = { ...task("task-22", "todo"), epic: "task-21" };
    board(["todo", "in-progress", "review", "done"], [epic, story, task("task-30", "todo")]);
    const todo = screen.getByRole("region", { name: "To do" });
    expect(todo.textContent).toContain("epic-21 · Ship v2");
    expect(todo.textContent).toContain("task-22");
    expect(todo.textContent).toContain("task-30");
    expect(todo.textContent).not.toContain("task-21");
  });
});

describe("TaskCard", () => {
  test("shows an epic's id as epic-N", () => {
    const epic: TaskSummary = { ...task("task-21", "in-progress"), kind: "epic" };
    const noop = () => {};
    render(
      <TaskCard
        task={epic}
        onPickLabel={noop}
        onPickEpic={noop}
        epicTitle=""
        coordinator=""
        onOpen={noop}
        draggable={false}
        dragging={false}
        onDragStart={noop}
        onDragEnd={noop}
      />,
    );
    expect(screen.getByText("epic-21")).toBeTruthy();
    expect(screen.getByRole("button").getAttribute("data-task")).toBe("task-21");
  });
});

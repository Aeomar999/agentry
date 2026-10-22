import { describe, expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { TaskSummary } from "@/api/types";
import { task } from "@/api/tasks.fixtures";
import { PlanPage } from "./PlanPage";

const tasks: TaskSummary[] = [];
vi.mock("@/live/use-resource", () => ({ useResource: () => ({ data: tasks, error: undefined }) }));

describe("PlanPage", () => {
  test("names an epic's column epic-N, and its stories and tasks of no epic task-N", () => {
    const epic: TaskSummary = { ...task("task-21", "backlog"), kind: "epic", title: "Ship v2" };
    tasks.splice(0, tasks.length, epic, { ...task("task-22", "backlog"), epic: "task-21" }, task("task-30", "backlog"));
    render(
      <MemoryRouter>
        <PlanPage />
      </MemoryRouter>,
    );
    expect(screen.getByRole("button", { name: "Open epic-21" })).toBeTruthy();
    expect(screen.getAllByText(/^(task|epic)-\d+$/).map((shown) => shown.textContent)).toEqual(["task-30", "epic-21", "task-22"]);
  });
});

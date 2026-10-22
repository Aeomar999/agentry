import { describe, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { FeaturesReply, TaskSummary } from "@/api/types";
import { task } from "@/api/tasks.fixtures";
import { TasksPage } from "./TasksPage";

// Each path answers with what replies holds, or is still loading.
const replies = new Map<string, unknown>();
vi.mock("@/live/use-resource", () => ({
  useResource: (path: string) => ({ data: replies.get(path), error: undefined, position: 0, reload: async () => {} }),
}));

const NO_SHIPPING: FeaturesReply = { features: [], board: { columns: ["backlog", "todo", "in-progress", "review", "done"] } };

function page(address: string, tasks: TaskSummary[], features?: FeaturesReply) {
  replies.clear();
  replies.set("/api/tasks", tasks);
  replies.set("/api/agents", []);
  if (features) replies.set("/api/features", features);
  render(
    <MemoryRouter initialEntries={[address]}>
      <TasksPage />
    </MemoryRouter>,
  );
}

const chip = (label: string) => screen.queryAllByRole("button").find((button) => button.firstChild?.textContent === label);

describe("TasksPage's filters", () => {
  test("fall back to All when ?status=shipping names a column the board hasn't", () => {
    page("/tasks?status=shipping", [task("task-1", "todo")], NO_SHIPPING);
    expect(chip("Shipping")).toBeUndefined();
    expect(chip("All")?.getAttribute("aria-pressed")).toBe("true");
  });

  test("keep Shipping while a task is in it, though the manifest left it out", () => {
    page("/tasks?status=shipping", [task("task-1", "shipping")], NO_SHIPPING);
    expect(chip("Shipping")?.getAttribute("aria-pressed")).toBe("true");
  });

  test("keep Shipping while a task is in it and the manifest loads", () => {
    page("/tasks?status=shipping", [task("task-1", "shipping")]);
    expect(chip("Shipping")?.getAttribute("aria-pressed")).toBe("true");
  });
});

describe("TasksPage's board", () => {
  test("shows a card in Shipping while the manifest loads", () => {
    page("/tasks?view=board", [task("task-1", "shipping")]);
    expect(screen.getByText("Title of task-1")).toBeTruthy();
  });
});

describe("TasksPage's ids", () => {
  const epic: TaskSummary = { ...task("task-21", "in-progress"), kind: "epic" };
  const story: TaskSummary = { ...task("task-22", "todo"), epic: "task-21" };
  const own = task("task-30", "todo");
  const ids = () => screen.getAllByRole("row").map((row) => row.querySelector("td")?.textContent ?? "");

  test("show an epic as epic-N in the list, and a story or a task on its own as task-N", () => {
    page("/tasks?view=list", [epic, story, own]);
    expect(ids().filter(Boolean)).toEqual(["epic-21", "task-22", "task-30"]);
  });

  test("find an epic by epic-N", () => {
    page("/tasks?view=list", [epic, story, own]);
    fireEvent.change(screen.getByPlaceholderText("Filter tasks"), { target: { value: "epic-21" } });
    expect(ids().filter(Boolean)).toEqual(["epic-21"]);
  });

  test("still find an epic by its task-N id", () => {
    page("/tasks?view=list", [epic, story, own]);
    fireEvent.change(screen.getByPlaceholderText("Filter tasks"), { target: { value: "task-21" } });
    expect(ids().filter(Boolean)).toEqual(["epic-21"]);
  });
});

import { describe, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import type { TaskSummary } from "@/api/types";
import { task } from "@/api/tasks.fixtures";
import { taskItem } from "@/shell/AdminShell";
import { CommandPalette } from "./CommandPalette";

// The shell's imports reach its install button, which reads matchMedia as
// it loads; jsdom has none.
vi.mock("@/lib/install", () => ({
  watchInstall: () => {},
  installState: () => "unavailable",
  useInstall: () => "unavailable",
  install: async () => {},
}));

const epic: TaskSummary = { ...task("task-21", "in-progress"), kind: "epic", title: "Ship v2" };
const own: TaskSummary = { ...task("task-30", "todo"), title: "Fix the login" };

function search(query: string) {
  Element.prototype.scrollIntoView = vi.fn(); // jsdom has none
  render(<CommandPalette open items={[epic, own].map((shown) => taskItem(shown, () => {}))} onClose={() => {}} />);
  fireEvent.change(screen.getByPlaceholderText("Search tasks, agents and commands"), { target: { value: query } });
  return screen.queryAllByRole("button").map((found) => found.textContent);
}

describe("CommandPalette's tasks", () => {
  test("show an epic as epic-N and a task on its own as task-N", () => {
    expect(search("")).toEqual(["Ship v2epic-21 · working", "Fix the logintask-30 · working"]);
  });

  test("find an epic by epic-N", () => {
    expect(search("epic-21")).toEqual(["Ship v2epic-21 · working"]);
  });

  test("still find an epic by task-N", () => {
    expect(search("task-21")).toEqual(["Ship v2epic-21 · working"]);
  });
});

import { describe, expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { TaskDetail } from "@/api/types";
import { task } from "@/api/tasks.fixtures";
import { TaskPage } from "./TaskPage";

// Each path answers with what replies holds, or is still loading.
const replies = new Map<string, unknown>();
vi.mock("@/live/use-resource", () => ({
  useResource: (path: string) => ({ data: replies.get(path), error: undefined, position: 0, reload: async () => {} }),
}));
// The page's imports reach the shell's install button, which reads
// matchMedia as it loads; jsdom has none.
vi.mock("@/lib/install", () => ({
  watchInstall: () => {},
  installState: () => "unavailable",
  useInstall: () => "unavailable",
  install: async () => {},
}));
vi.mock("@/live/use-events", () => ({
  useEvents: () => ({
    events: [],
    loading: false,
    error: undefined,
    earlier: false,
    loadingEarlier: false,
    loadEarlier: () => {},
    retry: () => {},
  }),
}));

const ARCHIVED_AT = "2026-09-01T10:00:00Z";

function detail(more: Partial<TaskDetail> = {}): TaskDetail {
  return {
    ...task("task-1", "done"),
    status: "done",
    description: "",
    firstTitle: "",
    options: [],
    page: "",
    coordinator: "tech-lead",
    teamChosen: true,
    epicTitle: "",
    storyList: [],
    shared: [],
    movingTo: "",
    summary: "Shipped it.",
    worktree: null,
    trail: null,
    turnList: null,
    tokensUnknown: "",
    approvals: null,
    actions: null,
    ...more,
  };
}

function page(shown: TaskDetail, tab = "coordinator") {
  replies.clear();
  replies.set("/api/tasks/task-1", shown);
  replies.set("/api/agents", []);
  replies.set("/api/tasks", []);
  render(
    <MemoryRouter initialEntries={[`/tasks/task-1?tab=${tab}`]}>
      <Routes>
        <Route path="/tasks/:taskId" element={<TaskPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

const button = (name: string) => screen.queryByRole("button", { name });

describe("TaskPage for an archived task", () => {
  test("marks it archived and offers nothing that writes", () => {
    page(detail({ archived: true, archivedAt: ARCHIVED_AT }));
    expect(screen.getByText(/^Archived /)).toBeTruthy();
    expect(button("Add labels")).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Message tech-lead" })).toBeNull();
    expect(screen.getByText("The task is archived: it takes no messages.")).toBeTruthy();
  });

  test("keeps its repo and epic as they are", () => {
    page(detail({ archived: true, archivedAt: ARCHIVED_AT }), "details");
    expect(button("Change")).toBeNull();
    expect(screen.queryByRole("combobox", { name: "Epic" })).toBeNull();
  });

  test("shares no files and takes none away", () => {
    page(detail({ archived: true, archivedAt: ARCHIVED_AT, shared: [{ name: "notes.md", size: 1, by: "you", at: ARCHIVED_AT }] }), "files");
    expect(button("Share files")).toBeNull();
    expect(screen.queryByTitle("Stop sharing it")).toBeNull();
  });

  // Only a done task is archived, and LimitEditor shows a done task's
  // limits without editing them.
  test("shows its tokens and turns, and changes neither", () => {
    page(detail({ archived: true, archivedAt: ARCHIVED_AT, budget: 5000, tokensUsed: 1200, maxTurns: 8 }));
    expect(screen.getByText("Tokens")).toBeTruthy();
    expect(screen.getByText("Turns since your answer")).toBeTruthy();
    expect(button("Change the budget")).toBeNull();
    expect(button("Change the max turns")).toBeNull();
  });
});

describe("TaskPage for a live task", () => {
  test("has no archived mark, and its controls", () => {
    page(detail());
    expect(screen.queryByText(/^Archived /)).toBeNull();
    expect(button("Add labels")).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Message tech-lead" })).toBeTruthy();
  });

  test("can change its repo and epic", () => {
    page(detail(), "details");
    expect(button("Change")).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Epic" })).toBeTruthy();
  });

  test("can share files and take them away", () => {
    page(detail({ shared: [{ name: "notes.md", size: 1, by: "you", at: ARCHIVED_AT }] }), "files");
    expect(button("Share files")).toBeTruthy();
    expect(screen.getByTitle("Stop sharing it")).toBeTruthy();
  });

  test("can change its budget and max turns while it isn't done", () => {
    page(detail({ status: "working", budget: 5000, tokensUsed: 1200, maxTurns: 8 }));
    expect(button("Change the budget")).toBeTruthy();
    expect(button("Change the max turns")).toBeTruthy();
  });
});

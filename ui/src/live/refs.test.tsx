import { describe, expect, test } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { TaskSummary } from "@/api/types";
import { Markdown } from "@/components/molecules/Markdown";
import { TaskTitlesProvider, useTaskLinks } from "./refs";

const tasks = [{ id: "task-21", title: "Fix the chat links" }] as TaskSummary[];
const commits = ["3f2a9c1d0e4b5a6978877665544332211aabbccd"];

function Entry({ text }: { text: string }) {
  return <Markdown text={text} marks {...useTaskLinks("task-75", commits)} />;
}

function page(text: string) {
  return render(
    <TaskTitlesProvider tasks={tasks}>
      <MemoryRouter initialEntries={["/tasks/task-75"]}>
        <Routes>
          <Route path="/tasks/task-75" element={<Entry text={text} />} />
          <Route path="/tasks/task-21" element={<p>the other task</p>} />
        </Routes>
      </MemoryRouter>
    </TaskTitlesProvider>,
  );
}

describe("useTaskLinks", () => {
  test("links a task the board knows, with its title from the task list, and opens it in the app", () => {
    page("see task-21 and task-999");
    const link = screen.getByRole("link", { name: "task-21, Fix the chat links" });
    expect(link.getAttribute("title")).toBe("Fix the chat links");
    expect(screen.getAllByRole("link")).toHaveLength(1);
    fireEvent.click(link);
    expect(screen.getByText("the other task")).toBeTruthy();
  });

  test("links a commit of the task, in a new tab", () => {
    page("made in `3f2a9c1`");
    const link = screen.getByRole("link", { name: "commit 3f2a9c1 (opens in a new tab)" });
    expect(link.getAttribute("href")).toBe("/tasks/task-75/workspace?commit=3f2a9c1d0e4b5a6978877665544332211aabbccd");
    expect(link.getAttribute("target")).toBe("_blank");
  });

  test("sends a file to the Workspace and the web as it is", () => {
    page("[the design](docs/design.md) and https://example.com");
    expect(screen.getByRole("link", { name: "the design (opens in a new tab)" }).getAttribute("href")).toBe(
      "/tasks/task-75/workspace?file=docs%2Fdesign.md&view=preview",
    );
    expect(screen.getByRole("link", { name: "https://example.com (opens in a new tab)" }).getAttribute("href")).toBe(
      "https://example.com/",
    );
  });
});

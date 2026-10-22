import { describe, expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { Question } from "@/api/types";
import { QuestionsPanel } from "./QuestionsPanel";

const questions: Question[] = [];
vi.mock("@/live/use-resource", () => ({ useResource: () => ({ data: questions, reload: async () => {} }) }));

const question = (task: string, more: Partial<Question> = {}): Question => ({
  id: `${task}/question/open`,
  kind: "question",
  task,
  taskTitle: `Title of ${task}`,
  agent: "tech-lead",
  at: new Date().toISOString(),
  text: "Which one?",
  context: "",
  ...more,
});

describe("QuestionsPanel", () => {
  test("names an epic's question as epic-N and links to task-N, and any other as task-N", () => {
    questions.splice(0, questions.length, question("task-21", { taskKind: "epic" }), question("task-30"));
    render(
      <MemoryRouter>
        <QuestionsPanel onOpenTerminal={() => {}} />
      </MemoryRouter>,
    );
    expect(screen.getAllByText("epic-21")).toHaveLength(2); // in the list and the one shown
    expect(screen.getByText("task-30")).toBeTruthy();
    expect(screen.getByRole("link", { name: "epic-21" }).getAttribute("href")).toBe("/tasks/task-21");
  });
});

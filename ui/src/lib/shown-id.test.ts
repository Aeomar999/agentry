import { describe, expect, it } from "vitest";
import { shownEpic, shownId } from "./shown-id";

describe("a task's id as you read it", () => {
  it("shows an epic as epic-N", () => {
    expect(shownId("task-21", "epic")).toBe("epic-21");
  });

  it("leaves a story or a task on its own as task-N", () => {
    expect(shownId("task-22", "")).toBe("task-22");
    expect(shownId("task-30")).toBe("task-30");
  });

  it("names a story's epic as epic-N, and no epic as nothing", () => {
    expect(shownEpic("task-21")).toBe("epic-21");
    expect(shownEpic("")).toBe("");
    expect(shownId("", "epic")).toBe("");
  });
});

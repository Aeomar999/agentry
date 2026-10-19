import { describe, expect, it } from "vitest";
import { RUNNING, choiceOf, limitOf, runningLine } from "./limits";

describe("the most tasks running at once", () => {
  it("shows the running count against a limit, and the queued first", () => {
    expect(runningLine(2, 2, 3)).toBe("Queued: 3 · 2 of 2 running");
    expect(runningLine(3, 1, 0)).toBe("1 of 3 running");
  });

  it("shows nothing with no limit", () => {
    expect(runningLine(0, 4, 0)).toBe("");
  });

  it("takes a whole number from 1 to 100", () => {
    expect(limitOf(RUNNING, { option: "custom", custom: "100" })).toBe(100);
    for (const text of ["0", "101", "2.5", "two", ""]) expect(limitOf(RUNNING, { option: "custom", custom: text })).toBeNaN();
  });

  it("picks No limit for 0 and a number of your own when it isn't a preset", () => {
    expect(choiceOf(RUNNING, 0)).toEqual({ option: "0", custom: "" });
    expect(choiceOf(RUNNING, 3)).toEqual({ option: "3", custom: "" });
    expect(choiceOf(RUNNING, 42)).toEqual({ option: "custom", custom: "42" });
  });
});

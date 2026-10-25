import { describe, expect, test } from "vitest";
import { tokenParts, tokenSplit } from "./tokens";

const split = { input: 4, cacheWrite: 12800, cacheRead: 12_300_000, output: 0 };

describe("tokenParts", () => {
  test("labels each part in full and counts it as people read it", () => {
    expect(tokenParts(split)).toEqual([
      ["in", "4"],
      ["cache write", "12.8k"],
      ["cache read", "12.3M"],
      ["out", "0"],
    ]);
  });
});

describe("tokenSplit", () => {
  test("puts the same parts on one line", () => {
    expect(tokenSplit(split)).toBe("in 4 · cache write 12.8k · cache read 12.3M · out 0");
  });
});

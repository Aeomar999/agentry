import { describe, expect, test } from "vitest";
import { extensionLink } from "./streams";

const ideas = { streams: { prefix: "idea-", to: (stream: string) => `/ideas#${stream}` } };
const plans = { streams: { prefix: "plan-", to: (stream: string) => `/plans/${stream}` } };
const noStreams = {};

describe("extensionLink", () => {
  test("links a stream to the feature whose prefix starts it", () => {
    expect(extensionLink("idea-7", [ideas, plans])).toBe("/ideas#idea-7");
    expect(extensionLink("plan-2", [ideas, plans])).toBe("/plans/plan-2");
  });

  test("gives no link for a stream no feature owns", () => {
    expect(extensionLink("task-3", [ideas, plans])).toBeUndefined();
    expect(extensionLink("idea-7", [])).toBeUndefined();
  });

  test("matches the whole prefix only", () => {
    expect(extensionLink("ideas-1", [ideas])).toBeUndefined();
    expect(extensionLink("idea", [ideas])).toBeUndefined();
    expect(extensionLink("idea-", [ideas])).toBeUndefined();
    expect(extensionLink("xidea-1", [ideas])).toBeUndefined();
  });

  test("gives no link for an empty stream", () => {
    expect(extensionLink("", [ideas])).toBeUndefined();
  });

  test("skips a feature that owns no streams", () => {
    expect(extensionLink("idea-7", [noStreams, ideas])).toBe("/ideas#idea-7");
    expect(extensionLink("idea-7", [noStreams])).toBeUndefined();
  });
});

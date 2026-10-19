import { describe, expect, test } from "vitest";
import { featureLink } from "./streams";

const ideas = { streams: { prefix: "idea-", to: (stream: string) => `/ideas#${stream}` } };
const plans = { streams: { prefix: "plan-", to: (stream: string) => `/plans/${stream}` } };
const noStreams = {};

describe("featureLink", () => {
  test("links a stream to the feature whose prefix starts it", () => {
    expect(featureLink("idea-7", [ideas, plans])).toBe("/ideas#idea-7");
    expect(featureLink("plan-2", [ideas, plans])).toBe("/plans/plan-2");
  });

  test("gives no link for a stream no feature owns", () => {
    expect(featureLink("task-3", [ideas, plans])).toBeUndefined();
    expect(featureLink("idea-7", [])).toBeUndefined();
  });

  test("matches the whole prefix only", () => {
    expect(featureLink("ideas-1", [ideas])).toBeUndefined();
    expect(featureLink("idea", [ideas])).toBeUndefined();
    expect(featureLink("idea-", [ideas])).toBeUndefined();
    expect(featureLink("xidea-1", [ideas])).toBeUndefined();
  });

  test("gives no link for an empty stream", () => {
    expect(featureLink("", [ideas])).toBeUndefined();
  });

  test("skips a feature that owns no streams", () => {
    expect(featureLink("idea-7", [noStreams, ideas])).toBe("/ideas#idea-7");
    expect(featureLink("idea-7", [noStreams])).toBeUndefined();
  });
});

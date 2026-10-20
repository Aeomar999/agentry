import { describe, expect, it } from "vitest";
import { AGENTS, streamName } from "./streams";

describe("a stream's name on the page", () => {
  it("names the agents' stream by what it holds", () => {
    expect(streamName("agents")).toBe(AGENTS);
    expect(AGENTS).toBe("the agents and what they learned");
  });

  it("names any other stream by its id", () => {
    for (const stream of ["task-3", "idea-1", "policy", "agents-2"]) expect(streamName(stream)).toBe(stream);
  });
});

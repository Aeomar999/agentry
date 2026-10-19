import { describe, expect, test } from "vitest";
import type { Agent } from "@/api/types";
import { roleOf, teamOf } from "./teams";

const agent = (name: string, fields: Partial<Agent> = {}) => ({ name, coordinates: false, column: "", team: "", ...fields }) as Agent;

describe("roleOf", () => {
  test.each([
    ["a coordinator", agent("product-lead", { coordinates: true, team: "Design" }), "coordinator"],
    ["a coordinator with a column", agent("lead", { coordinates: true, column: "review" }), "coordinator"],
    ["the Review column", agent("checker", { column: "review" }), "reviewer"],
    ["the Shipping column", agent("shipper", { column: "ship" }), "deployer"],
    ["no column", agent("engineer"), ""],
    ["a reviewer named reviewer", agent("reviewer", { column: "review" }), ""],
    ["a deployer named deployer", agent("deployer", { column: "ship" }), ""],
    ["a coordinator named coordinator", agent("coordinator", { coordinates: true }), ""],
  ])("names %s's role", (_, of, role) => {
    expect(roleOf(of)).toBe(role);
  });
});

describe("teamOf", () => {
  test("shows no team as Software", () => {
    expect(teamOf(agent("engineer"))).toBe("Software");
    expect(teamOf(agent("product-lead", { team: "Design" }))).toBe("Design");
  });
});

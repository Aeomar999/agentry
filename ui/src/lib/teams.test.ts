import { describe, expect, test, vi } from "vitest";
import type { Agent } from "@/api/types";
import { roleOf, teamMadeFrom, teamOf } from "./teams";

// The client reads the address when it loads, and there is none here.
vi.mock("@/api/client", () => ({ api: vi.fn(), ApiError: class {} }));
vi.mock("@/live/use-resource", () => ({ useResource: vi.fn() }));

// agent is a roster agent with only what teamMadeFrom reads set.
function agent(name: string, team: string, template: string, coordinates = false): Agent {
  return {
    name,
    description: "",
    prompt: "",
    engine: "claude",
    model: "",
    profile: "",
    coordinates,
    team,
    template,
    workers: [],
    color: "",
    column: "",
    knowledge: "",
  };
}

describe("teamMadeFrom", () => {
  test("finds a second copy by its template, by the name it has", () => {
    const agents = [
      agent("lead", "", "software", true),
      agent("visionary-lead", "Visionaries 2", "visionaries", true),
      agent("user-advocate", "Visionaries 2", "visionaries"),
    ];
    expect(teamMadeFrom(agents, "visionaries")).toBe("Visionaries 2");
  });

  test("skips a team you made and named as the template", () => {
    const agents = [agent("my-lead", "Visionaries", "", true), agent("my-thinker", "Visionaries", "")];
    expect(teamMadeFrom(agents, "visionaries")).toBeUndefined();
  });

  test("finds it beside a hand-made team of the same name", () => {
    const agents = [agent("my-lead", "Visionaries", "", true), agent("visionary-lead", "Visionaries 2", "visionaries", true)];
    expect(teamMadeFrom(agents, "visionaries")).toBe("Visionaries 2");
  });

  test("goes by the coordinator, not an agent the template made that moved team", () => {
    const agents = [agent("lead", "", "software", true), agent("user-advocate", "", "visionaries")];
    expect(teamMadeFrom(agents, "visionaries")).toBeUndefined();
  });

  test("takes the first of two teams made from the template", () => {
    const agents = [
      agent("visionary-lead", "Visionaries", "visionaries", true),
      agent("visionary-lead-2", "Visionaries 2", "visionaries", true),
    ];
    expect(teamMadeFrom(agents, "visionaries")).toBe("Visionaries");
  });

  test("finds none with no agents, or no template", () => {
    expect(teamMadeFrom([], "visionaries")).toBeUndefined();
    expect(teamMadeFrom([agent("my-lead", "Mine", "", true)], "")).toBeUndefined();
  });

  test("names the default team Software", () => {
    expect(teamMadeFrom([agent("lead", "", "software", true)], "software")).toBe("Software");
  });
});

const member = (name: string, fields: Partial<Agent> = {}) => ({ name, coordinates: false, column: "", team: "", ...fields }) as Agent;

describe("roleOf", () => {
  test.each([
    ["a coordinator", member("product-lead", { coordinates: true, team: "Design" }), "coordinator"],
    ["a coordinator with a column", member("lead", { coordinates: true, column: "review" }), "coordinator"],
    ["the Review column", member("checker", { column: "review" }), "reviewer"],
    ["the Shipping column", member("shipper", { column: "ship" }), "deployer"],
    ["no column", member("engineer"), ""],
    ["a reviewer named reviewer", member("reviewer", { column: "review" }), ""],
    ["a deployer named deployer", member("deployer", { column: "ship" }), ""],
    ["a coordinator named coordinator", member("coordinator", { coordinates: true }), ""],
  ])("names %s's role", (_, of, role) => {
    expect(roleOf(of)).toBe(role);
  });
});

describe("teamOf", () => {
  test("shows no team as Software", () => {
    expect(teamOf(member("engineer"))).toBe("Software");
    expect(teamOf(member("product-lead", { team: "Design" }))).toBe("Design");
  });
});

import { beforeEach, describe, expect, test, vi } from "vitest";
import { api } from "@/api/client";
import type { Agent, Policy, PolicyProfile, TeamTemplate } from "@/api/types";
import {
  addCustomTeam,
  agentCount,
  backNotice,
  blankNotice,
  blankTeam,
  checkTeam,
  changedAgents,
  copyTemplate,
  type DraftAgent,
  footerLine,
  listed,
  missingLine,
  missingProfiles,
  newAgent,
  problemCount,
  removedNotice,
  replacedNotice,
  replaceDetail,
  replaceQuestion,
  summaryTitle,
  type TeamDraft,
  type TeamRules,
  thingCount,
} from "./custom-team";

// The client reads the address when it loads, and there is none here.
vi.mock("@/api/client", () => ({ api: vi.fn(), ApiError: class {} }));
vi.mock("@/live/use-resource", () => ({ useResource: vi.fn() }));

// agent is a roster agent with only what the team checks read set.
function agent(name: string, team = "", coordinates = false): Agent {
  return {
    name,
    description: "",
    prompt: "",
    engine: "claude",
    model: "",
    profile: "",
    coordinates,
    team,
    template: "",
    workers: [],
    color: "",
    column: "",
    knowledge: "",
  };
}

// row is a draft row that passes every check, changed by what is given.
function row(key: string, given: Partial<DraftAgent> = {}): DraftAgent {
  return {
    key,
    name: key,
    description: "",
    coordinates: false,
    prompt: "Do the work.",
    engine: "claude",
    model: "",
    profile: "",
    color: "",
    column: "",
    ...given,
  };
}

const profile = (name: string): PolicyProfile => ({ name, why: "", otherwise: "ask", rules: [] });

const policy: Pick<Policy, "profiles" | "defaults"> = {
  profiles: [profile("default"), profile("read-mostly")],
  defaults: [
    { name: "read-mostly", state: "current", text: "" },
    { name: "research-only", state: "missing", text: "" },
  ],
};

const rules: TeamRules = {
  templates: [{ team: "Software" }, { team: "Writing" }],
  policy,
  engines: ["claude", "codex"],
};

const roster = [agent("lead", "", true), agent("engineer"), agent("editor", "Docs", true)];

// A valid team: a coordinator and one agent.
function draft(...more: DraftAgent[]): TeamDraft {
  return { team: "Research", agents: [row("chief", { coordinates: true, prompt: "" }), ...more] };
}

// problemsOf is what checkTeam finds, as [message, summary] pairs.
const problemsOf = (team: TeamDraft) => checkTeam(team, roster, rules).map((p) => [p.message, p.summary]);

const template: TeamTemplate = {
  id: "writing",
  team: "Writing",
  description: "Writes docs.",
  features: [],
  agents: [
    {
      name: "editor",
      description: "Hands out the writing.",
      coordinates: true,
      prompt: "You are the editor.",
      engine: "claude",
      model: "opus",
      profile: "read-mostly",
      color: "violet",
      column: "",
    },
    {
      name: "writer",
      description: "Writes.",
      coordinates: false,
      prompt: "You are the writer.",
      engine: "codex",
      model: "",
      profile: "research-only",
      color: "teal",
      column: "review",
    },
  ],
};

describe("blankTeam", () => {
  test("is one coordinator with no prompt and no team name", () => {
    const team = blankTeam([]);
    expect(team.team).toBe("");
    expect(team.agents).toHaveLength(1);
    expect(team.agents[0]).toMatchObject({ name: "coordinator", coordinates: true, prompt: "", engine: "claude", column: "" });
  });

  test("numbers the coordinator when the name is taken", () => {
    expect(blankTeam([agent("coordinator")]).agents[0].name).toBe("coordinator-2");
    expect(blankTeam([agent("coordinator"), agent("Coordinator-2")]).agents[0].name).toBe("coordinator-3");
  });

  test("passes the checks once named", () => {
    expect(checkTeam({ ...blankTeam(roster), team: "Research" }, roster, rules)).toEqual([]);
  });

  test("gives each row its own key", () => {
    expect(blankTeam([]).agents[0].key).not.toBe(blankTeam([]).agents[0].key);
  });
});

describe("copyTemplate", () => {
  test("copies every field of every agent, prompts included", () => {
    const team = copyTemplate(template, []);
    expect(team.agents.map(({ key: _, ...rest }) => rest)).toEqual(template.agents);
    expect(new Set(team.agents.map((a) => a.key)).size).toBe(2);
  });

  test("names the team after the template with a number, since the template's own name is a built-in's", () => {
    expect(copyTemplate(template, []).team).toBe("Writing 2");
    expect(copyTemplate(template, [agent("x", "writing 2"), agent("y", "Writing 3")]).team).toBe("Writing 4");
  });

  test("numbers names you have already, as the server's AddTeam does", () => {
    const team = copyTemplate(template, [agent("editor", "Docs"), agent("editor-2"), agent("writer")]);
    expect(team.agents.map((a) => a.name)).toEqual(["editor-3", "writer-2"]);
  });

  test("numbers a name another copied agent takes first", () => {
    const twice: TeamTemplate = { ...template, agents: [template.agents[0], template.agents[0]] };
    expect(copyTemplate(twice, []).agents.map((a) => a.name)).toEqual(["editor", "editor-2"]);
  });

  test("keeps a numbered name within 31 characters", () => {
    const long = "a".repeat(31);
    const named: TeamTemplate = { ...template, agents: [{ ...template.agents[0], name: long }] };
    const name = copyTemplate(named, [agent(long)]).agents[0].name;
    expect(name).toBe(`${"a".repeat(29)}-2`);
  });

  test("passes the checks, its missing shipped profile included", () => {
    expect(checkTeam(copyTemplate(template, roster), roster, rules)).toEqual([]);
  });
});

describe("checkTeam", () => {
  test("a valid team has no problems", () => {
    expect(problemsOf(draft(row("analyst")))).toEqual([]);
  });

  test("team name empty", () => {
    expect(problemsOf({ ...draft(), team: "  " })).toEqual([["Give the team a name.", "Team name: give the team a name"]]);
  });

  test("team name taken, in any case", () => {
    expect(problemsOf({ ...draft(), team: " docs " })).toEqual([
      ["You have a team called Docs already. Pick another name.", "Team name: you have a team called Docs already"],
    ]);
  });

  test("an agent with no team is in Software", () => {
    expect(problemsOf({ ...draft(), team: "software" })[0][1]).toBe("Team name: you have a team called Software already");
  });

  test("team name a built-in template's team has", () => {
    expect(problemsOf({ ...draft(), team: "WRITING" })).toEqual([
      ["Writing is a built-in team's name. Pick another, such as Writing 2.", "Team name: Writing is a built-in team's name"],
    ]);
  });

  test("no coordinator", () => {
    const team = { team: "Research", agents: [row("analyst")] };
    expect(problemsOf(team)).toEqual([["Pick one agent to coordinate.", "Agents: pick one agent to coordinate"]]);
    expect(checkTeam(team, roster, rules)[0].field).toBe("agents");
  });

  test("two or more coordinators", () => {
    const two = draft(row("head", { coordinates: true }));
    expect(problemsOf(two)).toEqual([
      ["chief and head both coordinate. Pick the one that takes the team's tasks.", "Agents: chief and head both coordinate"],
    ]);
    expect(checkTeam(two, roster, rules)[0].field).toBe("agents");
    const three = draft(row("head", { coordinates: true }), row("boss", { coordinates: true }));
    expect(problemsOf(three)[0][0]).toBe("chief, head and boss all coordinate. Pick the one that takes the team's tasks.");
  });

  test("no agents", () => {
    expect(problemsOf({ team: "Research", agents: [] })).toEqual([["Add an agent: it coordinates the team.", "Agents: add an agent"]]);
  });

  test("agent name empty", () => {
    expect(problemsOf(draft(row("a1", { name: " " })))).toEqual([["Give this agent a name.", "New agent: give it a name"]]);
  });

  test("agent name, wrong letters", () => {
    expect(problemsOf(draft(row("a1", { name: "Analyst_1" })))).toEqual([
      ["Use only lowercase letters, digits and dashes.", "Analyst_1: use only lowercase letters, digits and dashes"],
    ]);
  });

  test("agent name not starting with a letter", () => {
    expect(problemsOf(draft(row("a1", { name: "2nd" })))).toEqual([["Start the name with a letter.", "2nd: start the name with a letter"]]);
  });

  test("agent name too long", () => {
    const name = "a".repeat(32);
    expect(problemsOf(draft(row("a1", { name: "a".repeat(31) })))).toEqual([]);
    expect(problemsOf(draft(row("a1", { name })))).toEqual([["Use 31 characters or fewer.", `${name}: use 31 characters or fewer`]]);
  });

  test("same name twice in the team marks only the later row", () => {
    const team = draft(row("a1", { name: "analyst" }), row("a2", { name: " analyst " }));
    expect(problemsOf(team)).toEqual([
      [
        "Another agent in this team is called analyst. Pick another name.",
        "analyst (second one): another agent in this team is called analyst",
      ],
    ]);
    expect(checkTeam(team, roster, rules)[0].field).toEqual({ row: "a2", field: "name" });
  });

  test("name another team's agent has", () => {
    expect(problemsOf(draft(row("a1", { name: "editor" })))).toEqual([
      [
        "You have an agent called editor already, in the Docs team. Pick another name.",
        "editor: you have an agent called editor already, in Docs",
      ],
    ]);
    expect(problemsOf(draft(row("a1", { name: "engineer" })))[0][1]).toBe(
      "engineer: you have an agent called engineer already, in Software",
    );
  });

  test("prompt empty, not the coordinator", () => {
    expect(problemsOf(draft(row("analyst", { prompt: "  " })))).toEqual([
      ["Write a prompt: only the coordinator can do without one.", "analyst: write a prompt"],
    ]);
  });

  test("an unnamed agent's problems name it New agent", () => {
    expect(problemsOf(draft(row("a1", { name: "", prompt: "" })))[1]).toEqual([
      "Write a prompt: only the coordinator can do without one.",
      "New agent: write a prompt",
    ]);
  });

  test("unknown profile", () => {
    expect(problemsOf(draft(row("analyst", { profile: "made-up" })))).toEqual([
      ["policy.toml has no profile called made-up. Pick one it has.", "analyst: policy.toml has no profile called made-up"],
    ]);
  });

  test("a shipped profile policy.toml lacks isn't a problem", () => {
    expect(problemsOf(draft(row("analyst", { profile: "research-only" })))).toEqual([]);
  });

  test("unknown engine", () => {
    expect(problemsOf(draft(row("analyst", { engine: "gpt" })))).toEqual([
      ["Agentry has no engine called gpt. Pick claude or codex.", "analyst: Agentry has no engine called gpt"],
    ]);
  });

  test("no engine", () => {
    expect(problemsOf(draft(row("analyst", { engine: "" })))).toEqual([["Pick an engine: claude or codex.", "analyst: pick an engine"]]);
  });

  test("colour outside the agent colours", () => {
    expect(problemsOf(draft(row("analyst", { color: "mauve" })))).toEqual([
      ["There's no colour called mauve. Pick one of the swatches.", "analyst: there's no colour called mauve"],
    ]);
  });

  test("column outside builds, review and ship", () => {
    const column = "deploy" as DraftAgent["column"];
    expect(problemsOf(draft(row("analyst", { column })))).toEqual([
      ["Its work can't count as deploy. Pick Building, Review or Shipping.", "analyst: its work can't count as deploy"],
    ]);
  });

  test("finds every problem at once, in screen order", () => {
    const team: TeamDraft = {
      team: "docs",
      agents: [
        row("a1", { name: "editor", prompt: "" }),
        row("a2", { name: "", prompt: "", profile: "made-up" }),
        row("a3", { name: "Bad" }),
      ],
    };
    const problems = checkTeam(team, roster, rules);
    expect(problems.map((p) => p.field)).toEqual([
      "team",
      "agents",
      { row: "a1", field: "name" },
      { row: "a1", field: "prompt" },
      { row: "a2", field: "name" },
      { row: "a2", field: "prompt" },
      { row: "a2", field: "profile" },
      { row: "a3", field: "name" },
    ]);
  });
});

describe("newAgent", () => {
  test("has no name, a colour no row has, Building and the first engine", () => {
    const added = newAgent(draft(row("a1", { color: "blue" })), ["codex", "claude"]);
    expect(added).toMatchObject({ name: "", prompt: "", coordinates: false, engine: "codex", profile: "", column: "", color: "violet" });
  });

  test("coordinates a team with no agents", () => {
    expect(newAgent({ team: "", agents: [] }, ["claude"]).coordinates).toBe(true);
  });
});

describe("changedAgents", () => {
  test("names the rows changed, added and removed since the start", () => {
    const start = draft(row("a1"), row("a2"));
    const [chief, a1, a2] = start.agents;
    const now = { ...start, agents: [chief, a1, { ...a2, prompt: "Other." }, row("a3", { name: "" })] };
    expect(changedAgents(start, now)).toEqual(["a2", "New agent"]);
    expect(changedAgents(start, { ...start, agents: [a1, a2] })).toEqual(["chief"]);
    expect(changedAgents(start, { ...start, team: "Other" })).toEqual([]);
  });
});

describe("missingProfiles", () => {
  test("lists the shipped profiles policy.toml lacks, once each", () => {
    const team = draft(row("a1", { profile: "research-only" }), row("a2", { profile: "research-only" }), row("a3", { profile: "made-up" }));
    expect(missingProfiles(team, policy)).toEqual(["research-only"]);
  });

  test("is empty when policy.toml has them all", () => {
    expect(missingProfiles(draft(row("a1", { profile: "read-mostly" })), policy)).toEqual([]);
  });
});

describe("the panel's sentences", () => {
  const research = { team: "Research", agents: [row("r1"), row("r2"), row("r3")] };

  test("listed joins with or, or with and", () => {
    expect(listed([])).toBe("");
    expect(listed(["a"])).toBe("a");
    expect(listed(["a", "b"])).toBe("a or b");
    expect(listed(["a", "b", "c"], "and")).toBe("a, b and c");
  });

  test("counts say 1 of one, n of many", () => {
    expect([agentCount(1), agentCount(0), thingCount(1), thingCount(3), problemCount(1), problemCount(2)]).toEqual([
      "1 agent",
      "0 agents",
      "1 thing",
      "3 things",
      "1 problem",
      "2 problems",
    ]);
  });

  test("footerLine says what Create team does, or why it won't yet", () => {
    expect(footerLine(draft(), 0, true)).toBe("Creating the team…");
    expect(footerLine(draft(), 2, false)).toBe("2 things to fix, listed at the top.");
    expect(footerLine(draft(), 1, false)).toBe("1 thing to fix, listed at the top.");
    expect(footerLine({ team: "", agents: [row("chief", { coordinates: true })] }, 0, false)).toBe("Creates the team and its coordinator.");
    expect(footerLine({ team: "", agents: [row("a1")] }, 0, false)).toBe("Creates the team and its 1 agent.");
    expect(footerLine(draft(row("a1")), 0, false)).toBe("Creates the team and its 2 agents.");
  });

  test("summaryTitle counts the things to fix, if any", () => {
    expect(summaryTitle(1)).toBe("Couldn't create the team: 1 thing to fix");
    expect(summaryTitle(0)).toBe("Couldn't create the team");
  });

  test("missingLine names who uses which profiles policy.toml lacks", () => {
    const one = draft(row("a1", { profile: " research-only " }));
    expect(missingLine(one, ["research-only"])).toBe(
      "1 agent uses research-only, which policy.toml doesn't have. Create team offers to add it.",
    );
    const two = draft(row("a1", { profile: "x" }), row("a2", { profile: "y" }), row("a3", { profile: "z" }));
    expect(missingLine(two, ["x", "y", "z"])).toBe(
      "3 agents use x, y and z, which policy.toml doesn't have. Create team offers to add them.",
    );
  });

  test("replaceQuestion and replaceDetail say what replacing changes", () => {
    expect(replaceQuestion(["a1"], research)).toBe("Replace your changed agent with Research's?");
    expect(replaceQuestion(["a1", "a2"])).toBe("Replace your 2 changed agents with a blank team?");
    expect(replaceDetail(["a1", "a2"], research, { team: "Writing" })).toBe(
      "Research's 3 agents replace Writing's; your changes to a1 and a2 go.",
    );
    expect(replaceDetail(["a1"], research)).toBe("Research's 3 agents replace yours; your changes to a1 go.");
    expect(replaceDetail(["a1"])).toBe("A blank team has one agent, its coordinator.");
  });

  test("the live region's notices name the agent, as yet unnamed too", () => {
    expect(removedNotice(row("a1"))).toBe("a1 removed. Undo is next.");
    expect(removedNotice(row("a1", { name: " " }))).toBe("New agent removed. Undo is next.");
    expect(backNotice(row("a1"))).toBe("a1 is back.");
    expect(replacedNotice(research)).toBe("Replaced with Research's 3 agents.");
    expect(blankNotice).toBe("Replaced with a blank team: one agent, its coordinator.");
  });
});

describe("addCustomTeam", () => {
  beforeEach(() => vi.mocked(api).mockReset());

  const team: TeamDraft = {
    team: " Research ",
    agents: [row("k1", { name: " chief ", coordinates: true, prompt: " ", description: " Leads. ", model: " opus ", color: " blue " })],
  };
  const sent = {
    team: "Research",
    agents: [
      {
        name: "chief",
        description: "Leads.",
        coordinates: true,
        prompt: "",
        engine: "claude",
        model: "opus",
        profile: "",
        color: "blue",
        column: "",
      },
    ],
  };

  test("posts the team and its agents, trimmed, without the row keys", async () => {
    await addCustomTeam(team);
    expect(api).toHaveBeenCalledWith("POST", "/api/teams", sent);
  });

  test("with the policy's hash, asks for the missing profiles too", async () => {
    await addCustomTeam(team, "h1");
    expect(api).toHaveBeenCalledWith("POST", "/api/teams", { ...sent, addMissingProfiles: true, hash: "h1" });
  });
});

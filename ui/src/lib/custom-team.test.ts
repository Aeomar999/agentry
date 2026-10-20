import { beforeEach, describe, expect, test, vi } from "vitest";
import { api, ApiError } from "@/api/client";
import type { Agent, Policy, PolicyProfile, TeamTemplate } from "@/api/types";
import { AGENT_COLORS } from "@/live/agent-colors";
import {
  addCustomTeam,
  agentCount,
  backNotice,
  blankNotice,
  blankTeam,
  checkTeam,
  changedAgents,
  copyTemplate,
  discardEditDetail,
  type DraftAgent,
  editCustomTeam,
  editFooterLine,
  FIRST_TEMPLATE,
  footerLine,
  listed,
  type LoadedTeam,
  loadTeam,
  missingLine,
  missingProfiles,
  newAgent,
  problemCount,
  problemsOfEdit,
  removedNotice,
  replacedNotice,
  replaceDetail,
  replaceQuestion,
  rowChange,
  saveAgain,
  summaryTitle,
  type TeamDraft,
  type TeamRules,
  templateToCopy,
  thingCount,
  unsaved,
  whyNotLoaded,
  whyNotSaved,
} from "./custom-team";

// The client reads the address when it loads, and there is none here.
vi.mock("@/api/client", () => ({
  api: vi.fn(),
  ApiError: class extends Error {
    status: number;
    body?: Record<string, unknown>;
    constructor(message: string, status: number, body?: Record<string, unknown>) {
      super(message);
      this.status = status;
      this.body = body;
    }
  },
}));
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

  test("gives the coordinator the first colour on an empty roster", () => {
    expect(blankTeam([]).agents[0].color).toBe(AGENT_COLORS[0]);
  });

  test("gives the coordinator a colour no roster agent has", () => {
    const colored = AGENT_COLORS.slice(0, 2).map((color, i) => ({ ...agent(`a${i}`), color }));
    expect(blankTeam([...colored, agent("plain")]).agents[0].color).toBe(AGENT_COLORS[2]);
  });

  test("falls back to the first colour when every colour is taken", () => {
    const colored = AGENT_COLORS.map((color, i) => ({ ...agent(`a${i}`), color }));
    expect(blankTeam(colored).agents[0].color).toBe(AGENT_COLORS[0]);
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

describe("unsaved", () => {
  test("is a renamed team or a changed, added or removed agent; a change typed back is none", () => {
    const start = draft(row("a1"));
    const [chief, a1] = start.agents;
    expect(unsaved(start, start)).toBe(false);
    expect(unsaved(start, { ...start, team: ` ${start.team} ` })).toBe(false);
    expect(unsaved(start, { ...start, team: "Other" })).toBe(true);
    expect(unsaved(start, { ...start, agents: [chief] })).toBe(true);
    expect(unsaved(start, { ...start, agents: [chief, { ...a1, description: "Other." }] })).toBe(true);
    expect(unsaved(start, { ...start, agents: [chief, { ...a1 }] })).toBe(false);
  });
});

describe("templateToCopy", () => {
  const templates = [{ id: "writing" }, { id: "research" }] as TeamTemplate[];

  test("is the template the id names, the first for one picked before they came, or none", () => {
    expect(templateToCopy(templates, "research")).toBe(templates[1]);
    expect(templateToCopy(templates, FIRST_TEMPLATE)).toBe(templates[0]);
    expect(templateToCopy(templates, "")).toBeUndefined();
    expect(templateToCopy(templates, "gone")).toBeUndefined();
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

  test("summaryTitle says, editing, the team wasn't saved", () => {
    expect(summaryTitle(2, true)).toBe("Couldn't save the team: 2 things to fix");
    expect(summaryTitle(0, true)).toBe("Couldn't save the team");
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

  test("a read that couldn't reach Agentry says so in our words; the server's own words stay", () => {
    const unreachable = "Couldn't reach Agentry. Check it's running, then try again.";
    expect(whyNotLoaded("Failed to fetch")).toBe(unreachable);
    expect(whyNotLoaded("NetworkError when attempting to fetch resource.")).toBe(unreachable);
    expect(whyNotLoaded("Load failed")).toBe(unreachable);
    expect(whyNotLoaded("the config is unreadable")).toBe("the config is unreadable");
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

// The Docs team as GET /api/agents gives it, at version v1, beside Software.
const docs = [
  agent("lead", "", true),
  { ...agent("writer", "Docs"), prompt: "You write.", profile: "read-mostly", color: "teal", column: "review" as const, teamVersion: "v1" },
  { ...agent("editor", "Docs", true), description: "Leads.", model: "opus", teamVersion: "v1" },
];

describe("loadTeam", () => {
  test("is the team named in any case, its coordinator first, each agent's fields in a row of its own, at the team's version", () => {
    const loaded = loadTeam(docs, "docs");
    expect(loaded.team).toBe("Docs");
    expect(loaded.version).toBe("v1");
    expect(loaded.agents.map(({ key: _, ...fields }) => fields)).toEqual([
      {
        name: "editor",
        description: "Leads.",
        coordinates: true,
        prompt: "",
        engine: "claude",
        model: "opus",
        profile: "",
        color: "",
        column: "",
      },
      {
        name: "writer",
        description: "",
        coordinates: false,
        prompt: "You write.",
        engine: "claude",
        model: "",
        profile: "read-mostly",
        color: "teal",
        column: "review",
      },
    ]);
    expect(new Set(loaded.agents.map((agent) => agent.key)).size).toBe(2);
  });

  test("a team you don't have has no agents and no version", () => {
    expect(loadTeam(docs, "Research")).toEqual({ team: "Research", agents: [], version: "" });
  });

  test("starts a draft with nothing unsaved", () => {
    const loaded = loadTeam(docs, "Docs");
    expect(unsaved(loaded, { team: loaded.team, agents: [...loaded.agents] })).toBe(false);
  });
});

// The Docs team loaded to edit: editor coordinates writer and proofer.
const loaded: LoadedTeam = {
  team: "Docs",
  version: "v1",
  agents: [row("k1", { name: "editor", coordinates: true }), row("k2", { name: "writer" }), row("k3", { name: "proofer" })],
};

const sentAgent = (was: string, name: string, coordinates = false) => ({
  was,
  name,
  description: "",
  coordinates,
  prompt: "Do the work.",
  engine: "claude",
  model: "",
  profile: "",
  color: "",
  column: "",
});

describe("editCustomTeam", () => {
  beforeEach(() => vi.mocked(api).mockReset());

  // Renamed Writing; writer renamed author and coordinating; proofer removed; reviewer added.
  const edited: TeamDraft = {
    team: " Writing ",
    agents: [
      { ...loaded.agents[0], coordinates: false },
      { ...loaded.agents[1], name: " author ", coordinates: true },
      row("k9", { name: "reviewer" }),
    ],
  };
  const sent = {
    version: "v1",
    team: "Writing",
    agents: [sentAgent("editor", "editor"), sentAgent("writer", "author", true), sentAgent("", "reviewer")],
  };

  test("puts the team under its loaded name: renamed, each row with the name it had, a new one with none, a removed one left out", async () => {
    await editCustomTeam(loaded, edited);
    expect(api).toHaveBeenCalledWith("PUT", "/api/teams/Docs", sent);
  });

  test("with the policy's hash, asks for the missing profiles too", async () => {
    await editCustomTeam(loaded, edited, "h1");
    expect(api).toHaveBeenCalledWith("PUT", "/api/teams/Docs", { ...sent, addMissingProfiles: true, hash: "h1" });
  });

  test("escapes the team's name in the address", async () => {
    await editCustomTeam({ ...loaded, team: "R&D 2" }, edited);
    expect(api).toHaveBeenCalledWith("PUT", "/api/teams/R%26D%202", sent);
  });

  test("saveAgain saves the reloaded team unchanged", async () => {
    await saveAgain(loaded);
    expect(api).toHaveBeenCalledWith("PUT", "/api/teams/Docs", {
      version: "v1",
      team: "Docs",
      agents: [sentAgent("editor", "editor", true), sentAgent("writer", "writer"), sentAgent("proofer", "proofer")],
    });
  });
});

describe("problemsOfEdit", () => {
  const draft = loaded;

  test("a row's problem goes under its field, by the row's key", () => {
    expect(
      problemsOfEdit([{ row: 1, field: "name", error: "an agent called writer already exists, in the Docs team" }], draft, loaded),
    ).toEqual([
      {
        field: { row: "k2", field: "name" },
        message: "An agent called writer already exists, in the Docs team.",
        summary: "writer: an agent called writer already exists, in the Docs team",
      },
    ]);
  });

  test("the name a row had goes under its name", () => {
    expect(problemsOfEdit([{ row: 2, field: "was", error: "ghost isn't an agent of the Docs team" }], draft, loaded)[0].field).toEqual({
      row: "k3",
      field: "name",
    });
  });

  test("the team's name goes under Team name; its coordinators and agents under the Agents legend", () => {
    const problems = problemsOfEdit(
      [
        { field: "team", error: "a team called Software already exists" },
        { field: "coordinates", error: "the Docs team has two coordinators" },
        { field: "agents", error: "the edit neither keeps nor removes proofer" },
      ],
      draft,
      loaded,
    );
    expect(problems.map((p) => [p.field, p.message, p.summary])).toEqual([
      ["team", "A team called Software already exists.", "Team name: a team called Software already exists"],
      ["agents", "The Docs team has two coordinators.", "Agents: the Docs team has two coordinators"],
      ["agents", "The edit neither keeps nor removes proofer.", "Agents: the edit neither keeps nor removes proofer"],
    ]);
  });

  test("a removed agent tasks still need goes under the Agents legend, naming it and the tasks", () => {
    const [one] = problemsOfEdit(
      [{ field: "removed", agent: "proofer", error: "proofer is still needed by task-1: finish those first, or keep proofer" }],
      draft,
      loaded,
    );
    expect(one).toEqual({
      field: "agents",
      message: "proofer is still needed by task-1. Finish it first, or undo to keep proofer.",
      summary: "proofer: still needed by task-1",
      removed: { agent: "proofer", tasks: ["task-1"] },
    });
    const [two] = problemsOfEdit(
      [{ field: "removed", agent: "proofer", error: "proofer is still needed by task-1, task-2: finish those first, or keep proofer" }],
      draft,
      loaded,
    );
    expect([two.message, two.summary, two.removed]).toEqual([
      "proofer is still needed by task-1 and task-2. Finish them first, or undo to keep proofer.",
      "proofer: still needed by task-1 and task-2",
      { agent: "proofer", tasks: ["task-1", "task-2"] },
    ]);
  });

  test("a removed agent's other problem keeps the server's words, and no stub of a removed agent tasks need", () => {
    const problems = problemsOfEdit(
      [
        { field: "removed", agent: "ghost", error: "ghost isn't an agent of the Docs team to remove" },
        { field: "removed", agent: "writer", error: "writer can't be both kept and removed" },
      ],
      draft,
      loaded,
    );
    expect(problems).toEqual([
      {
        field: "agents",
        message: "ghost isn't an agent of the Docs team to remove.",
        summary: "ghost: ghost isn't an agent of the Docs team to remove",
      },
      { field: "agents", message: "writer can't be both kept and removed.", summary: "writer: writer can't be both kept and removed" },
    ]);
  });

  test("a reason starting with an agent's name keeps it lowercase, the name it had or the one it has", () => {
    const renamed: TeamDraft = { ...loaded, agents: [loaded.agents[0], { ...loaded.agents[1], name: "author" }, loaded.agents[2]] };
    const problems = problemsOfEdit(
      [
        { row: 1, field: "profile", error: "author's profile x isn't in policy.toml" },
        { field: "agents", error: "writer's tasks can't move" },
        { field: "agents", error: "editors can't be two" },
      ],
      renamed,
      loaded,
    );
    expect(problems.map((p) => p.message)).toEqual([
      "author's profile x isn't in policy.toml.",
      "writer's tasks can't move.",
      "Editors can't be two.",
    ]);
  });

  test("the request's rows are the draft's: with a middle row removed, the last is row 1", () => {
    const removed: TeamDraft = { ...loaded, agents: [loaded.agents[0], loaded.agents[2]] };
    expect(problemsOfEdit([{ row: 1, field: "name", error: "use another name" }], removed, loaded)[0]).toEqual({
      field: { row: "k3", field: "name" },
      message: "Use another name.",
      summary: "proofer: use another name",
    });
  });

  test("a field the panel has no place for goes under the Agents legend, never dropped", () => {
    const problems = problemsOfEdit(
      [
        { row: 0, field: "description", error: "the description is too long" },
        { field: "version", error: "something new" },
        { row: 7, field: "name", error: "a row the draft hasn't" },
      ],
      draft,
      loaded,
    );
    expect(problems.map((p) => [p.field, p.message, p.summary])).toEqual([
      ["agents", "editor: The description is too long.", "editor: the description is too long"],
      ["agents", "Something new.", "Agents: something new"],
      ["agents", "A row the draft hasn't.", "Agents: a row the draft hasn't"],
    ]);
  });
});

describe("checkTeam editing a team", () => {
  // The Docs team as loaded, with Software's lead beside it.
  const roster = [agent("lead", "", true), agent("editor", "Docs", true), agent("writer", "Docs"), agent("scribe", "Writing", true)];
  const team = (name: string, ...names: string[]): TeamDraft => ({
    team: name,
    agents: names.map((n, i) => row(`k${i}`, { name: n, coordinates: i === 0 })),
  });
  const problemsOf = (draft: TeamDraft, editing?: TeamDraft) => checkTeam(draft, roster, rules, editing).map((p) => p.summary);
  const docs = team("Docs", "editor", "writer");

  test("its own name and its agents' names are free, in any case and whichever rows have them", () => {
    expect(problemsOf(team("Docs", "editor", "writer"), docs)).toEqual([]);
    expect(problemsOf(team("DOCS", "writer", "editor"), docs)).toEqual([]);
  });

  test("only a new coordinator may have no prompt: the server writes the default for a new agent alone", () => {
    const cleared = { ...docs, agents: [{ ...docs.agents[0], prompt: " " }, docs.agents[1]] };
    expect(checkTeam(cleared, roster, rules, docs).map((p) => [p.field, p.message])).toEqual([
      [{ row: "k0", field: "prompt" }, "Write a prompt: only a new coordinator can do without one."],
    ]);
    const handedOn = {
      ...docs,
      agents: [{ ...docs.agents[0], coordinates: false }, docs.agents[1], row("k9", { coordinates: true, prompt: "" })],
    };
    expect(problemsOf(handedOn, docs)).toEqual([]);
  });

  test("a new team with the same names clashes, as before", () => {
    expect(problemsOf(team("Docs", "editor", "writer"))).toEqual([
      "Team name: you have a team called Docs already",
      "editor: you have an agent called editor already, in Docs",
      "writer: you have an agent called writer already, in Docs",
    ]);
  });

  test("another team's name and agents, a built-in's name and a name twice in the team still clash", () => {
    expect(problemsOf(team("Software", "lead", "writer"), docs)).toEqual([
      "Team name: you have a team called Software already",
      "lead: you have an agent called lead already, in Software",
    ]);
    expect(problemsOf(team("Writing 2", "writer", "writer"), docs)).toEqual([
      "writer (second one): another agent in this team is called writer",
    ]);
    expect(problemsOf(team("writing", "editor"), docs)).toEqual(["Team name: you have a team called Writing already"]);
  });

  test("a team named as a built-in one keeps its name", () => {
    expect(problemsOf(team("Writing", "scribe", "writer-2"), { team: "Writing", agents: [] })).toEqual([]);
    expect(problemsOf(team("Writing", "editor", "writer"), docs)).toEqual(["Team name: you have a team called Writing already"]);
  });
});

describe("whyNotSaved", () => {
  const draft: TeamDraft = { ...loaded, team: "Writing" };
  const refused = (status: number, body?: Record<string, unknown>, message = String(body?.error ?? "")) =>
    whyNotSaved(new ApiError(message, status, body), loaded, draft);

  test("problems, put where the panel shows them", () => {
    expect(
      refused(400, { error: "the Writing team can't be saved: …", problems: [{ row: 1, field: "prompt", error: "give it a prompt" }] }),
    ).toEqual({
      kind: "problems",
      problems: [{ field: { row: "k2", field: "prompt" }, message: "Give it a prompt.", summary: "writer: give it a prompt" }],
    });
  });

  test("a removed agent a task needs is problems too, though a 409", () => {
    const why = refused(409, {
      error: "…",
      problems: [{ field: "removed", agent: "proofer", error: "proofer is still needed by task-1: finish those first, or keep proofer" }],
    });
    expect(why.kind).toBe("problems");
  });

  test("changed elsewhere: reload before saving", () => {
    const message = "the Docs team changed since you opened it: reload it, or save your changes over it with this version";
    expect(refused(409, { error: "changed elsewhere", version: "v2", message })).toEqual({
      kind: "changed",
      title: "Couldn't save the team: it changed since you opened it",
      detail: `The Docs team was changed somewhere else after you opened it. Copy anything you want to keep, then reload to see the team as it is now. Agentry said: "${message}"`,
    });
  });

  test("cut short: saved, and saving again with the version given finishes the tasks", () => {
    const error = "saved the Writing team, but 1 task still needs moving: save again to finish (task-1: no)";
    expect(refused(409, { error, version: "v3" })).toEqual({
      kind: "cut short",
      version: "v3",
      title: "Saved the Writing team, but some of its tasks haven't moved yet",
      detail: `Save again to finish moving them. Agentry said: "${error}"`,
    });
  });

  test("missing profiles: the offer Create team makes", () => {
    expect(refused(409, { error: "needs profiles", missingProfiles: ["research-only"] })).toEqual({
      kind: "missing profiles",
      profiles: ["research-only"],
    });
  });

  test("anything else in the server's words, or ours when Agentry couldn't be reached", () => {
    expect(refused(500, undefined, "the log is unwritable")).toEqual({
      kind: "failed",
      title: "Couldn't save the team",
      detail: "the log is unwritable",
    });
    expect(whyNotSaved(new TypeError("Failed to fetch"), loaded, draft)).toEqual({
      kind: "failed",
      title: "Couldn't save the team",
      detail: "Couldn't reach Agentry. Check it's running, then try again.",
    });
  });
});

describe("an edited team's sentences", () => {
  const renamed = { ...loaded.agents[1], name: "author" };
  const added = row("k9", { name: "critic" });

  test("rowChange says a row is new, edited, or renamed from the name it had", () => {
    expect(rowChange(loaded, loaded.agents[0])).toEqual({});
    expect(rowChange(loaded, { ...loaded.agents[0], description: "Leads." })).toEqual({ mark: "edited", was: undefined });
    expect(rowChange(loaded, renamed)).toEqual({ mark: "edited", was: "writer" });
    expect(rowChange(loaded, added)).toEqual({ mark: "new" });
  });

  test("discardEditDetail counts each change once, in order, the first naming agents", () => {
    expect(discardEditDetail(loaded, { team: "Writing", agents: [loaded.agents[0], renamed, added] })).toBe(
      "The team renamed, 1 agent changed, 1 added and 1 removed. They aren't saved.",
    );
    expect(discardEditDetail(loaded, { team: "Docs", agents: [...loaded.agents, added, row("k8", { name: "indexer" })] })).toBe(
      "2 agents added. They aren't saved.",
    );
    expect(discardEditDetail(loaded, { team: " Writing ", agents: loaded.agents })).toBe("The team renamed. They aren't saved.");
  });

  test("editFooterLine says why Save changes is off, or what it is doing", () => {
    expect(editFooterLine(0, false, false)).toBe("No changes to save.");
    expect(editFooterLine(0, false, true)).toBe("");
    expect(editFooterLine(2, false, true)).toBe("2 things to fix, listed at the top.");
    expect(editFooterLine(0, true, true)).toBe("Saving the team…");
  });
});

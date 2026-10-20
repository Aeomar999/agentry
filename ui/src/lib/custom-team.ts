import { api, ApiError } from "@/api/client";
import type { Agent, Policy, TeamTemplate, TemplateAgent } from "@/api/types";
import { AGENT_COLORS } from "@/live/agent-colors";
import { shippedProfiles } from "./profiles";
import { teamOf } from "./teams";

// A team you build yourself on the Agents page (docs/design/custom-teams.md):
// the form's draft, how it starts (Blank or a copy of a template), every
// problem with it at once, and saving it in one request (POST /api/teams).
// The server checks the same rules (agents.CheckCustomTeam, addCustomTeam)
// but stops at the first problem; the words are the copy deck's.

// A row of the form: an agent of the team, as typed. key stays the same
// while the row's name changes, so problems and focus can find the row.
export type DraftAgent = TemplateAgent & { key: string };

export interface TeamDraft {
  team: string;
  agents: DraftAgent[];
}

// The fields of a row a problem can be under.
export type AgentField = "name" | "prompt" | "color" | "column" | "engine" | "profile";

// Where a problem shows: under Team name, under the Agents legend, or under
// one field of one row.
export type ProblemField = "team" | "agents" | { row: string; field: AgentField };

export interface Problem {
  field: ProblemField;
  message: string; // under the field
  summary: string; // the summary's link, "{where}: {what}"
}

// What the checks know besides the roster: the built-in templates (whose
// team names are taken), the policy's profiles, and the engines Agentry runs.
export interface TeamRules {
  templates: Pick<TeamTemplate, "team">[];
  policy: Pick<Policy, "profiles" | "defaults">;
  engines: string[];
}

export const MAX_NAME = 31;

const COLUMNS: string[] = ["", "review", "ship"];

let rows = 0;
const nextKey = () => `agent-${++rows}`;

// sameName compares names as the server does: trimmed, in any case.
const sameName = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase();

// blankTeam is a team of one coordinator with no prompt, so the server's
// default coordinator prompt is used. Blank suggests no team name.
export function blankTeam(roster: Agent[]): TeamDraft {
  const coordinator: DraftAgent = {
    key: nextKey(),
    name: freeName(
      "coordinator",
      roster.map((agent) => agent.name),
    ),
    description: "",
    coordinates: true,
    prompt: "",
    engine: "claude",
    model: "",
    profile: "",
    color: "",
    column: "",
  };
  return { team: "", agents: [coordinator] };
}

// copyTemplate is every agent of the template, prompts included, under the
// template's team name with a number, since a built-in's own team name is
// never free. Names you have already get a number, as the server's AddTeam
// gives them.
export function copyTemplate(template: TeamTemplate, roster: Agent[]): TeamDraft {
  const taken = roster.map((agent) => agent.name);
  const agents = template.agents.map((agent) => {
    const name = freeName(agent.name, taken);
    taken.push(name);
    return { ...agent, key: nextKey(), name };
  });
  return { team: freeTeamName(template.team, roster), agents };
}

// newAgent is the row + Add agent adds: no name yet, a colour no row has,
// Building, the first engine and the default profile. The first row of an
// empty team coordinates it, since there is nobody else to pick.
export function newAgent(draft: TeamDraft, engines: string[]): DraftAgent {
  const used = draft.agents.map((agent) => agent.color);
  return {
    key: nextKey(),
    name: "",
    description: "",
    coordinates: draft.agents.length === 0,
    prompt: "",
    engine: engines[0] ?? "claude",
    model: "",
    profile: "",
    color: AGENT_COLORS.find((color) => !used.includes(color)) ?? "",
    column: "",
  };
}

// changedAgents are the names of the rows that differ from how the team
// started: changed, added or removed. Starting again replaces them.
export function changedAgents(start: TeamDraft, now: TeamDraft): string[] {
  const same = (a: DraftAgent, b: DraftAgent) => (Object.keys(a) as (keyof DraftAgent)[]).every((field) => a[field] === b[field]);
  const changed = now.agents.filter((agent) => {
    const was = start.agents.find((other) => other.key === agent.key);
    return !was || !same(was, agent);
  });
  const removed = start.agents.filter((agent) => !now.agents.some((other) => other.key === agent.key));
  return [...changed, ...removed].map(shownName);
}

// freeName is name, or name-2, name-3… when it is taken, cut to fit MAX_NAME.
function freeName(name: string, taken: string[]): string {
  let free = name.slice(0, MAX_NAME);
  for (let n = 2; taken.some((other) => sameName(other, free)); n++) {
    const suffix = `-${n}`;
    free = name.slice(0, MAX_NAME - suffix.length) + suffix;
  }
  return free;
}

function freeTeamName(team: string, roster: Agent[]): string {
  const teams = roster.map(teamOf);
  let n = 2;
  while (teams.some((other) => sameName(other, `${team} ${n}`))) n++;
  return `${team} ${n}`;
}

// checkTeam is every problem with the draft, in screen order: the team
// name, the agents as a whole, then each row's fields. A profile policy.toml
// lacks but Agentry ships isn't one: the server offers to add it (the 409
// that missingProfilesOf reads), and missingProfiles lists them beforehand.
export function checkTeam(draft: TeamDraft, roster: Agent[], rules: TeamRules): Problem[] {
  const problems: Problem[] = [];
  const teamProblem = checkTeamName(draft.team, roster, rules.templates);
  if (teamProblem) problems.push({ field: "team", ...teamProblem });
  const agentsProblem = checkCoordinator(draft.agents);
  if (agentsProblem) problems.push({ field: "agents", ...agentsProblem });
  draft.agents.forEach((agent, i) => {
    for (const [field, check] of AGENT_CHECKS) {
      const problem = check(agent, { earlier: draft.agents.slice(0, i), roster, rules });
      if (problem) problems.push({ field: { row: agent.key, field }, ...problem });
    }
  });
  return problems;
}

type Words = Omit<Problem, "field">;

function checkTeamName(team: string, roster: Agent[], templates: TeamRules["templates"]): Words | undefined {
  team = team.trim();
  if (team === "") return { message: "Give the team a name.", summary: "Team name: give the team a name" };
  const yours = roster.map(teamOf).find((other) => sameName(other, team));
  if (yours) {
    return {
      message: `You have a team called ${yours} already. Pick another name.`,
      summary: `Team name: you have a team called ${yours} already`,
    };
  }
  const builtIn = templates.find((template) => sameName(template.team, team));
  if (builtIn) {
    return {
      message: `${builtIn.team} is a built-in team's name. Pick another, such as ${builtIn.team} 2.`,
      summary: `Team name: ${builtIn.team} is a built-in team's name`,
    };
  }
}

// checkCoordinator asks for exactly one coordinator. The panel's radio
// across rows can't check two, but a team made agent by agent can have them.
function checkCoordinator(agents: DraftAgent[]): Words | undefined {
  if (agents.length === 0) return { message: "Add an agent: it coordinates the team.", summary: "Agents: add an agent" };
  const coordinators = agents.filter((agent) => agent.coordinates).map(shownName);
  if (coordinators.length === 0) {
    return { message: "Pick one agent to coordinate.", summary: "Agents: pick one agent to coordinate" };
  }
  if (coordinators.length > 1) {
    const who = `${listed(coordinators, "and")} ${coordinators.length === 2 ? "both" : "all"} coordinate`;
    return { message: `${who}. Pick the one that takes the team's tasks.`, summary: `Agents: ${who}` };
  }
}

interface RowContext {
  earlier: DraftAgent[]; // the rows above this one
  roster: Agent[];
  rules: TeamRules;
}

type AgentCheck = (agent: DraftAgent, context: RowContext) => Words | undefined;

// The row's checks, one per field, in the order the open row shows them.
const AGENT_CHECKS: [AgentField, AgentCheck][] = [
  ["name", checkName],
  ["prompt", checkPrompt],
  ["color", checkColor],
  ["column", checkColumn],
  ["engine", checkEngine],
  ["profile", checkProfile],
];

// shownName is how the panel names a row's agent, as yet unnamed too.
export const shownName = (agent: DraftAgent) => agent.name.trim() || "New agent";

function checkName(agent: DraftAgent, { earlier, roster }: RowContext): Words | undefined {
  const name = agent.name.trim();
  if (name === "") return { message: "Give this agent a name.", summary: "New agent: give it a name" };
  if (!/^[a-z0-9-]+$/.test(name)) {
    return {
      message: "Use only lowercase letters, digits and dashes.",
      summary: `${name}: use only lowercase letters, digits and dashes`,
    };
  }
  if (!/^[a-z]/.test(name)) {
    return { message: "Start the name with a letter.", summary: `${name}: start the name with a letter` };
  }
  if (name.length > MAX_NAME) {
    return { message: `Use ${MAX_NAME} characters or fewer.`, summary: `${name}: use ${MAX_NAME} characters or fewer` };
  }
  if (earlier.some((other) => sameName(other.name, name))) {
    return {
      message: `Another agent in this team is called ${name}. Pick another name.`,
      summary: `${name} (second one): another agent in this team is called ${name}`,
    };
  }
  const yours = roster.find((other) => sameName(other.name, name));
  if (yours) {
    return {
      message: `You have an agent called ${name} already, in the ${teamOf(yours)} team. Pick another name.`,
      summary: `${name}: you have an agent called ${name} already, in ${teamOf(yours)}`,
    };
  }
}

// checkPrompt asks every agent but the coordinator for a prompt; the server
// would write one from the description, which says too little to work by.
function checkPrompt(agent: DraftAgent): Words | undefined {
  if (agent.coordinates || agent.prompt.trim() !== "") return;
  return {
    message: "Write a prompt: only the coordinator can do without one.",
    summary: `${shownName(agent)}: write a prompt`,
  };
}

function checkColor(agent: DraftAgent): Words | undefined {
  const color = agent.color.trim();
  if (color === "" || AGENT_COLORS.includes(color)) return;
  return {
    message: `There's no colour called ${color}. Pick one of the swatches.`,
    summary: `${shownName(agent)}: there's no colour called ${color}`,
  };
}

function checkColumn(agent: DraftAgent): Words | undefined {
  const column = agent.column.trim();
  if (COLUMNS.includes(column)) return;
  return {
    message: `Its work can't count as ${column}. Pick Building, Review or Shipping.`,
    summary: `${shownName(agent)}: its work can't count as ${column}`,
  };
}

function checkEngine(agent: DraftAgent, { rules }: RowContext): Words | undefined {
  const engine = agent.engine.trim();
  if (rules.engines.includes(engine)) return;
  if (engine === "") return { message: `Pick an engine: ${listed(rules.engines)}.`, summary: `${shownName(agent)}: pick an engine` };
  return {
    message: `Agentry has no engine called ${engine}. Pick ${listed(rules.engines)}.`,
    summary: `${shownName(agent)}: Agentry has no engine called ${engine}`,
  };
}

// checkProfile refuses only a profile neither policy.toml has nor Agentry
// ships, since only you can write that one.
function checkProfile(agent: DraftAgent, { rules }: RowContext): Words | undefined {
  const profile = agent.profile.trim();
  if (profile === "" || inPolicy(profile, rules.policy) || shippedProfiles(rules.policy).has(profile)) return;
  return {
    message: `policy.toml has no profile called ${profile}. Pick one it has.`,
    summary: `${shownName(agent)}: policy.toml has no profile called ${profile}`,
  };
}

const inPolicy = (profile: string, policy: Pick<Policy, "profiles">) => policy.profiles.some((p) => p.name === profile);

// listed is "a", "a or b", "a, b or c", or with "and".
export function listed(words: string[], last = "or"): string {
  return words.length < 2 ? words.join("") : `${words.slice(0, -1).join(", ")} ${last} ${words[words.length - 1]}`;
}

// missingProfiles are the profiles the draft's agents use that policy.toml
// lacks and Agentry ships, in the order the rows first use them: the ones
// saving offers to add.
export function missingProfiles(draft: TeamDraft, policy: Pick<Policy, "profiles" | "defaults">): string[] {
  const shipped = shippedProfiles(policy);
  const missing: string[] = [];
  for (const agent of draft.agents) {
    const profile = agent.profile.trim();
    if (shipped.has(profile) && !inPolicy(profile, policy) && !missing.includes(profile)) missing.push(profile);
  }
  return missing;
}

// The panel's sentences, in the copy deck's words (docs/design/custom-teams/copy.md).

const counted = (n: number, one: string, many: string) => (n === 1 ? `1 ${one}` : `${n} ${many}`);
export const agentCount = (n: number) => counted(n, "agent", "agents");
export const thingCount = (n: number) => counted(n, "thing", "things");
export const problemCount = (n: number) => counted(n, "problem", "problems");

// footerLine is the line beside Create team: what it does, or why it won't yet.
export function footerLine(draft: TeamDraft, problems: number, saving: boolean): string {
  if (saving) return "Creating the team…";
  if (problems > 0) return `${thingCount(problems)} to fix, listed at the top.`;
  if (draft.agents.length === 1 && draft.agents[0].coordinates) return "Creates the team and its coordinator.";
  return `Creates the team and its ${agentCount(draft.agents.length)}.`;
}

// summaryTitle heads the summary of why Create team didn't create the team.
export const summaryTitle = (problems: number) =>
  problems > 0 ? `Couldn't create the team: ${thingCount(problems)} to fix` : "Couldn't create the team";

// missingLine says, under the agents, which profiles creating the team offers to add.
export function missingLine(draft: TeamDraft, missing: string[]): string {
  const users = draft.agents.filter((agent) => missing.includes(agent.profile.trim())).length;
  const who = users === 1 ? "1 agent uses" : `${users} agents use`;
  return `${who} ${listed(missing, "and")}, which policy.toml doesn't have. Create team offers to add ${missing.length === 1 ? "it" : "them"}.`;
}

// replaceQuestion asks before Start from replaces the agents you changed,
// with a template's (to) or a blank team (none).
export function replaceQuestion(changed: string[], to?: Pick<TeamTemplate, "team">): string {
  const yours = changed.length === 1 ? "your changed agent" : `your ${changed.length} changed agents`;
  return `Replace ${yours} with ${to ? `${to.team}'s` : "a blank team"}?`;
}

// replaceDetail says under the question what replacing does; from is the
// template the agents were copied from, if any.
export function replaceDetail(changed: string[], to?: Pick<TeamTemplate, "team" | "agents">, from?: Pick<TeamTemplate, "team">): string {
  if (!to) return "A blank team has one agent, its coordinator.";
  return `${to.team}'s ${to.agents.length} agents replace ${from ? `${from.team}'s` : "yours"}; your changes to ${listed(changed, "and")} go.`;
}

// What the panel's live region says when a row goes, comes back, or every
// row is replaced by a template's or a blank team's.
export const removedNotice = (agent: DraftAgent) => `${shownName(agent)} removed. Undo is next.`;
export const backNotice = (agent: DraftAgent) => `${shownName(agent)} is back.`;
export const replacedNotice = (to: Pick<TeamTemplate, "team" | "agents">) => `Replaced with ${to.team}'s ${to.agents.length} agents.`;
export const blankNotice = "Replaced with a blank team: one agent, its coordinator.";

// whyNotCreated is a refusal in words: the server's own, or ours when
// Agentry couldn't be reached at all.
export function whyNotCreated(failure: unknown): string {
  if (failure instanceof ApiError || !(failure instanceof TypeError)) return (failure as Error).message;
  return "Couldn't reach Agentry. Check it's running, then try again.";
}

// addCustomTeam adds the team as drafted, trimmed. With policyHash, as for
// addTeam, it also adds the shipped profiles policy.toml lacks.
export function addCustomTeam(draft: TeamDraft, policyHash?: string): Promise<unknown> {
  const agents = draft.agents.map((agent) => ({
    name: agent.name.trim(),
    description: agent.description.trim(),
    coordinates: agent.coordinates,
    prompt: agent.prompt.trim(),
    engine: agent.engine.trim(),
    model: agent.model.trim(),
    profile: agent.profile.trim(),
    color: agent.color.trim(),
    column: agent.column.trim(),
  }));
  const team = { team: draft.team.trim(), agents };
  return api("POST", "/api/teams", policyHash === undefined ? team : { ...team, addMissingProfiles: true, hash: policyHash });
}

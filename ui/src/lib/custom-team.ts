import { api, ApiError } from "@/api/client";
import type { Agent, Policy, TeamTemplate, TemplateAgent } from "@/api/types";
import { AGENT_COLORS } from "@/live/agent-colors";
import { shippedProfiles } from "./profiles";
import { missingProfilesOf, teamOf, teamsOf } from "./teams";

// A team you build yourself on the Agents page (docs/design/custom-teams.md):
// the form's draft, how it starts (Blank, a copy of a template, or a team
// you have, to edit), every problem with it at once, and saving it in one
// request (POST /api/teams, or PUT /api/teams/{team} for an edit). The
// server checks the same rules (agents.CheckCustomTeam, addCustomTeam) but
// stops at the first problem for a new team; the words are the copy deck's.

// A row of the form: an agent of the team, as typed. key stays the same
// while the row's name changes, so problems and focus can find the row.
export type DraftAgent = TemplateAgent & { key: string };

export interface TeamDraft {
  team: string;
  agents: DraftAgent[];
  copiedFrom?: string; // a copy's built-in template, whose features the team keeps
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
  removed?: NeededBy; // a removed agent unfinished tasks still need, for its stub
}

// A team you have, as the edit panel loaded it: the draft it starts from,
// whose rows keep the names they had then, and the team's version, which
// saving checks so a change made elsewhere meanwhile isn't lost.
export interface LoadedTeam extends TeamDraft {
  version: string;
}

// What the checks know besides the roster: the built-in templates (whose
// team names are taken), the policy's profiles, and the engines Agentry runs.
export interface TeamRules {
  templates: Pick<TeamTemplate, "team" | "userMade">[]; // a built-in one's team name is never free
  policy: Pick<Policy, "profiles" | "defaults">;
  engines: string[];
}

export const MAX_NAME = 31;

const COLUMNS: string[] = ["", "review", "ship"];

let rows = 0;
const nextKey = () => `agent-${++rows}`;

// sameName compares names as the server does: trimmed, in any case.
export const sameName = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase();

// blankTeam is a team of one coordinator with no prompt, so the server's
// default coordinator prompt is used, in a colour no roster agent has (the
// first colour when every one is taken). Blank suggests no team name.
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
    color: freeColor(roster.map((agent) => agent.color)) ?? AGENT_COLORS[0],
    column: "",
  };
  return { team: "", agents: [coordinator] };
}

// copyTemplate is every agent of the template, prompts included, under the
// template's team name: with a number for a built-in one, whose own team
// name is never free, and for one of yours when you have a team of its
// name. Names you have already get a number, as the server's AddTeam
// gives them. It was copied from the built-in template, or from the one
// yours is based on, if it is.
export function copyTemplate(template: TeamTemplate, roster: Agent[]): TeamDraft {
  const taken = roster.map((agent) => agent.name);
  const agents = template.agents.map((agent) => {
    const name = freeName(agent.name, taken);
    taken.push(name);
    return { ...agent, key: nextKey(), name };
  });
  const copiedFrom = template.userMade ? template.basedOn : template.id;
  return { team: freeTeamName(template.team, roster, template.userMade), agents, copiedFrom };
}

// loadTeam is the team named, in any case, as a draft to edit: its agents,
// the coordinator first, each in a row of its own. A team you don't have
// has no agents.
export function loadTeam(roster: Agent[], team: string): LoadedTeam {
  const found = teamsOf(roster).find((group) => sameName(group.team, team));
  const agents = (found?.agents ?? []).map((agent): DraftAgent => ({
    key: nextKey(),
    name: agent.name,
    description: agent.description,
    coordinates: agent.coordinates,
    prompt: agent.prompt,
    engine: agent.engine,
    model: agent.model,
    profile: agent.profile,
    color: agent.color,
    column: agent.column,
  }));
  return { team: found?.team ?? team, agents, version: found?.agents[0]?.teamVersion ?? "" };
}

// newAgent is the row + Add agent adds: no name yet, a colour no row has,
// Building, the first engine and the default profile. The first row of an
// empty team coordinates it, since there is nobody else to pick.
export function newAgent(draft: TeamDraft, engines: string[]): DraftAgent {
  return {
    key: nextKey(),
    name: "",
    description: "",
    coordinates: draft.agents.length === 0,
    prompt: "",
    engine: engines[0] ?? "claude",
    model: "",
    profile: "",
    color: freeColor(draft.agents.map((agent) => agent.color)) ?? "",
    column: "",
  };
}

// changedAgents are the names of the rows that differ from how the team
// started: changed, added or removed. Starting again replaces them.
export function changedAgents(start: TeamDraft, now: TeamDraft): string[] {
  const changed = now.agents.filter((agent) => {
    const was = start.agents.find((other) => other.key === agent.key);
    return !was || !sameAgent(was, agent);
  });
  const removed = start.agents.filter((agent) => !now.agents.some((other) => other.key === agent.key));
  return [...changed, ...removed].map(shownName);
}

// rowChange is how a row differs from the team as loaded, until saved: a
// new agent, or an edited one, with the name it had when renamed and
// whether its prompt changed.
export function rowChange(loaded: TeamDraft, agent: DraftAgent): { mark?: "new" | "edited"; was?: string; promptChanged?: boolean } {
  const was = loaded.agents.find((row) => row.key === agent.key);
  if (!was) return { mark: "new" };
  if (sameAgent(was, agent)) return {};
  return {
    mark: "edited",
    was: was.name !== agent.name.trim() ? was.name : undefined,
    promptChanged: was.prompt !== agent.prompt || undefined,
  };
}

const sameAgent = (a: DraftAgent, b: DraftAgent) => (Object.keys(a) as (keyof DraftAgent)[]).every((field) => a[field] === b[field]);

// unsaved says whether closing would lose something: the team's name or
// an agent differs from how the team started. Typing a change back undoes it.
export const unsaved = (start: TeamDraft, now: TeamDraft) => now.team.trim() !== start.team.trim() || changedAgents(start, now).length > 0;

// Copy a template picked before the templates came: the first of them, once they do.
export const FIRST_TEMPLATE = "*";

// templateToCopy is the template a copy starts from, or none for Blank or an id that names none.
export const templateToCopy = (templates: TeamTemplate[], id: string) =>
  id === FIRST_TEMPLATE ? templates[0] : templates.find((template) => template.id === id);

// freeColor is the first agent colour not in used, or none when all are.
const freeColor = (used: string[]) => AGENT_COLORS.find((color) => !used.includes(color));

// freeName is name, or name-2, name-3… when it is taken, cut to fit MAX_NAME.
function freeName(name: string, taken: string[]): string {
  let free = name.slice(0, MAX_NAME);
  for (let n = 2; taken.some((other) => sameName(other, free)); n++) {
    const suffix = `-${n}`;
    free = name.slice(0, MAX_NAME - suffix.length) + suffix;
  }
  return free;
}

function freeTeamName(team: string, roster: Agent[], mayBeFree: boolean): string {
  const teams = roster.map(teamOf);
  if (mayBeFree && !teams.some((other) => sameName(other, team))) return team;
  let n = 2;
  while (teams.some((other) => sameName(other, `${team} ${n}`))) n++;
  return `${team} ${n}`;
}

// checkTeam is every problem with the draft, in screen order: the team
// name, the agents as a whole, then each row's fields. A profile policy.toml
// lacks but Agentry ships isn't one: the server offers to add it (the 409
// that missingProfilesOf reads), and missingProfiles lists them beforehand.
// Editing a team (editing, as loaded), its own name, in any case, and its
// agents' names, whichever rows have them now, are free.
export function checkTeam(draft: TeamDraft, roster: Agent[], rules: TeamRules, editing?: TeamDraft): Problem[] {
  if (editing) roster = roster.filter((agent) => teamOf(agent) !== editing.team);
  const problems: Problem[] = [];
  const teamProblem = editing && sameName(draft.team, editing.team) ? undefined : checkTeamName(draft.team, roster, rules.templates);
  if (teamProblem) problems.push({ field: "team", ...teamProblem });
  const agentsProblem = checkCoordinator(draft.agents);
  if (agentsProblem) problems.push({ field: "agents", ...agentsProblem });
  draft.agents.forEach((agent, i) => {
    for (const [field, check] of AGENT_CHECKS) {
      const kept = editing?.agents.some((row) => row.key === agent.key) ?? false;
      const problem = check(agent, { earlier: draft.agents.slice(0, i), kept, roster, rules });
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
  const builtIn = templates.find((template) => !template.userMade && sameName(template.team, team));
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
  kept: boolean; // the row is an agent the team has, being edited
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
// It writes the default coordinator prompt only for a new agent, so a
// coordinator the team has keeps needing one.
function checkPrompt(agent: DraftAgent, { kept }: RowContext): Words | undefined {
  if (agent.prompt.trim() !== "" || (agent.coordinates && !kept)) return;
  if (agent.coordinates) {
    return {
      message: "Write a prompt: Agentry's coordinator prompt is only for a new agent.",
      summary: `${shownName(agent)}: write a prompt`,
    };
  }
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

// summaryTitle heads the summary of why Create team didn't create the
// team, or Save changes didn't save it (editing).
export function summaryTitle(problems: number, editing = false): string {
  const title = editing ? "Couldn't save the team" : "Couldn't create the team";
  return problems > 0 ? `${title}: ${thingCount(problems)} to fix` : title;
}

// missingLine says, under the agents, which profiles saving the team
// (action: Create team, or Save changes) offers to add.
export function missingLine(draft: TeamDraft, missing: string[], action = "Create team"): string {
  const users = draft.agents.filter((agent) => missing.includes(agent.profile.trim())).length;
  const who = users === 1 ? "1 agent uses" : `${users} agents use`;
  return `${who} ${listed(missing, "and")}, which policy.toml doesn't have. ${action} offers to add ${missing.length === 1 ? "it" : "them"}.`;
}

// routingLine is the line other coordinators read about the team, as the
// server writes it in their brief's Other teams (coordinator/brief.go): no
// colon when there is no description. Placeholders stand in for a name or
// coordinator not typed yet.
export function routingLine(team: string, coordinator: string, description: string): string {
  const line = `- ${team.trim() || "(team name)"}, coordinated by ${coordinator.trim() || "(coordinator)"}`;
  return description.trim() === "" ? line : `${line}: ${description.trim()}`;
}

// The hints that warn, without stopping a save, that work won't find its way.
export const noTeamDescription = "Other coordinators won't know what to send this team.";
export const noAgentDescription = "Without a description, the coordinator won't know what to hand it.";
export const onlyCoordinator = "Its coordinator hands work out rather than doing it: add the agents it hands work to.";
export const noCoordinator = "Make one agent the coordinator first: this is its description.";
export const builtInDescription = (template: string) => `Other coordinators read the ${template} template's description instead.`;

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

// While what a new team needs loads, and if it didn't.
export const loadingAgents = "Loading engines and profiles…";
export const loadingTemplates = "Loading templates…";
export const loadFailed = "Couldn't load what a new team needs";

// What closing asks when it would lose the new team.
export const discardQuestion = "Discard this new team?";
export const discardDetail = "It hasn't been created yet.";

// The Edit panel's sentences: its title, while the team loads, if it
// didn't, and when there is no team by that name.
export const editTitle = (team: string) => `Edit the ${team} team`;
export const loadingTeam = (team: string) => `Loading the ${team} team…`;
export const teamLoadFailed = "Couldn't load the team";
export const notFoundTitle = (team: string) => `Couldn't find the ${team} team`;
export const notFoundDetail =
  "It may have been renamed or removed since this link was made. Its agents, if any are left, are on the Agents page.";

// What the live region says when Edit's remove marks a row to go.
export const removingNotice = (agent: string) => `${agent} will be removed when you save. Undo is next.`;

// editFooterLine is the line beside Save changes: why it is off, or what it is doing.
export function editFooterLine(problems: number, saving: boolean, changed: boolean): string {
  if (saving) return "Saving the team…";
  if (problems > 0) return `${thingCount(problems)} to fix, listed at the top.`;
  return changed ? "" : "No changes to save.";
}

// What closing asks when it would lose your changes to a team: each agent
// counts once, as edited, new or removed.
export const discardEditQuestion = (team: string) => `Discard your changes to ${team}?`;

export function discardEditDetail(loaded: TeamDraft, draft: TeamDraft): string {
  const kept = (agent: DraftAgent) => loaded.agents.some((row) => row.key === agent.key);
  const counts = [
    [draft.agents.filter((agent) => kept(agent) && rowChange(loaded, agent).mark).length, "changed"],
    [draft.agents.filter((agent) => !kept(agent)).length, "added"],
    [loaded.agents.filter((row) => !draft.agents.some((agent) => agent.key === row.key)).length, "removed"],
  ] as const;
  // The first count names what it counts: "2 agents added", then "1 removed".
  const agents = counts.filter(([n]) => n > 0).map(([n, what], i) => `${i === 0 ? agentCount(n) : n} ${what}`);
  const changes = [...(draft.team.trim() !== loaded.team.trim() ? ["the team renamed"] : []), ...agents];
  const said = listed(changes, "and");
  return `${said.charAt(0).toUpperCase()}${said.slice(1)}. They aren't saved.`;
}

// What Save changes refused because the team changed elsewhere asks before
// reloading it over your changes.
export const reloadQuestion = "Reload and lose your changes?";

// openQuestion is what following a link out of the panel asks with unsaved
// changes: to an agent's page, or to a task. Nothing for any other page.
export function openQuestion(path: string): { question: string; detail: string } | undefined {
  const [, page, name] = path.split("/");
  if (!name) return;
  const named = decodeURIComponent(name);
  if (page === "agents")
    return { question: `Open ${named}'s page and discard your changes?`, detail: "Save first to keep them; the page shows what is saved." };
  if (page === "tasks") return { question: `Open ${named} and discard your changes?`, detail: "" };
}

// What Remove team asks, and why it is off for your only team.
export const removeTeamQuestion = (team: string, agents: number) => `Remove the ${team} team and its ${agentCount(agents)}?`;
export const removeTeamDetail = (changed: boolean) =>
  changed ? "Their prompts, what they learned and your unsaved changes go too." : "Their prompts and what they learned go too.";
export const onlyTeamReason = "Can't remove your only team: tasks need its coordinator.";

// How the server words a team that unfinished tasks still need.
const TEAM_NEEDED = /^.+ team is still needed by (.+?): finish those first/;

// tasksNeeding are the tasks a refused removal of a team names.
export const tasksNeeding = (refusal: string) => TEAM_NEEDED.exec(refusal)?.[1].split(", ") ?? [];

const unreachable = "Couldn't reach Agentry. Check it's running, then try again.";

// whyNotCreated is a refusal in words: the server's own, or ours when
// Agentry couldn't be reached at all.
export function whyNotCreated(failure: unknown): string {
  if (failure instanceof ApiError || !(failure instanceof TypeError)) return (failure as Error).message;
  return unreachable;
}

// What fetch's TypeError says when the server can't be reached: Chrome's,
// Firefox's and Safari's words.
const NETWORK_FAILURES = [/^Failed to fetch/, /^NetworkError when attempting to fetch resource/, /^Load failed$/];

// whyNotLoaded is a failed read in words, as whyNotCreated: a read keeps
// only its error's message, so an unreachable server is known by its words.
export const whyNotLoaded = (error: string): string => (NETWORK_FAILURES.some((words) => words.test(error)) ? unreachable : error);

// trimmed is a row as the server takes it: without its key, trimmed.
const trimmed = (agent: DraftAgent) => ({
  name: agent.name.trim(),
  description: agent.description.trim(),
  coordinates: agent.coordinates,
  prompt: agent.prompt.trim(),
  engine: agent.engine.trim(),
  model: agent.model.trim(),
  profile: agent.profile.trim(),
  color: agent.color.trim(),
  column: agent.column.trim(),
});

// offering asks, with policyHash, as addTeam does, for the shipped
// profiles policy.toml lacks too.
const offering = <T extends object>(request: T, policyHash?: string) =>
  policyHash === undefined ? request : { ...request, addMissingProfiles: true, hash: policyHash };

// addCustomTeam adds the team as drafted, trimmed, and the built-in
// template it was copied from, if it was. With policyHash, as for addTeam,
// it also adds the shipped profiles policy.toml lacks.
export function addCustomTeam(draft: TeamDraft, policyHash?: string): Promise<unknown> {
  const { copiedFrom } = draft;
  const team = { team: draft.team.trim(), agents: draft.agents.map(trimmed), ...(copiedFrom && { copiedFrom }) };
  return api("POST", "/api/teams", offering(team, policyHash));
}

// editCustomTeam saves the team as edited from loaded, in one request:
// each row with the name it had when loaded ("" for a new one), and the
// rows removed left out, which removes their agents. The reply is the
// team's name and version after.
export function editCustomTeam(loaded: LoadedTeam, draft: TeamDraft, policyHash?: string): Promise<{ team: string; version: string }> {
  const agents = draft.agents.map((agent) => ({ was: loaded.agents.find((row) => row.key === agent.key)?.name ?? "", ...trimmed(agent) }));
  const edit = { version: loaded.version, team: draft.team.trim(), agents };
  return api("PUT", `/api/teams/${encodeURIComponent(loaded.team)}`, offering(edit, policyHash));
}

// A problem with an edit, as PUT /api/teams/{team} lists it: an agent's,
// with its row in the request, or the team's, with the agent removed that
// a task still needs.
interface ServerProblem {
  row?: number;
  field: string;
  agent?: string;
  error: string;
}

// The server's row fields the panel shows a problem under; was is the
// row's name too, the one it had.
const SHOWN_FIELDS: Record<string, AgentField> = {
  name: "name",
  was: "name",
  prompt: "prompt",
  color: "color",
  column: "column",
  engine: "engine",
  profile: "profile",
};

// The unfinished tasks that refused an agent's removal; turns when only the
// turns it is taking on them did, so they need only end.
export interface NeededBy {
  agent: string;
  tasks: string[];
  turns?: boolean;
}

// How the rules word a removed agent that unfinished tasks need, and one
// that only its turns on them need.
const STILL_NEEDED = /^.+ is still needed by (.+): finish those first, or keep .+$/;
const TURNS_RUNNING = /^.+ has (?:a turn|turns) running on (.+): wait for (?:it|them) to end, or keep .+$/;

// problemsOfEdit puts the server's problems with an edit of loaded where
// the panel shows them: a row's under its field, by the row's key; the team
// name's under Team name; the rest, a removed agent's too, under the Agents
// legend, so none is dropped. A row's field the panel has no place for
// goes there too, naming the row.
export function problemsOfEdit(problems: ServerProblem[], draft: TeamDraft, loaded: TeamDraft): Problem[] {
  const names = [...draft.agents, ...loaded.agents].map((agent) => agent.name.trim());
  return problems.map((problem) => {
    const said = asSentence(problem.error, problem.agent ? [...names, problem.agent] : names);
    const agent = problem.row === undefined ? undefined : draft.agents[problem.row];
    const field = agent && SHOWN_FIELDS[problem.field];
    if (agent && field) return { field: { row: agent.key, field }, message: said, summary: `${shownName(agent)}: ${problem.error}` };
    if (agent) return { field: "agents", message: `${shownName(agent)}: ${said}`, summary: `${shownName(agent)}: ${problem.error}` };
    if (problem.field === "team") return { field: "team", message: said, summary: `Team name: ${problem.error}` };
    if (problem.field === "removed" && problem.agent) return removedProblem(problem.agent, problem.error, said);
    return { field: "agents", message: said, summary: `Agents: ${problem.error}` };
  });
}

// removedProblem is a problem with removing an agent: when unfinished tasks
// need it, in the stub's words, naming them; else in the server's (said).
function removedProblem(agent: string, error: string, said: string): Problem {
  const running = TURNS_RUNNING.exec(error);
  const tasks = (running ?? STILL_NEEDED.exec(error))?.[1].split(", ") ?? [];
  if (tasks.length === 0) return { field: "agents", message: said, summary: `${agent}: ${error}` };
  const by = listed(tasks, "and");
  const it = tasks.length === 1 ? "it" : "them";
  if (running) {
    const turns = tasks.length === 1 ? "a turn" : "turns";
    return {
      field: "agents",
      message: `${agent} has ${turns} running on ${by}. Wait for ${it} to end, or undo to keep ${agent}.`,
      summary: `${agent}: ${turns} running on ${by}`,
      removed: { agent, tasks, turns: true },
    };
  }
  return {
    field: "agents",
    message: `${agent} is still needed by ${by}. Finish ${it} first, or undo to keep ${agent}.`,
    summary: `${agent}: still needed by ${by}`,
    removed: { agent, tasks },
  };
}

// asSentence makes the server's reason a sentence: a capital and a full
// stop. A reason starting with one of the agent names stays as it is,
// since agent names are lowercase.
function asSentence(reason: string, names: string[]): string {
  const first = /^[a-z0-9-]+/.exec(reason)?.[0];
  const start = first && names.includes(first) ? reason : reason.charAt(0).toUpperCase() + reason.slice(1);
  return start + (/[.!?]$/.test(reason) ? "" : ".");
}

// Why Save changes didn't save, and what the panel does about it:
// - problems: show them, as checkTeam's, keeping everything typed.
// - changed: the team changed elsewhere since it was loaded. Reload it
//   before saving: saving over it would delete agents added there, so the
//   server's words, which offer that, aren't shown.
// - cut short: the team is saved, but some tasks haven't followed it yet.
//   Reload the team (its rows have their new names now), then save it
//   again unchanged (saveAgain) to finish them.
// - missing profiles: the offer to add them, as Create team's.
// - failed: anything else, in the server's words or ours.
export type EditRefusal =
  | { kind: "problems"; problems: Problem[] }
  | { kind: "changed"; title: string; detail: string }
  | { kind: "cut short"; title: string; detail: string }
  | { kind: "missing profiles"; profiles: string[] }
  | { kind: "failed"; title: string; detail: string };

// whyNotSaved reads a refused editCustomTeam of draft, loaded as loaded.
export function whyNotSaved(failure: unknown, loaded: LoadedTeam, draft: TeamDraft): EditRefusal {
  const profiles = missingProfilesOf(failure);
  if (profiles) return { kind: "missing profiles", profiles };
  const body = failure instanceof ApiError ? failure.body : undefined;
  if (Array.isArray(body?.problems)) return { kind: "problems", problems: problemsOfEdit(body.problems as ServerProblem[], draft, loaded) };
  if (typeof body?.version === "string" && body.error === "changed elsewhere") {
    return {
      kind: "changed",
      title: "Couldn't save the team: it changed since you opened it",
      detail: `The ${loaded.team} team was changed somewhere else after you opened it. Copy anything you want to keep, then reload to see the team as it is now.`,
    };
  }
  if (typeof body?.version === "string") {
    return {
      kind: "cut short",
      title: `Saved the ${draft.team.trim()} team, but some of its tasks haven't moved yet`,
      detail: `Save again to finish moving them.${quoted((failure as Error).message)}`,
    };
  }
  return { kind: "failed", title: summaryTitle(0, true), detail: whyNotCreated(failure) };
}

// quoted is the server's own words after ours, when it sent some.
const quoted = (words: unknown) => (typeof words === "string" && words !== "" ? ` Agentry said: "${words}"` : "");

// saveAgain finishes moving the tasks a save cut short: the team saved
// unchanged, reloaded first so its rows have the names they have now.
export const saveAgain = (reloaded: LoadedTeam) => editCustomTeam(reloaded, reloaded);

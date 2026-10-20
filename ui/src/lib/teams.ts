import { api, ApiError } from "@/api/client";
import type { Agent, TeamTemplate } from "@/api/types";
import { useResource } from "@/live/use-resource";

// Teams of agents (internal/agents): each has a coordinator, which hands its
// tasks only to the team's own agents. An agent with no team is in Software.

export const DEFAULT_TEAM = "Software";

// The feature of the team whose sessions suggest features to the Ideas page.
export const IDEAS = "ideas";

export const teamOf = (agent: Agent) => agent.team || DEFAULT_TEAM;

// roleOf is the agent's part in its team, in words: coordinator, or what its
// column's work counts as; "" when it has none, or when the word is only its
// own name again ("reviewer, Software · reviewer" reads as a glitch).
export function roleOf(agent: Agent): string {
  const role = agent.coordinates ? "coordinator" : { "": "", review: "reviewer", ship: "deployer" }[agent.column];
  return role === agent.name ? "" : role;
}

// templatesWith is the ids of the templates that turn the feature on: the
// built-in one that lists it, and yours based on it, whose teams keep it.
export function templatesWith(templates: TeamTemplate[], feature: string): string[] {
  return templates.filter((t) => t.features.includes(feature)).map((t) => t.id);
}

// teamMadeFrom is the team made from one of the templates, if you have one:
// the team of a coordinator one of them made, as the server decides, so a
// second copy ("Visionaries 2") counts and a team you made and named as the
// template doesn't. The first such team when you have several.
export function teamMadeFrom(agents: Agent[], templates: string[]): string | undefined {
  const lead = agents.find((agent) => agent.coordinates && agent.template !== "" && templates.includes(agent.template));
  return lead && teamOf(lead);
}

export interface TeamWith {
  ready: boolean; // the templates and the agents are read, so team says what you have
  error: string; // why the templates or the agents couldn't be read, the last time they were
  agentsError: string; // why the agents couldn't be read, the last time they were
  template?: TeamTemplate; // the built-in template that turns the feature on, which adding the team adds
  templates: string[]; // the ids of every template that turns it on, yours included
  team?: string; // the team you have made from one of them, by its name now
  reload: () => Promise<void>; // reads the agents again, returning once they are shown
}

// useTeamWith finds the team that turns a feature on: the built-in template
// that lists it, and the team you made from it or from a template of yours
// based on it, whatever it is called.
export function useTeamWith(feature: string): TeamWith {
  const templates = useResource<TeamTemplate[]>("/api/teams/templates");
  const agents = useResource<Agent[]>("/api/agents");
  const template = templates.data?.find((t) => !t.userMade && t.features.includes(feature));
  const withFeature = templatesWith(templates.data ?? [], feature);
  return {
    ready: templates.data !== undefined && agents.data !== undefined,
    error: templates.error || agents.error,
    agentsError: agents.error,
    template,
    templates: withFeature,
    team: teamMadeFrom(agents.data ?? [], withFeature),
    reload: agents.reload,
  };
}

// teamsOf groups agents by team, in the order the first agent of each was
// made, the coordinator first in each.
export function teamsOf(agents: Agent[]): { team: string; agents: Agent[] }[] {
  const teams: { team: string; agents: Agent[] }[] = [];
  for (const agent of agents) {
    let group = teams.find((t) => t.team === teamOf(agent));
    if (!group) teams.push((group = { team: teamOf(agent), agents: [] }));
    group.agents.push(agent);
  }
  for (const group of teams) group.agents.sort((a, b) => Number(b.coordinates) - Number(a.coordinates));
  return teams;
}

// addTeam adds the team made from a template (GET /api/teams/templates).
// With policyHash, the version of the policy the page saw, it also adds the
// default profiles the team needs and policy.toml lacks; "" adds them to
// the file as it is then, which is safe since adding one only appends.
export function addTeam(template: string, policyHash?: string): Promise<unknown> {
  const request = policyHash === undefined ? { template } : { template, addMissingProfiles: true, hash: policyHash };
  return api("POST", "/api/teams", request);
}

// missingProfilesOf is the profiles a refused team needs, if that is why it was refused.
export function missingProfilesOf(failure: unknown): string[] | undefined {
  if (!(failure instanceof ApiError) || failure.status !== 409) return undefined;
  const missing = failure.body?.missingProfiles;
  return Array.isArray(missing) && missing.length > 0 ? missing.map(String) : undefined;
}

export function profilesNamed(profiles: string[]): string {
  if (profiles.length === 1) return `the ${profiles[0]} profile`;
  return `the ${profiles.slice(0, -1).join(", ")} and ${profiles[profiles.length - 1]} profiles`;
}

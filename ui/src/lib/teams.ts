import { api, ApiError } from "@/api/client";
import type { Agent } from "@/api/types";

// Teams of agents (internal/agents): each has a coordinator, which hands its
// tasks only to the team's own agents. An agent with no team is in Software.

export const DEFAULT_TEAM = "Software";

// The team whose sessions suggest features to the Ideas page.
export const VISIONARIES = "Visionaries";

export const teamOf = (agent: Agent) => agent.team || DEFAULT_TEAM;

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

import type { Agent } from "@/api/types";

// Teams of agents (internal/agents): each has a coordinator, which hands its
// tasks only to the team's own agents. An agent with no team is in Software.

export const DEFAULT_TEAM = "Software";

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

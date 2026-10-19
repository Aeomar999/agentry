import type { Agent } from "@/api/types";

// profileInForce says which policy profile judges an agent's calls, the same
// way wherever agents show: "read-mostly profile", or "read-mostly, overridden
// by allow-everything" while use_for_everyone puts another in its place.
export function profileInForce(agent: Agent): string {
  const effective = agent.effective;
  if (!effective) return `${agent.profile || "default"} profile`;
  return effective.overridden ? effective.text : `${effective.profile} profile`;
}

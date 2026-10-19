import type { Agent, EffectiveProfile, Policy } from "@/api/types";

// profileInForce says which policy profile judges an agent's calls, the same
// way wherever agents show: "read-mostly profile", or "read-mostly, overridden
// by allow-everything" while use_for_everyone puts another in its place.
export function profileInForce(agent: Agent): string {
  const effective = agent.effective;
  if (!effective) return `${agent.profile || "default"} profile`;
  return effective.overridden ? effective.text : `${effective.profile} profile`;
}

// effectiveProfile is the profile in force for an agent whose own profile is
// own ("" for the default), as the server works it out
// (policy.EffectiveProfile): for an agent being edited, before it is saved.
export function effectiveProfile(own: string, policy: Policy): EffectiveProfile {
  own ||= policy.defaultProfile;
  const everyone = policy.useForEveryone;
  if (everyone && everyone !== own) {
    return { profile: everyone, own, overridden: true, text: `${own}, overridden by ${everyone}` };
  }
  return { profile: own, own, overridden: false, text: own };
}

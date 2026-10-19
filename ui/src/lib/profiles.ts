import type { Agent, EffectiveProfile, Policy } from "@/api/types";

// profileInForce says which policy profile judges an agent's calls, the same
// way wherever agents show: "read-mostly profile", "read-mostly, overridden
// by allow-everything" while use_for_everyone puts another in its place, or
// "read-mostly (missing from policy.toml)" when the file has no such profile.
export function profileInForce(agent: Agent): string {
  const effective = agent.effective;
  if (!effective) return `${agent.profile || "default"} profile`;
  return effective.overridden || effective.missing ? effective.text : `${effective.profile} profile`;
}

// effectiveProfile is the profile in force for an agent whose own profile is
// own ("" for the default), as the server works it out
// (policy.EffectiveProfile): for an agent being edited, before it is saved.
export function effectiveProfile(own: string, policy: Policy): EffectiveProfile {
  own ||= policy.defaultProfile;
  const missing = !policy.profiles.some((profile) => profile.name === own);
  const shown = missing ? `${own} (missing from policy.toml)` : own;
  const everyone = policy.useForEveryone;
  if (everyone && everyone !== own) {
    return { profile: everyone, own, overridden: true, missing, text: `${shown}, overridden by ${everyone}` };
  }
  return { profile: own, own, overridden: false, missing, text: shown };
}

// deniedUntilBack says what a missing profile means for the agents that have
// it: the policy denies every call an agent makes with a profile it lacks,
// unless use_for_everyone puts another in its place.
export function deniedUntilBack(policy: Pick<Policy, "useForEveryone">): string {
  return policy.useForEveryone
    ? `${policy.useForEveryone} covers them while use_for_everyone is on; with it off, every call they make is denied until it's back.`
    : "Every call they make is denied until it's back.";
}

import { Button } from "@/components/atoms/Button";
import { AgentName } from "@/components/molecules/AgentName";
import { Banner } from "@/components/molecules/Banner";
import { Mono } from "@/components/molecules/Mono";
import { addDefaultProfile, turnOffUseForEveryone } from "@/api/client";
import type { Policy } from "@/api/types";
import { usePolicyChange } from "@/lib/policy-change";
import { notShipped, shippedProfiles } from "@/lib/profiles";

// use_for_everyone puts one profile in place of every agent's own. Wherever
// agents' profiles show, this says so, with the way to turn it off: the
// line is commented out, so each agent uses its own profile again. An agent
// whose own profile policy.toml lacks is denied every call from then on, so
// the notice names those agents, and offers to add a profile Agentry ships.
export function UseForEveryoneNotice({ policy, onChanged }: { policy: Policy; onChanged: () => void }) {
  const { busy, run } = usePolicyChange(onChanged);
  const profile = policy.useForEveryone;
  const shipped = shippedProfiles(policy);
  const missing = Object.entries(policy.missing ?? {}).sort(([a], [b]) => a.localeCompare(b));
  return (
    <Banner tone="info" title={`Every agent uses ${profile}; their own profiles are ignored`}>
      <div className="flex flex-wrap items-center gap-3">
        <span className="min-w-0 flex-1">
          policy.toml sets <Mono>use_for_everyone = "{profile}"</Mono>.{" "}
          {missing.length === 0
            ? "Turn it off and each agent uses its own profile again."
            : "Turn it off and each agent whose profile is in policy.toml uses it again."}
        </span>
        <Button size="sm" disabled={busy} onClick={() => run("Turned off use_for_everyone", () => turnOffUseForEveryone(policy.hash))}>
          Turn it off
        </Button>
      </div>
      {missing.length > 0 && (
        <ul className="mt-2 flex flex-col gap-2">
          {missing.map(([name, agents]) => (
            <li key={name} className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <span className="text-warning">Turning this off denies every call by</span>
              {agents.map((agent) => (
                <AgentName key={agent} name={agent} className="text-ink-primary" />
              ))}
              <span className="text-warning">
                until <Mono>{name}</Mono> is back.
              </span>
              {shipped.has(name) ? (
                <Button size="sm" disabled={busy} onClick={() => run(`Added profile ${name}`, () => addDefaultProfile(name, policy.hash))}>
                  Add it
                </Button>
              ) : (
                <span className="text-ink-muted">{notShipped}</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </Banner>
  );
}

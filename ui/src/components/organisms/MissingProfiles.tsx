import { Badge } from "@/components/atoms/Badge";
import { AgentName } from "@/components/molecules/AgentName";
import { Mono } from "@/components/molecules/Mono";
import { Panel } from "@/components/molecules/Panel";
import type { Policy } from "@/api/types";
import { deniedUntilBack, notShipped, shippedProfiles } from "@/lib/profiles";

// A profile agents have that policy.toml lacks: a hand edit can drop one
// that a change from the page couldn't. Whoever has it and what that means
// shows on its row; a default profile's row can add it back.

export function MissingProfileUsers({ agents, policy }: { agents: string[]; policy: Pick<Policy, "useForEveryone"> }) {
  return (
    <p className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-divider px-4 py-2 text-[12px] text-ink-muted">
      <span>Used by</span>
      {agents.map((agent) => (
        <AgentName key={agent} name={agent} className="text-ink-secondary" />
      ))}
      <span className="text-warning">{deniedUntilBack(policy)}</span>
    </p>
  );
}

// UnshippedMissingProfiles are the missing profiles Agentry doesn't ship, so
// it has nothing to add: you write them back, or give the agents another.
export function UnshippedMissingProfiles({ policy }: { policy: Policy }) {
  const shipped = shippedProfiles(policy);
  const missing = Object.entries(policy.missing ?? {})
    .filter(([name]) => !shipped.has(name))
    .sort(([a], [b]) => a.localeCompare(b));
  if (missing.length === 0) return null;
  return (
    <Panel flush title="Profiles agents need" aside={<>not in policy.toml</>}>
      <ul>
        {missing.map(([name, agents]) => (
          <li key={name} className="border-b border-divider last:border-b-0">
            <div className="flex flex-wrap items-center gap-3 px-4 py-2.5">
              <Mono className="text-[13px] text-ink-primary">{name}</Mono>
              <Badge variant="warning">missing</Badge>
              <span className="min-w-0 flex-1 text-[12px] text-ink-muted">{notShipped}</span>
            </div>
            <MissingProfileUsers agents={agents} policy={policy} />
          </li>
        ))}
      </ul>
    </Panel>
  );
}

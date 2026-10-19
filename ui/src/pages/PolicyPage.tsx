import { useBreadcrumb } from "@/components/templates/AdminShell";
import { PageHeader } from "@/components/molecules/PageHeader";
import { Panel } from "@/components/molecules/Panel";
import { Banner } from "@/components/molecules/Banner";
import { CodeViewer } from "@/components/molecules/CodeViewer";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { DecisionBadge } from "@/components/molecules/StatusBadges";
import { Mono } from "@/components/molecules/Mono";
import { AgentName } from "@/components/molecules/AgentName";
import { DefaultProfiles } from "@/components/organisms/DefaultProfiles";
import { PolicyHistory } from "@/components/organisms/PolicyHistory";
import { UnshippedMissingProfiles } from "@/components/organisms/MissingProfiles";
import { UseForEveryoneNotice } from "@/components/organisms/UseForEveryoneNotice";
import type { Policy, PolicyProfile } from "@/api/types";
import { useResource } from "@/live/use-resource";

type Rule = PolicyProfile["rules"][number] & { order: number };

const RULE_COLUMNS: Column<Rule>[] = [
  { header: "#", className: "w-10 text-right", render: (rule) => <Mono className="text-ink-faint">{rule.order}</Mono> },
  { header: "Decision", className: "w-24", render: (rule) => <DecisionBadge decision={rule.decision} /> },
  { header: "Rule", className: "w-72 max-md:w-auto", render: (rule) => <span className="font-medium text-ink-primary">{rule.name}</span> },
  {
    header: "Why",
    className: "w-full max-w-0 max-md:hidden",
    render: (rule) => (
      <p className="truncate text-ink-muted" title={rule.why}>
        {rule.why}
      </p>
    ),
  },
];

// The policy is a file you edit: ~/.agentry/policy.toml. This page shows
// the one in force; saving the file reloads it within two seconds. The
// changes Agentry can make for you are here too: adding and updating its
// default profiles, turning off use_for_everyone, restoring an earlier
// version. Each is made against the version shown, and logged.
export function PolicyPage() {
  useBreadcrumb({ label: "Policy" });
  const { data: policy, error, reload } = useResource<Policy>("/api/policy");

  return (
    <>
      <PageHeader
        title="Policy"
        meta={
          policy && (
            <>
              <Mono>{policy.path}</Mono>
              <span>·</span>
              <span>
                version <Mono>{policy.hash.slice(0, 8)}</Mono>
              </span>
            </>
          )
        }
      />
      <div className="flex flex-col gap-4 p-6">
        {error && <Banner title="Couldn't load the policy">{error}</Banner>}
        {policy?.problem && (
          <Banner title="The file has a mistake, so the version before it is still in force">
            <span className="font-mono text-[12px] whitespace-pre-wrap">{policy.problem}</span>
          </Banner>
        )}
        {policy?.useForEveryone && <UseForEveryoneNotice profile={policy.useForEveryone} hash={policy.hash} onChanged={reload} />}
        {policy && (
          <Banner tone="info" title="Edit the file to change the policy">
            Rules are read top to bottom and the first that matches decides. Save the file and it takes effect within two seconds.
          </Banner>
        )}
        {policy && <UnshippedMissingProfiles policy={policy} />}
        {policy && (
          <DefaultProfiles
            defaults={policy.defaults}
            missing={policy.missing ?? {}}
            useForEveryone={policy.useForEveryone}
            version={policy.version}
            hash={policy.hash}
            onChanged={reload}
          />
        )}
        {policy?.profiles.map((profile) => (
          <ProfilePanel key={profile.name} profile={profile} policy={policy} />
        ))}
        {policy && <PolicyHistory hash={policy.hash} onChanged={reload} />}
        {policy && (
          <Panel flush title={<Mono>policy.toml</Mono>} aside="The file as loaded">
            <CodeViewer text={policy.text} language="ini" />
          </Panel>
        )}
      </div>
    </>
  );
}

function ProfilePanel({ profile, policy }: { profile: PolicyProfile; policy: Policy }) {
  const inUse = policy.useForEveryone ? policy.useForEveryone === profile.name : true;
  const rules = profile.rules.map((rule, i) => ({ ...rule, order: i + 1 }));
  return (
    <Panel
      flush
      title={
        <>
          <Mono className="text-[13px]">{profile.name}</Mono>
          {policy.useForEveryone === profile.name && <span className="text-[11px] font-normal text-info">used by every agent</span>}
          {policy.defaultProfile === profile.name && <span className="text-[11px] font-normal text-ink-muted">default</span>}
          {!inUse && <span className="text-[11px] font-normal text-ink-faint">not in use</span>}
        </>
      }
      aside={
        <>
          otherwise <DecisionBadge decision={profile.otherwise} />
        </>
      }
    >
      <p className="border-b border-divider px-4 py-2.5 text-[13px] text-ink-secondary">{profile.why}</p>
      <ProfileUsers profile={profile.name} policy={policy} />
      {rules.length > 0 ? (
        <ResourceTable columns={RULE_COLUMNS} rows={rules} rowKey={(rule) => String(rule.order)} />
      ) : (
        <p className="px-4 py-2.5 text-[12px] text-ink-muted">No rules: every call is decided by otherwise.</p>
      )}
    </Panel>
  );
}

// ProfileUsers are the agents whose own profile this is: named by them, or
// default_profile for those that name none. Removing a profile agents name
// is refused, so this is who stops it.
function ProfileUsers({ profile, policy }: { profile: string; policy: Policy }) {
  const users = policy.users[profile] ?? [];
  const overridden = policy.useForEveryone !== "" && policy.useForEveryone !== profile;
  return (
    <p className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-divider px-4 py-2 text-[12px] text-ink-muted">
      {users.length === 0 ? (
        <span>No agent has this profile.</span>
      ) : (
        <>
          <span>{overridden ? "Agents' own profile, overridden for now:" : "Used by"}</span>
          {users.map((user) => (
            <span key={user.agent} className="inline-flex items-center gap-1">
              <AgentName name={user.agent} className="text-ink-secondary" />
              {user.followsDefault && <span className="text-ink-faint">(default_profile)</span>}
            </span>
          ))}
        </>
      )}
    </p>
  );
}

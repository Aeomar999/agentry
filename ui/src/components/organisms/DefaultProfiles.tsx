import { useState } from "react";
import { Badge, BadgeVariant } from "@/components/atoms/Badge";
import { Button } from "@/components/atoms/Button";
import { Diff } from "@/components/molecules/Diff";
import { Mono } from "@/components/molecules/Mono";
import { Panel } from "@/components/molecules/Panel";
import { addDefaultProfile, takeDefaultProfile, updateDefaultProfile } from "@/api/client";
import type { DefaultProfile } from "@/api/types";
import { usePolicyChange } from "@/lib/policy-change";

// The profiles this Agentry ships, against your file. A missing one can be
// added, an outdated one you never touched updated, and one you changed is
// never overwritten: you see yours against the default and choose.

const STATES: Record<DefaultProfile["state"], { badge: BadgeVariant; label: string }> = {
  missing: { badge: "warning", label: "missing" },
  outdated: { badge: "info", label: "outdated" },
  edited: { badge: "default", label: "yours" },
  current: { badge: "success", label: "up to date" },
};

export function DefaultProfiles({
  defaults,
  version,
  hash,
  onChanged,
}: {
  defaults: DefaultProfile[];
  version: string;
  hash: string;
  onChanged: () => void;
}) {
  return (
    <Panel flush title="Default profiles" aside={<>as Agentry {version} ships them</>}>
      <ul>
        {defaults.map((profile) => (
          <DefaultProfileRow key={profile.name} profile={profile} version={version} hash={hash} onChanged={onChanged} />
        ))}
      </ul>
    </Panel>
  );
}

function DefaultProfileRow({
  profile,
  version,
  hash,
  onChanged,
}: {
  profile: DefaultProfile;
  version: string;
  hash: string;
  onChanged: () => void;
}) {
  const { busy, run } = usePolicyChange(onChanged);
  const [kept, setKept] = useState(false); // you chose to keep your changed profile
  const state = STATES[profile.state];
  return (
    <li className="border-b border-divider last:border-b-0">
      <div className="flex flex-wrap items-center gap-3 px-4 py-2.5">
        <Mono className="text-[13px] text-ink-primary">{profile.name}</Mono>
        <Badge variant={state.badge}>{state.label}</Badge>
        <span className="min-w-0 flex-1 text-[12px] text-ink-muted">{summaryOf(profile, version, kept)}</span>
        {profile.state === "missing" && (
          <Button
            size="sm"
            disabled={busy}
            onClick={() => run(`Added profile ${profile.name}`, () => addDefaultProfile(profile.name, hash))}
          >
            Add this profile
          </Button>
        )}
        {profile.state === "outdated" && (
          <Button
            size="sm"
            disabled={busy}
            onClick={() => run(`Updated ${profile.name} to the ${version} default`, () => updateDefaultProfile(profile.name, hash))}
          >
            Update to the {version} default
          </Button>
        )}
        {profile.state === "edited" && !kept && (
          <>
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => setKept(true)}>
              Keep mine
            </Button>
            <Button
              size="sm"
              disabled={busy}
              onClick={() => run(`Replaced your ${profile.name} with the ${version} default`, () => takeDefaultProfile(profile.name, hash))}
            >
              Use the default
            </Button>
          </>
        )}
      </div>
      {profile.diff && !kept && <Diff text={profile.diff} className="border-t border-divider" />}
    </li>
  );
}

function summaryOf(profile: DefaultProfile, version: string, kept: boolean): string {
  switch (profile.state) {
    case "missing":
      return `Your file doesn't have it. Adding it appends the ${version} default, under a comment naming the version.`;
    case "outdated":
      return `The ${profile.shippedIn} default, which you never changed. ${version} ships a newer one:`;
    case "edited":
      return kept ? "You changed it, and keep your version." : "You changed it, so Agentry won't overwrite it. Yours against the default:";
    case "current":
      return `The ${version} default.`;
  }
}

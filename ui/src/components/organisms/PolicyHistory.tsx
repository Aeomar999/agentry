import { useState } from "react";
import { Badge } from "@/components/atoms/Badge";
import { Button } from "@/components/atoms/Button";
import { Diff } from "@/components/molecules/Diff";
import { Panel } from "@/components/molecules/Panel";
import { revertPolicy } from "@/api/client";
import type { PolicyVersion } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { dateTime, timeAgo } from "@/lib/time";
import { usePolicyChange } from "./usePolicyChange";

// Every version of the policy, newest first: who put it in force and what
// changed. One whose text the log holds can be restored, as a new version;
// those from before Agentry kept the text show without Revert.
export function PolicyHistory({ hash, onChanged }: { hash: string; onChanged: () => void }) {
  const { data: history, error } = useResource<PolicyVersion[]>("/api/policy/history");
  const { busy, run } = usePolicyChange(onChanged);
  const inForce = history?.find((v) => !v.refused)?.position;
  return (
    <Panel flush title="History" aside={<>newest first</>}>
      {error && <p className="px-4 py-2.5 text-[12px] text-danger">{error}</p>}
      {history?.length === 0 && <p className="px-4 py-2.5 text-[12px] text-ink-muted">No versions recorded yet.</p>}
      <ol>
        {history?.map((version) => (
          <VersionRow
            key={version.position}
            version={version}
            inForce={version.position === inForce}
            canRevert={version.canRevert && !version.refused && version.hash !== hash}
            busy={busy}
            onRevert={() => run("Restored an earlier version", () => revertPolicy(version.position, hash))}
          />
        ))}
      </ol>
    </Panel>
  );
}

interface VersionRowProps {
  version: PolicyVersion;
  inForce: boolean;
  canRevert: boolean; // its text is in the log, and isn't the one in force
  busy: boolean;
  onRevert: () => void;
}

function VersionRow({ version, inForce, canRevert, busy, onRevert }: VersionRowProps) {
  const [open, setOpen] = useState(false);
  return (
    <li className="border-b border-divider last:border-b-0">
      <div className="flex flex-wrap items-center gap-3 px-4 py-2 text-[12px]">
        <span className="w-16 shrink-0 text-ink-faint" title={dateTime(version.at)}>
          {timeAgo(version.at)}
        </span>
        <span className="w-16 shrink-0 font-mono text-ink-secondary">{whoOf(version)}</span>
        {version.refused && <Badge variant="danger">refused</Badge>}
        {inForce && <Badge variant="success">in force</Badge>}
        <span className="min-w-0 flex-1 truncate text-ink-primary" title={version.change}>
          {version.change}
        </span>
        {version.diff && (
          <Button size="sm" variant="ghost" aria-expanded={open} onClick={() => setOpen(!open)}>
            {open ? "Hide diff" : "Diff"}
          </Button>
        )}
        {canRevert && (
          <Button size="sm" disabled={busy} onClick={onRevert}>
            Revert
          </Button>
        )}
      </div>
      {open && version.diff && <Diff text={version.diff} className="border-t border-divider" />}
    </li>
  );
}

// whoOf is you, or Agentry for what it loaded by itself: at start, or a change it noticed on disk.
function whoOf(version: PolicyVersion): string {
  return version.actor.kind === "human" ? version.actor.id : "Agentry";
}

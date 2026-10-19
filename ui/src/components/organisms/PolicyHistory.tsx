import { useState } from "react";
import { Badge } from "@/components/atoms/Badge";
import { Button } from "@/components/atoms/Button";
import { Banner } from "@/components/molecules/Banner";
import { Dialog } from "@/components/molecules/Dialog";
import { Diff } from "@/components/molecules/Diff";
import { Panel } from "@/components/molecules/Panel";
import { revertPolicy } from "@/api/client";
import type { PolicyVersion, PolicyVersionChange } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { dateTime, timeAgo } from "@/lib/time";
import { usePolicyChange } from "@/lib/policy-change";

// Every version of the policy, newest first: who put it in force and what
// changed. One whose text the log holds can be restored, as a new version,
// once you have seen what that changes, since an early version can loosen the
// policy; those from before Agentry kept the text show without Revert.
export function PolicyHistory({ hash, onChanged }: { hash: string; onChanged: () => void }) {
  const { data: history, error } = useResource<PolicyVersion[]>("/api/policy/history");
  const { busy, run } = usePolicyChange(onChanged);
  const [reverting, setReverting] = useState<PolicyVersion | null>(null);
  // The version in force is the newest one put in force with this text:
  // a later one can be refused, and an older one can share its text.
  const inForce = history?.find((v) => !v.refused && v.hash === hash)?.position;
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
            onRevert={() => setReverting(version)}
          />
        ))}
      </ol>
      {reverting && (
        <RevertDialog
          version={reverting}
          onClose={() => setReverting(null)}
          onRevert={() => {
            setReverting(null);
            run("Restored an earlier version", () => revertPolicy(reverting.position, hash));
          }}
        />
      )}
    </Panel>
  );
}

interface VersionRowProps {
  version: PolicyVersion;
  inForce: boolean;
  canRevert: boolean; // its text is in the log, and isn't the text in force
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

// RevertDialog shows what reverting to version would change, from the
// version in force, and says so when it would put every agent under one
// profile. Nothing changes until you confirm.
function RevertDialog({ version, onClose, onRevert }: { version: PolicyVersion; onClose: () => void; onRevert: () => void }) {
  const { data: change, error } = useResource<PolicyVersionChange>(`/api/policy/history/${version.position}`);
  const loosens = change?.useForEveryone && change.useForEveryone !== change.inForceUseForEveryone;
  return (
    <Dialog title={`Revert to the version of ${dateTime(version.at)}`} onClose={onClose} large>
      <div className="flex flex-col gap-4 text-[13px] text-ink-secondary">
        {error && <Banner title="Can't show what it would change">{error}</Banner>}
        {!change && !error && <p className="text-ink-muted">Reading the version…</p>}
        {change && !change.diff && <p>This version is the one in force: reverting to it changes nothing.</p>}
        {change?.diff && (
          <>
            <p>
              Reverting replaces the policy in force with this version ({version.change}), written back to policy.toml as a new one. The
              text in force now stays in History and in a backup. It changes:
            </p>
            <Diff text={change.diff} className="border border-divider" />
          </>
        )}
        {loosens && (
          <Banner tone="warning" title="It puts every agent under one profile">
            This version puts every agent under the {change.useForEveryone} profile (use_for_everyone), whatever profile each one names.
          </Banner>
        )}
        <div className="flex gap-2">
          <Button variant="danger" disabled={!change?.diff || !!error} onClick={onRevert}>
            Revert
          </Button>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </div>
    </Dialog>
  );
}

// whoOf is you for a change from the page, and Agentry for what it loaded at
// start. A hand edit of the file, or a file refused, shows as on disk:
// whoever edited the file, Agentry only read it.
function whoOf(version: PolicyVersion): string {
  if (version.actor.kind === "human") return version.actor.id;
  return version.onDisk ? "on disk" : "Agentry";
}

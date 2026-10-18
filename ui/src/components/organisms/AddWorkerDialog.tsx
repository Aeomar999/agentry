import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/atoms/Button";
import { Banner } from "@/components/molecules/Banner";
import { InstallCommand } from "@/components/molecules/InstallCommand";
import { Dialog } from "@/components/molecules/Dialog";
import { api } from "@/api/client";
import type { Invite, Worker } from "@/api/types";
import { clockTime } from "@/lib/time";

interface AddWorkerDialogProps {
  workers: Worker[]; // as the page has them now, live
  onClose: () => void;
}

// Makes a join code and shows the one command that uses it: run on the
// other machine, it installs agentry, joins with the code and starts. The
// dialog says so when the machine shows up.
export function AddWorkerDialog({ workers, onClose }: AddWorkerDialogProps) {
  const [invite, setInvite] = useState<Invite | null>(null);
  const [error, setError] = useState("");
  const before = useRef(new Set(workers.map((w) => w.name)));
  const joined = workers.find((w) => !before.current.has(w.name));

  useEffect(() => {
    api<Invite>("POST", "/api/workers/invite")
      .then(setInvite)
      .catch((failure: Error) => setError(failure.message));
  }, []);

  return (
    <Dialog title="Add a worker" onClose={onClose} wide>
      <div className="flex flex-col gap-4 text-[13px] text-ink-secondary">
        {error && <Banner title="Couldn't make a join code">{error}</Banner>}
        {joined ? (
          <Banner tone="success" title={`${joined.name} joined`}>
            {joined.online ? "It is online. Pick it under Runs on in an agent's settings." : "It joined and is starting."}
          </Banner>
        ) : (
          <>
            <p>
              On the machine you want agents to run on, run this. It installs agentry, joins this server and keeps the worker running. That
              machine needs git and Claude Code signed in.
            </p>
            {invite ? <InstallCommand commands={invite} /> : <div className="h-10 rounded-md bg-surface-2" />}
            <p className="text-[12px] text-ink-muted">
              The code in it works once{invite && <>, until {clockTime(invite.expires)}</>}. To name the machine, put{" "}
              <code className="font-mono text-ink-secondary">AGENTRY_WORKER_NAME=name</code> before <code className="font-mono">sh</code>{" "}
              (on Windows, set <code className="font-mono text-ink-secondary">$env:AGENTRY_WORKER_NAME</code> first). Run on a machine that
              has joined, it updates it instead. There, <code className="font-mono text-ink-secondary">agentry worker</code> does the rest:
              status, logs, stop, start, update, leave.
            </p>
          </>
        )}
        <div className="flex justify-end">
          <Button variant={joined ? "primary" : "secondary"} onClick={onClose}>
            {joined ? "Done" : "Close"}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}

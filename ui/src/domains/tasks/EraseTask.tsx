import { useState } from "react";
import type { Erasure } from "@/api/types";
import { eraseTask } from "@/api/privacy";
import { Button } from "@/components/atoms/Button";
import { Input } from "@/components/atoms/Input";
import { Dialog } from "@/components/molecules/Dialog";
import { Field, FieldError } from "@/components/molecules/Field";
import { Panel } from "@/components/molecules/Panel";
import { shownId } from "@/lib/shown-id";

// EraseTask is the Details tab's way to erase the task for good
// (docs/design/log-growth.md section 5, "Removing a whole task"), behind a
// question that says nothing undoes it and what goes. The server refuses,
// with its reason, an epic whose stories still name it.
export function EraseTask({ task, kind, onErased }: { task: string; kind?: string; onErased: (erasure: Erasure) => void }) {
  const [asking, setAsking] = useState(false);
  return (
    <Panel title="Erase this task" tone="danger">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-xl text-[12px] text-ink-secondary">
          Takes every word of it out of Agentry, for good. To take out one secret and keep the task, use Redact a text at the top.
        </p>
        <Button variant="danger" onClick={() => setAsking(true)}>
          Erase this task…
        </Button>
      </div>
      {asking && <EraseQuestion task={task} kind={kind} onKeep={() => setAsking(false)} onErased={onErased} />}
    </Panel>
  );
}

function EraseQuestion({
  task,
  kind,
  onKeep,
  onErased,
}: {
  task: string;
  kind?: string;
  onKeep: () => void;
  onErased: (erasure: Erasure) => void;
}) {
  const [why, setWhy] = useState("");
  const [busy, setBusy] = useState(false);
  const [refusal, setRefusal] = useState("");

  async function erase() {
    setBusy(true);
    setRefusal("");
    try {
      onErased(await eraseTask(task, why));
    } catch (failure) {
      setRefusal((failure as Error).message);
      setBusy(false);
    }
  }

  return (
    <Dialog title={`Erase ${shownId(task, kind)} for good?`} onClose={onKeep} held={busy}>
      <div className="flex flex-col gap-3 text-[13px] text-ink-secondary">
        <p className="font-medium text-ink-primary">Nothing undoes this.</p>
        <p>If an agent's turn is running on it, the turn is stopped first. Then these go:</p>
        <ul className="flex list-disc flex-col gap-1 pl-4">
          <li>every row of its history in the log</li>
          <li>its archive, if it was archived</li>
          <li>the shared files no other task names</li>
          <li>the copies of its words in its epic and in the task that asked for it</li>
        </ul>
        <Field label="Why, for the log (optional)" htmlFor="erase-why">
          <Input id="erase-why" value={why} disabled={busy} onChange={(e) => setWhy(e.target.value)} />
        </Field>
        {refusal && <FieldError id="erase-refusal">It wasn&apos;t erased: {refusal}</FieldError>}
        <div className="flex justify-end gap-2">
          <Button autoFocus disabled={busy} onClick={onKeep}>
            Keep
          </Button>
          <Button variant="danger" disabled={busy} onClick={erase}>
            {busy ? "Erasing…" : "Erase for good"}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}

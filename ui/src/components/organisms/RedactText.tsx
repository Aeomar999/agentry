import { useState } from "react";
import type { Redaction } from "@/api/types";
import { previewAgentsRedaction, previewRedaction, redact, redactAgents } from "@/api/privacy";
import { Button } from "@/components/atoms/Button";
import { Dialog } from "@/components/molecules/Dialog";
import { FieldError } from "@/components/molecules/Field";
import { LeftBehind } from "@/components/molecules/LeftBehind";
import { MarkText } from "@/components/molecules/MarkText";
import { Warnings } from "@/components/molecules/Warnings";
import { useCountedRedaction } from "@/lib/counted-redaction";
import { rowsOf } from "@/lib/rows";
import { shownId } from "@/lib/shown-id";
import { AGENTS } from "@/lib/streams";
import { dateTime } from "@/lib/time";

// RedactText takes a secret out of a task, live or archived, or out of
// the agents' roster and what they learned (docs/design/log-growth.md
// section 5, "Removing a secret"): you paste or mark the text, see how many
// rows hold it, and only then redact it, each row keeping the rest of its
// words with [redacted] in its place.

// Target is where the text is taken out of: its name on the page, and its
// preview and redaction.
interface Target {
  name: string;
  preview: (text: string) => Promise<number>;
  redact: (text: string, why: string, rows: number) => Promise<Redaction>;
}

type Where = { task: string; kind?: string } | { agents: true };

function targetOf(where: Where): Target {
  if ("agents" in where) return { name: AGENTS, preview: previewAgentsRedaction, redact: redactAgents };
  const { task, kind } = where;
  return {
    name: shownId(task, kind),
    preview: (text) => previewRedaction(task, text),
    redact: (text, why, rows) => redact(task, text, why, rows),
  };
}

type RedactTextProps = Where & {
  marked: string; // the text marked on the page as it opened, to start from
  onClose: () => void;
  onRedacted: () => void;
};

export function RedactText({ marked, onClose, onRedacted, ...where }: RedactTextProps) {
  const [text, setText] = useState(marked);
  const [why, setWhy] = useState("");
  const target = targetOf(where);
  const redaction = useCountedRedaction(target.preview, target.redact, onRedacted);
  const { stage, busy } = redaction;
  return (
    <Dialog title={`Redact a text in ${target.name}`} onClose={onClose} held={busy} wide>
      <div className="flex flex-col gap-3 text-[13px] text-ink-secondary">
        {stage.step === "mark" && (
          <MarkText
            idPrefix="redact"
            hint="Paste it, or mark it on the page before you open this. It is matched exactly, spaces and all."
            text={text}
            why={why}
            refusal={stage.refusal}
            busy={busy}
            onText={setText}
            onWhy={setWhy}
            onCount={() => redaction.count(text)}
            onCancel={onClose}
          />
        )}
        {stage.step === "confirm" && (
          <>
            <p className="text-ink-primary">
              {rowsOf(stage.count)} of {target.name} {stage.count === 1 ? "holds" : "hold"} this text. Each keeps the rest of its words,
              with [redacted] in its place. Nothing undoes this.
            </p>
            {stage.note && <FieldError id="redact-note">{stage.note}</FieldError>}
            <div className="flex justify-end gap-2">
              <Button disabled={busy} onClick={redaction.back}>
                Back
              </Button>
              <Button variant="danger" disabled={busy} onClick={() => redaction.confirm(text, why, stage.count)}>
                {busy ? "Redacting…" : `Redact ${rowsOf(stage.count)}`}
              </Button>
            </div>
          </>
        )}
        {stage.step === "done" && (
          <>
            <p className="text-ink-primary">
              Redacted {rowsOf(stage.redaction.rows)} of {target.name} at {dateTime(stage.redaction.at)}.
            </p>
            <Warnings warnings={stage.redaction.warnings} />
            <LeftBehind before={dateTime(stage.redaction.at)} what="the text" worktrees={"task" in where ? "The task's" : null} />
            <div className="flex justify-end">
              <Button autoFocus onClick={onClose}>
                Close
              </Button>
            </div>
          </>
        )}
      </div>
    </Dialog>
  );
}

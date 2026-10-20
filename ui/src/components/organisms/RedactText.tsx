import { useState } from "react";
import type { Redaction } from "@/api/types";
import { ApiError } from "@/api/client";
import { previewRedaction, redact } from "@/api/privacy";
import { Button } from "@/components/atoms/Button";
import { Input, TextArea } from "@/components/atoms/Input";
import { Dialog } from "@/components/molecules/Dialog";
import { Field, FieldError, errorId } from "@/components/molecules/Field";
import { LeftBehind } from "@/components/molecules/LeftBehind";
import { Warnings } from "@/components/molecules/Warnings";
import { dateTime } from "@/lib/time";

// RedactText takes a secret out of a task, live or archived
// (docs/design/log-growth.md section 5, "Removing a secret"): you paste or
// mark the text, see how many rows hold it, and only then redact it, each
// row keeping the rest of its words with [redacted] in its place.

// Where the question is: the text being marked, with the server's refusal
// of the last preview; the count, waiting for you to say yes, with what
// happened to the last try; or what the redaction did.
type Stage = { step: "mark"; refusal: string } | { step: "confirm"; rows: number; note: string } | { step: "done"; redaction: Redaction };

// useRedaction runs the preview and the redaction. A redaction refused
// 409, as when the rows changed since the preview, previews again and asks
// again with the new count, rather than failing.
function useRedaction(task: string, onRedacted: () => void) {
  const [stage, setStage] = useState<Stage>({ step: "mark", refusal: "" });
  const [busy, setBusy] = useState(false);

  async function countRows(text: string, note: string) {
    try {
      setStage({ step: "confirm", rows: await previewRedaction(task, text), note });
    } catch (failure) {
      setStage({ step: "mark", refusal: (failure as Error).message });
    }
  }

  async function preview(text: string) {
    setBusy(true);
    await countRows(text, "");
    setBusy(false);
  }

  async function confirm(text: string, why: string, rows: number) {
    setBusy(true);
    try {
      setStage({ step: "done", redaction: await redact(task, text, why) });
      onRedacted();
    } catch (failure) {
      const reason = (failure as Error).message;
      if (failure instanceof ApiError && failure.status === 409) {
        await countRows(text, `Agentry refused it (${reason}), so the rows were counted again.`);
      } else {
        setStage({ step: "confirm", rows, note: `It wasn't redacted: ${reason}` });
      }
    }
    setBusy(false);
  }

  return { stage, busy, preview, confirm, back: () => setStage({ step: "mark", refusal: "" }) };
}

interface RedactTextProps {
  task: string;
  marked: string; // the text marked on the page as it opened, to start from
  onClose: () => void;
  onRedacted: () => void;
}

export function RedactText({ task, marked, onClose, onRedacted }: RedactTextProps) {
  const [text, setText] = useState(marked);
  const [why, setWhy] = useState("");
  const redaction = useRedaction(task, onRedacted);
  const { stage, busy } = redaction;
  return (
    <Dialog title={`Redact a text in ${task}`} onClose={onClose} held={busy} wide>
      <div className="flex flex-col gap-3 text-[13px] text-ink-secondary">
        {stage.step === "mark" && (
          <>
            <Field
              label="The text to take out"
              htmlFor="redact-text"
              error={stage.refusal && `It can't be redacted: ${stage.refusal}`}
              hint="Paste it, or mark it on the page before you open this. It is matched exactly, spaces and all."
            >
              <TextArea
                id="redact-text"
                className="font-mono text-[12px]"
                autoFocus
                value={text}
                disabled={busy}
                aria-describedby={stage.refusal ? errorId("redact-text") : undefined}
                onChange={(e) => setText(e.target.value)}
              />
            </Field>
            <Field label="Why, for the log (optional)" htmlFor="redact-why">
              <Input id="redact-why" value={why} disabled={busy} onChange={(e) => setWhy(e.target.value)} />
            </Field>
            <div className="flex justify-end gap-2">
              <Button disabled={busy} onClick={onClose}>
                Cancel
              </Button>
              <Button variant="primary" disabled={busy || !text.trim()} onClick={() => redaction.preview(text)}>
                {busy ? "Counting…" : "Count the rows"}
              </Button>
            </div>
          </>
        )}
        {stage.step === "confirm" && (
          <>
            <p className="text-ink-primary">
              {rowsOf(stage.rows)} of {task} {stage.rows === 1 ? "holds" : "hold"} this text. Each keeps the rest of its words, with
              [redacted] in its place. Nothing undoes this.
            </p>
            {stage.note && <FieldError id="redact-note">{stage.note}</FieldError>}
            <div className="flex justify-end gap-2">
              <Button disabled={busy} onClick={redaction.back}>
                Back
              </Button>
              <Button variant="danger" disabled={busy} onClick={() => redaction.confirm(text, why, stage.rows)}>
                {busy ? "Redacting…" : `Redact ${rowsOf(stage.rows)}`}
              </Button>
            </div>
          </>
        )}
        {stage.step === "done" && (
          <>
            <p className="text-ink-primary">
              Redacted {rowsOf(stage.redaction.rows)} of {task} at {dateTime(stage.redaction.at)}.
            </p>
            <Warnings warnings={stage.redaction.warnings} />
            <LeftBehind before={dateTime(stage.redaction.at)} what="the text" />
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

const rowsOf = (rows: number) => (rows === 1 ? "1 row" : `${rows} rows`);

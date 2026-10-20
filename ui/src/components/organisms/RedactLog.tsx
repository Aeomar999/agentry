import { type ReactNode, useState } from "react";
import type { LogPreview, LogRedaction } from "@/api/types";
import { previewLogRedaction, redactLog } from "@/api/privacy";
import { Badge } from "@/components/atoms/Badge";
import { Button } from "@/components/atoms/Button";
import { Dialog } from "@/components/molecules/Dialog";
import { FieldError } from "@/components/molecules/Field";
import { LeftBehind } from "@/components/molecules/LeftBehind";
import { MarkText } from "@/components/molecules/MarkText";
import { Warnings } from "@/components/molecules/Warnings";
import { useCountedRedaction } from "@/lib/counted-redaction";
import { rowsOf } from "@/lib/rows";
import { dateTime } from "@/lib/time";

// RedactLog takes a secret pasted widely out of the whole log
// (docs/design/log-growth.md section 5, "Removing a secret"): every task,
// archived ones too, what the agents learned, and the other streams. As
// for one task (RedactText), you see the rows first, here in each stream,
// and only the rows you saw are redacted: a stream that took one since,
// or that you didn't see, is left and named, for you to count again.

// countsOf is the rows the preview counted in each stream that can be
// redacted, which the redaction sends; a stream whose preview was refused
// has none.
const countsOf = (preview: LogPreview) =>
  Object.fromEntries(preview.streams.filter((listed) => !listed.error && listed.rows > 0).map((listed) => [listed.stream, listed.rows]));

// redactCounted redacts the text in each stream the preview counted.
const redactCounted = (text: string, why: string, counted: LogPreview) => redactLog(text, why, countsOf(counted));

// otherWarnings is the warnings besides the streams left, which the
// server also lists among them, as "<stream>: <reason>", and the page
// names on their own.
const otherWarnings = (redaction: LogRedaction) =>
  redaction.warnings.filter((warning) => !redaction.failures.some((failed) => warning === `${failed.stream}: ${failed.reason}`));

export function RedactLog({ onClose }: { onClose: () => void }) {
  const [text, setText] = useState("");
  const [why, setWhy] = useState("");
  const redaction = useCountedRedaction(previewLogRedaction, redactCounted);
  const { stage, busy } = redaction;
  return (
    <Dialog title="Redact a text everywhere" onClose={onClose} held={busy} wide>
      <div className="flex flex-col gap-3 text-[13px] text-ink-secondary">
        {stage.step === "mark" && (
          <MarkText
            idPrefix="redact-log"
            hint="Paste it. It is matched exactly, spaces and all, in every task, archived ones too, and in what the agents learned."
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
              {rowsOf(stage.count.rows)} {stage.count.rows === 1 ? "holds" : "hold"} this text. Each keeps the rest of its words, with
              [redacted] in its place. Nothing undoes this.
            </p>
            <StreamList>
              {stage.count.streams.map((listed) => (
                <StreamRow key={listed.stream} stream={listed.stream} archived={listed.archived}>
                  {listed.error ? <span className="text-danger-ink">can&apos;t be redacted: {listed.error}</span> : rowsOf(listed.rows)}
                </StreamRow>
              ))}
            </StreamList>
            {stage.note && <FieldError id="redact-log-note">{stage.note}</FieldError>}
            <div className="flex justify-end gap-2">
              <Button disabled={busy} onClick={redaction.back}>
                Back
              </Button>
              <Button variant="danger" disabled={busy} onClick={() => redaction.confirm(text, why, stage.count)}>
                {busy ? "Redacting…" : `Redact ${rowsOf(stage.count.rows)}`}
              </Button>
            </div>
          </>
        )}
        {stage.step === "done" && (
          <Redacted redaction={stage.redaction} busy={busy} onRecount={() => redaction.count(text)} onClose={onClose} />
        )}
      </div>
    </Dialog>
  );
}

// Redacted is what a redaction across the log did: the rows in each
// stream, each stream left and why, with counting again for those it
// would redact, and what may be left for now.
function Redacted({
  redaction,
  busy,
  onRecount,
  onClose,
}: {
  redaction: LogRedaction;
  busy: boolean;
  onRecount: () => void;
  onClose: () => void;
}) {
  const at = redaction.streams[0]?.at ?? "";
  const recount = redaction.failures.some((failed) => failed.recount);
  return (
    <>
      <p className="text-ink-primary">
        Redacted {rowsOf(redaction.rows)} at {dateTime(at)}.
      </p>
      <StreamList>
        {redaction.streams.map((done) => (
          <StreamRow key={done.stream} stream={done.stream}>
            {rowsOf(done.rows)}
          </StreamRow>
        ))}
      </StreamList>
      {redaction.failures.length > 0 && (
        <>
          <p className="text-ink-primary">
            {redaction.failures.length === 1 ? "This still holds the text:" : "These still hold the text:"}
          </p>
          <StreamList>
            {redaction.failures.map((failed) => (
              <StreamRow key={failed.stream} stream={failed.stream}>
                <span className="text-danger-ink">{failed.reason}</span>
              </StreamRow>
            ))}
          </StreamList>
        </>
      )}
      <Warnings warnings={otherWarnings(redaction)} />
      <LeftBehind before={dateTime(at)} what="the text" worktrees="Each task's" />
      <div className="flex justify-end gap-2">
        {recount && (
          <Button variant="primary" disabled={busy} onClick={onRecount}>
            {busy ? "Counting…" : "Count again"}
          </Button>
        )}
        <Button autoFocus onClick={onClose}>
          Close
        </Button>
      </div>
    </>
  );
}

function StreamList({ children }: { children: ReactNode }) {
  return <ul className="flex flex-col divide-y divide-divider border border-divider">{children}</ul>;
}

// StreamRow is one stream of the log, marked when it is an archived task's,
// with what is said of it.
function StreamRow({ stream, archived, children }: { stream: string; archived?: boolean; children: ReactNode }) {
  return (
    <li className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-3 py-2">
      <span className="flex items-center gap-2 font-mono text-[12px] text-ink-primary">
        {stream}
        {archived && <Badge>archived</Badge>}
      </span>
      <span className="ml-auto text-[12px]">{children}</span>
    </li>
  );
}

import { useState } from "react";
import { ApiError } from "@/api/client";

// Where a redaction's question is: the text being marked, with the
// server's refusal of the last preview; the count, waiting for you to say
// yes, with what happened to the last try; or what the redaction did.
export type RedactionStage<Count, Done> =
  { step: "mark"; refusal: string } | { step: "confirm"; count: Count; note: string } | { step: "done"; redaction: Done };

// useCountedRedaction runs a preview and the redaction, which sends the
// count you confirmed (docs/design/log-growth.md section 5, "Removing a
// secret"). The server refuses it 409 when the rows holding the text aren't
// that many any more, as when one was written since the preview, or none
// holds it now: that previews again and asks again with the new count, or
// shows why it can't be redacted, rather than failing.
export function useCountedRedaction<Count, Done>(
  preview: (text: string) => Promise<Count>,
  redact: (text: string, why: string, count: Count) => Promise<Done>,
  onRedacted: () => void = () => {},
) {
  const [stage, setStage] = useState<RedactionStage<Count, Done>>({ step: "mark", refusal: "" });
  const [busy, setBusy] = useState(false);

  async function countRows(text: string, note: string) {
    try {
      setStage({ step: "confirm", count: await preview(text), note });
    } catch (failure) {
      setStage({ step: "mark", refusal: (failure as Error).message });
    }
  }

  async function count(text: string) {
    setBusy(true);
    await countRows(text, "");
    setBusy(false);
  }

  async function confirm(text: string, why: string, counted: Count) {
    setBusy(true);
    try {
      setStage({ step: "done", redaction: await redact(text, why, counted) });
      onRedacted();
    } catch (failure) {
      const reason = (failure as Error).message;
      if (failure instanceof ApiError && failure.status === 409) {
        await countRows(text, `Agentry refused it (${reason}), so the rows were counted again.`);
      } else {
        setStage({ step: "confirm", count: counted, note: `It wasn't redacted: ${reason}` });
      }
    }
    setBusy(false);
  }

  return { stage, busy, count, confirm, back: () => setStage({ step: "mark", refusal: "" }) };
}

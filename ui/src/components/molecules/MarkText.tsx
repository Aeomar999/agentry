import { Button } from "@/components/atoms/Button";
import { Input, TextArea } from "@/components/atoms/Input";
import { Field, errorId } from "@/components/molecules/Field";

// MarkText is a redaction's first step: the text to take out, matched
// exactly, and why, for the log, with the server's refusal of the last
// count under the text. Its ids start with idPrefix, so two can't clash.
export function MarkText({
  idPrefix,
  hint,
  text,
  why,
  refusal,
  busy,
  onText,
  onWhy,
  onCount,
  onCancel,
}: {
  idPrefix: string;
  hint: string;
  text: string;
  why: string;
  refusal: string;
  busy: boolean;
  onText: (text: string) => void;
  onWhy: (why: string) => void;
  onCount: () => void;
  onCancel: () => void;
}) {
  const textId = `${idPrefix}-text`;
  return (
    <>
      <Field label="The text to take out" htmlFor={textId} error={refusal && `It can't be redacted: ${refusal}`} hint={hint}>
        <TextArea
          id={textId}
          className="font-mono text-[12px]"
          autoFocus
          value={text}
          disabled={busy}
          aria-describedby={refusal ? errorId(textId) : undefined}
          onChange={(e) => onText(e.target.value)}
        />
      </Field>
      <Field label="Why, for the log (optional)" htmlFor={`${idPrefix}-why`}>
        <Input id={`${idPrefix}-why`} value={why} disabled={busy} onChange={(e) => onWhy(e.target.value)} />
      </Field>
      <div className="flex justify-end gap-2">
        <Button disabled={busy} onClick={onCancel}>
          Cancel
        </Button>
        <Button variant="primary" disabled={busy || !text.trim()} onClick={onCount}>
          {busy ? "Counting…" : "Count the rows"}
        </Button>
      </div>
    </>
  );
}

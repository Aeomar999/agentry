import { KeyboardEvent, useId, useState } from "react";
import { LabelChip } from "./LabelChip";

interface LabelInputProps {
  labels: string[];
  onChange: (labels: string[]) => void;
  known: string[]; // labels other tasks use, offered first
  autoFocus?: boolean;
}

// Labels as chips, with a field to add more: Enter or a comma adds one,
// Backspace in an empty field takes the last off.
export function LabelInput({ labels, onChange, known, autoFocus }: LabelInputProps) {
  const [draft, setDraft] = useState("");
  const listId = useId();

  function add(text: string) {
    const label = text.trim().toLowerCase();
    if (label && !labels.includes(label)) onChange([...labels, label]);
    setDraft("");
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter" || event.key === ",") {
      event.preventDefault();
      add(draft);
    } else if (event.key === "Backspace" && draft === "" && labels.length > 0) {
      onChange(labels.slice(0, -1));
    }
  }

  return (
    <div className="flex min-h-8 w-full flex-wrap items-center gap-1 rounded-md border border-divider bg-surface-1 px-2 py-1 transition-colors duration-(--dur-fast) focus-within:border-focus hover:not-focus-within:border-divider-2">
      {labels.map((label) => (
        <LabelChip key={label} label={label} onRemove={() => onChange(labels.filter((l) => l !== label))} />
      ))}
      <input
        value={draft}
        autoFocus={autoFocus}
        list={listId}
        onChange={(event) => (event.target.value.endsWith(",") ? add(event.target.value.slice(0, -1)) : setDraft(event.target.value))}
        onKeyDown={onKeyDown}
        onBlur={() => add(draft)}
        placeholder={labels.length ? "" : "bug, docs, frontend"}
        className="h-6 min-w-24 flex-1 bg-transparent text-[13px] text-ink-primary outline-none placeholder:text-ink-faint"
      />
      <datalist id={listId}>
        {known
          .filter((label) => !labels.includes(label))
          .map((label) => (
            <option key={label} value={label} />
          ))}
      </datalist>
    </div>
  );
}

import { ReactNode } from "react";
import { cn } from "@/lib/cn";

interface RadioProps {
  name: string; // the group it is one choice of
  checked: boolean;
  onChange: () => void;
  label: ReactNode;
  hint?: ReactNode;
  disabled?: boolean;
}

// One choice of a group, drawn like a Checkbox; the whole row picks it.
export function Radio({ name, checked, onChange, label, hint, disabled }: RadioProps) {
  return (
    <label className={cn("flex cursor-pointer items-start gap-2.5 text-[13px]", disabled && "cursor-not-allowed opacity-50")}>
      <input type="radio" className="peer sr-only" name={name} checked={checked} disabled={disabled} onChange={onChange} />
      <span
        className={cn(
          "mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-sm border transition-colors duration-(--dur-fast)",
          "peer-focus-visible:border-focus",
          checked ? "border-brand bg-surface-1" : "border-divider-2 bg-surface-1",
        )}
        aria-hidden
      >
        {checked && <span className="size-2 bg-brand" />}
      </span>
      <span className="flex flex-col gap-0.5">
        <span className="text-ink-primary">{label}</span>
        {hint && <span className="text-[11px] text-ink-muted">{hint}</span>}
      </span>
    </label>
  );
}

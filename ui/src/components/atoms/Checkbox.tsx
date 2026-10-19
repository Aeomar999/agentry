import { ReactNode } from "react";
import { Check } from "lucide-react";
import { cn } from "@/lib/cn";

interface CheckboxProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: ReactNode;
  hint?: ReactNode;
  disabled?: boolean;
}

// A square box and its label; the whole row toggles it.
export function Checkbox({ checked, onChange, label, hint, disabled }: CheckboxProps) {
  return (
    <label className={cn("flex cursor-pointer items-start gap-2.5 text-[13px]", disabled && "cursor-not-allowed opacity-50")}>
      <input
        type="checkbox"
        className="peer sr-only"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span
        className={cn(
          "mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-sm border transition-colors duration-(--dur-fast)",
          "peer-focus-visible:border-focus",
          checked ? "border-brand bg-brand text-white" : "border-divider-2 bg-surface-1",
        )}
        aria-hidden
      >
        {checked && <Check className="size-3" strokeWidth={3} />}
      </span>
      <span className="flex flex-col gap-0.5">
        <span className="text-ink-primary">{label}</span>
        {hint && <span className="text-[11px] text-ink-muted">{hint}</span>}
      </span>
    </label>
  );
}

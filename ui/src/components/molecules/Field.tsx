import { ReactNode } from "react";

// A form field: its label, the control, and a hint under it. The label points
// at one control by htmlFor, or, for a group of controls such as chips, gives
// labelId for the group to name itself with aria-labelledby.
type Labels = { htmlFor: string; labelId?: never } | { labelId: string; htmlFor?: never };

export function Field({ label, htmlFor, labelId, hint, children }: Labels & { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={htmlFor} id={labelId} className="text-[12px] font-medium text-ink-secondary">
        {label}
      </label>
      {children}
      {hint && <p className="text-[11px] text-ink-muted">{hint}</p>}
    </div>
  );
}

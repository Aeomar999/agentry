import { ReactNode } from "react";
import { CircleAlert } from "lucide-react";

// A form field: its label, the control, and a hint under it. The label points
// at one control by htmlFor, or, for a group of controls such as chips, gives
// labelId for the group to name itself with aria-labelledby. An error shows
// above the hint, under errorId(the control's id), for the control's
// aria-describedby.
type Labels = { htmlFor: string; labelId?: never } | { labelId: string; htmlFor?: never };

export const errorId = (id: string) => `${id}-error`;

export function Field({
  label,
  htmlFor,
  labelId,
  hint,
  error,
  aside,
  children,
}: Labels & { label: string; hint?: ReactNode; error?: string; aside?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <span className="flex items-baseline justify-between gap-2">
        <label htmlFor={htmlFor} id={labelId} className="text-[12px] font-medium text-ink-secondary">
          {label}
        </label>
        {aside && <span className="text-[11px] text-ink-muted">{aside}</span>}
      </span>
      {children}
      {error && <FieldError id={errorId(htmlFor ?? labelId!)}>{error}</FieldError>}
      {hint && <p className="text-[11px] text-ink-muted">{hint}</p>}
    </div>
  );
}

// FieldError is what is wrong with a field, in words with an icon, so colour is never the only sign.
export function FieldError({ id, children }: { id: string; children: ReactNode }) {
  return (
    <p id={id} className="flex items-start gap-1.5 text-[12px] text-danger-ink">
      <CircleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
      <span>{children}</span>
    </p>
  );
}

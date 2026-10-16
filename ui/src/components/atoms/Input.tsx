import { forwardRef, InputHTMLAttributes, ReactNode, TextareaHTMLAttributes } from "react";
import { cn } from "@/lib/cn";

// The wrapper carries the border and changes it on focus, so the field
// itself needs no focus ring of its own.
const FIELD =
  "flex w-full items-center gap-2 rounded-md border border-divider bg-surface-1 px-2.5 text-[13px] text-ink-primary " +
  "transition-colors duration-(--dur-fast) focus-within:border-brand hover:not-focus-within:border-divider-2";

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  icon?: ReactNode;
  trailing?: ReactNode;
  mono?: boolean;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input({ icon, trailing, mono, className, ...rest }, ref) {
  return (
    <div className={cn(FIELD, "h-8", className)}>
      {icon && <span className="shrink-0 text-ink-muted [&>svg]:size-3.5">{icon}</span>}
      <input
        ref={ref}
        className={cn("h-full min-w-0 flex-1 bg-transparent outline-none placeholder:text-ink-faint", mono && "font-mono text-[12px]")}
        {...rest}
      />
      {trailing}
    </div>
  );
});

export const TextArea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(function TextArea(
  { className, ...rest },
  ref,
) {
  return (
    <div className={cn(FIELD, "py-1.5", className)}>
      <textarea ref={ref} className="min-h-20 w-full resize-y bg-transparent leading-5 outline-none placeholder:text-ink-faint" {...rest} />
    </div>
  );
});

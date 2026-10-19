import { forwardRef, SelectHTMLAttributes } from "react";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib/cn";

// A native select, dressed like Input: the browser's own list, keyboard and
// accessibility, with the wrapper's border turning orange on focus.
export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(function Select(
  { className, children, ...rest },
  ref,
) {
  return (
    <div
      className={cn(
        "relative flex h-8 w-full items-center rounded-md border border-divider bg-surface-1 text-[13px] text-ink-primary",
        "transition-colors duration-(--dur-fast) focus-within:border-focus hover:not-focus-within:border-divider-2",
        className,
      )}
    >
      <select ref={ref} className="h-full w-full cursor-pointer appearance-none bg-transparent pr-8 pl-2.5 outline-none" {...rest}>
        {children}
      </select>
      <ChevronDown className="pointer-events-none absolute right-2.5 size-3.5 text-ink-muted" aria-hidden />
    </div>
  );
});

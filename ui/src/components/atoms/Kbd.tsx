import { ReactNode } from "react";
import { cn } from "@/lib/cn";

export function Kbd({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <kbd
      className={cn(
        "inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-sm border border-divider-2 bg-surface-3 px-1",
        "font-mono text-[10px] text-ink-secondary",
        className,
      )}
    >
      {children}
    </kbd>
  );
}

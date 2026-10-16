import { ReactNode } from "react";
import { cn } from "@/lib/cn";

interface PanelProps {
  title?: ReactNode;
  aside?: ReactNode; // right side of the title bar
  tone?: "default" | "warning" | "danger";
  flush?: boolean; // no padding, for tables
  className?: string;
  children: ReactNode;
}

const TONES = { default: "border-divider", warning: "border-warning/50", danger: "border-danger/50" };

// A bordered box with a hairline title bar: the one way content is grouped.
export function Panel({ title, aside, tone = "default", flush = false, className, children }: PanelProps) {
  return (
    <section className={cn("min-w-0 overflow-hidden rounded-lg border bg-surface-0", TONES[tone], className)}>
      {title && (
        <div className="flex h-10 items-center justify-between gap-3 border-b border-divider bg-surface-1 px-4">
          <h2 className="flex min-w-0 items-center gap-2 text-[13px] font-semibold text-ink-primary">{title}</h2>
          {aside && <div className="flex shrink-0 items-center gap-2 text-[12px] text-ink-muted">{aside}</div>}
        </div>
      )}
      <div className={cn(!flush && "p-4")}>{children}</div>
    </section>
  );
}

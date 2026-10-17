import { cn } from "@/lib/cn";

// How far an epic is: its stories done of all, as a bar and a count.
export function StoryProgress({ done, total, className }: { done: number; total: number; className?: string }) {
  if (!total) return <span className={cn("font-mono text-[11px] text-ink-faint", className)}>no stories yet</span>;
  return (
    <span className={cn("flex items-center gap-2", className)} title={`${done} of ${total} stories done`}>
      <span className="h-1 w-16 overflow-hidden bg-surface-3" aria-hidden>
        <span className="block h-full bg-success" style={{ width: `${(100 * done) / total}%` }} />
      </span>
      <span className="font-mono text-[11px] text-ink-muted">
        {done}/{total}
      </span>
    </span>
  );
}

// EpicBadge marks an epic, flat and small.
export function EpicBadge() {
  return (
    <span className="shrink-0 border border-brand/40 px-1 font-mono text-[10px] leading-4 tracking-wide text-brand uppercase">epic</span>
  );
}

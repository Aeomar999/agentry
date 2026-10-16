import { ReactNode } from "react";
import { cn } from "@/lib/cn";

export interface Segment {
  id: string;
  label: string;
  icon?: ReactNode;
}

// A row of choices where one is on, such as Board or List.
export function SegmentedControl({ segments, active, onChange }: { segments: Segment[]; active: string; onChange: (id: string) => void }) {
  return (
    <div role="radiogroup" className="inline-flex h-7 shrink-0 rounded-md border border-divider p-0.5">
      {segments.map((segment) => (
        <button
          key={segment.id}
          type="button"
          role="radio"
          aria-checked={segment.id === active}
          onClick={() => onChange(segment.id)}
          className={cn(
            "inline-flex cursor-pointer items-center gap-1.5 rounded-sm px-2 text-[12px] font-medium [&>svg]:size-3.5",
            segment.id === active ? "bg-surface-3 text-ink-primary" : "text-ink-muted hover:text-ink-primary",
          )}
        >
          {segment.icon}
          {segment.label}
        </button>
      ))}
    </div>
  );
}

import { MouseEvent } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/cn";

// A label: its name and a dot whose colour comes from the name, so a label
// looks the same everywhere without anyone picking colours.
const DOTS = ["bg-info", "bg-success", "bg-warning", "bg-danger", "bg-brand", "bg-ink-muted"];

function dotFor(label: string): string {
  let hash = 0;
  for (const char of label) hash = (hash * 31 + char.charCodeAt(0)) | 0;
  return DOTS[Math.abs(hash) % DOTS.length];
}

interface LabelChipProps {
  label: string;
  onClick?: () => void; // as when a chip filters by its label
  onRemove?: () => void;
}

export function LabelChip({ label, onClick, onRemove }: LabelChipProps) {
  const stop = (handler?: () => void) => (event: MouseEvent) => {
    event.stopPropagation(); // a chip on a card or row doesn't open the task
    handler?.();
  };
  return (
    <span
      onClick={onClick ? stop(onClick) : undefined}
      className={cn(
        "inline-flex h-[18px] max-w-full shrink-0 items-center gap-1 rounded-sm border border-divider bg-surface-2 pr-1 pl-1.5 text-[11px] text-ink-secondary",
        onClick && "cursor-pointer hover:border-divider-2 hover:text-ink-primary",
      )}
    >
      <span className={cn("size-1.5 shrink-0 rounded-full", dotFor(label))} aria-hidden />
      <span className="truncate">{label}</span>
      {onRemove && (
        <button
          type="button"
          aria-label={`Remove ${label}`}
          onClick={stop(onRemove)}
          className="cursor-pointer text-ink-faint hover:text-ink-primary"
        >
          <X className="size-3" />
        </button>
      )}
    </span>
  );
}

export function LabelList({ labels, onPick }: { labels: string[]; onPick?: (label: string) => void }) {
  if (labels.length === 0) return null;
  return (
    <span className="flex flex-wrap gap-1">
      {labels.map((label) => (
        <LabelChip key={label} label={label} onClick={onPick ? () => onPick(label) : undefined} />
      ))}
    </span>
  );
}

import { cn } from "@/lib/cn";

interface FilterChipProps {
  label: string;
  count?: number;
  active: boolean;
  onClick: () => void;
}

export function FilterChip({ label, count, active, onClick }: FilterChipProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        "inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md border px-2.5 text-[12px] font-medium",
        "transition-colors duration-(--dur-fast)",
        active
          ? "border-divider-2 bg-surface-3 text-ink-primary"
          : "border-divider text-ink-secondary hover:border-divider-2 hover:bg-surface-2",
      )}
    >
      {label}
      {count !== undefined && <span className="font-mono text-[11px] text-ink-muted">{count}</span>}
    </button>
  );
}

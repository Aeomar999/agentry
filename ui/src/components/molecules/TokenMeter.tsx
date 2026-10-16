import { cn } from "@/lib/cn";
import { tokens } from "@/lib/tokens";

// Tokens used against the budget: a thin bar that turns amber near the end
// and red at it.
export function TokenMeter({ used, budget }: { used: number; budget: number }) {
  if (budget <= 0) return <span className="font-mono text-[12px]">{tokens(used)} (no limit)</span>;
  const share = Math.min(1, used / budget);
  return (
    <span className="flex flex-col gap-1">
      <span className="font-mono text-[12px]">
        {tokens(used)} <span className="text-ink-muted">of {tokens(budget)}</span>
      </span>
      <span className="h-1 w-full overflow-hidden rounded-sm bg-surface-3">
        <span
          className={cn("block h-full", share >= 1 ? "bg-danger" : share >= 0.8 ? "bg-warning" : "bg-info")}
          style={{ width: `${share * 100}%` }}
        />
      </span>
    </span>
  );
}

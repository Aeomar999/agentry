import { useLayoutEffect, useRef } from "react";
import { ChevronRight, CircleCheck } from "lucide-react";
import type { Turn } from "@/api/types";
import { cn } from "@/lib/cn";
import { useAgentColor } from "@/live/agent-colors";

// Who the task has passed through, in order: engineer › reviewer › tester.
// The coordinator stands between every step, so it is left out. Turns of the
// same agent in a row are one step with a count. A step opens the activity
// of its latest turn; the agent at work now pulses. A long path scrolls,
// and opens at its end, where the work is now.

interface HandoffPathProps {
  turns: Turn[];
  coordinator: string;
  done: boolean;
  onOpenTurn: (turn: string) => void;
}

interface Stage {
  agent: string;
  turns: Turn[];
}

export function HandoffPath({ turns, coordinator, done, onOpenTurn }: HandoffPathProps) {
  const colorOf = useAgentColor();
  const path = useRef<HTMLElement>(null);
  useLayoutEffect(() => {
    if (path.current) path.current.scrollLeft = path.current.scrollWidth;
  }, [turns.length]);
  const stages: Stage[] = [];
  for (const turn of turns) {
    if (turn.agent === coordinator) continue;
    const last = stages[stages.length - 1];
    if (last?.agent === turn.agent) last.turns.push(turn);
    else stages.push({ agent: turn.agent, turns: [turn] });
  }
  if (stages.length === 0) return null;

  return (
    <nav ref={path} aria-label="Handoffs" className="flex min-w-0 scrollbar-thin items-center gap-1 overflow-x-auto px-6 py-2.5">
      {stages.map((stage, i) => {
        const latest = stage.turns[stage.turns.length - 1];
        const working = !latest.ended;
        const failed = Boolean(latest.issue);
        return (
          <span key={i} className="flex shrink-0 items-center gap-1">
            {i > 0 && <ChevronRight className="size-3.5 text-ink-faint" aria-hidden />}
            <button
              type="button"
              onClick={() => onOpenTurn(latest.id)}
              title={stage.turns.map((t) => t.id).join(", ")}
              className={cn(
                "inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md border px-2.5 font-mono text-[12px]",
                "transition-colors duration-(--dur-fast) hover:bg-surface-2",
                working
                  ? "border-success/60 text-ink-primary"
                  : failed
                    ? "border-danger/50 text-ink-secondary"
                    : "border-divider text-ink-secondary",
              )}
            >
              <span
                className={cn("size-2 shrink-0 rounded-full", working && "animate-pulse")}
                style={{ background: colorOf(stage.agent) }}
                aria-hidden
              />
              {stage.agent}
              {stage.turns.length > 1 && <span className="text-[11px] text-ink-faint">×{stage.turns.length}</span>}
            </button>
          </span>
        );
      })}
      {done && (
        <span className="flex shrink-0 items-center gap-1">
          <ChevronRight className="size-3.5 text-ink-faint" aria-hidden />
          <span className="inline-flex h-7 items-center gap-1.5 px-1 text-[12px] font-medium text-success [&>svg]:size-3.5">
            <CircleCheck />
            done
          </span>
        </span>
      )}
    </nav>
  );
}

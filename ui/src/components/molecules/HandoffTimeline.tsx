import { ArrowRight } from "lucide-react";
import type { Step } from "@/api/types";
import { clockTime } from "@/lib/time";
import { cn } from "@/lib/cn";
import { AgentName } from "./AgentName";
import { useAgentColor } from "@/live/agent-colors";

// The handoff trail: every message between you, the coordinator and the
// agents, oldest first, as a vertical line of steps.
export function HandoffTimeline({ steps }: { steps: Step[] }) {
  const colorOf = useAgentColor(); // each step's dot in the colour of who spoke
  return (
    <ol className="relative flex flex-col gap-4 before:absolute before:top-2 before:bottom-2 before:left-[5px] before:w-px before:bg-divider">
      {steps.map((step, i) => (
        <li key={i} className="relative pl-6">
          <span
            className={cn(
              "absolute top-1.5 left-0 size-[11px] rounded-full border-2 border-surface-0",
              (step.to === "you" || step.from === "you") && "bg-warning",
            )}
            style={step.to === "you" || step.from === "you" ? undefined : { background: colorOf(step.from) }}
            aria-hidden
          />
          <div className="flex flex-wrap items-center gap-1.5 text-[12px]">
            <AgentName name={step.from} className="font-semibold text-ink-primary" />
            <ArrowRight className="size-3 text-ink-faint" aria-hidden />
            <AgentName name={step.to} className="font-semibold text-ink-primary" />
            <span className="font-mono text-ink-faint">{clockTime(step.at)}</span>
          </div>
          {step.text && (
            <div className="mt-1.5 rounded-md border border-divider bg-surface-1 px-3 py-2 text-[13px] leading-5 break-words whitespace-pre-wrap text-ink-secondary">
              {step.text}
            </div>
          )}
        </li>
      ))}
    </ol>
  );
}

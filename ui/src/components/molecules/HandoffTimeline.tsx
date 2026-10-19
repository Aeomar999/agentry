import { ArrowRight, ChevronRight } from "lucide-react";
import type { Step, Turn } from "@/api/types";
import { Badge } from "@/components/atoms/Badge";
import { clockTime, duration } from "@/lib/time";
import { tokens } from "@/lib/tokens";
import { cn } from "@/lib/cn";
import { AgentName } from "./AgentName";
import { Mono } from "./Mono";
import { useAgentColor } from "@/live/agent-colors";

// The handoff trail: every message between you, the coordinator and the
// agents, newest first, as a vertical line of steps. A step opens the
// activity of the turn it led to: what the agent it went to did with it.
// What is said to you leads nowhere, so it opens the turn that said it.

interface HandoffTimelineProps {
  steps: Step[];
  turns: Turn[];
  actionsIn: (turn: string) => number;
  working: string; // the agent at work now, "" when none
  onOpen: (turn: string) => void;
  onWatch: () => void; // the working agent's terminal
}

export function HandoffTimeline({ steps, turns, actionsIn, working, onOpen, onWatch }: HandoffTimelineProps) {
  const colorOf = useAgentColor(); // each step's dot in the colour of who spoke
  const turnOf = new Map(turns.map((t) => [t.id, t]));
  return (
    <ol className="relative flex flex-col gap-3 before:absolute before:top-3 before:bottom-3 before:left-[5px] before:w-px before:bg-divider">
      {working && (
        <li className="relative pl-6">
          <span className="absolute top-2 left-0 size-[11px] animate-pulse rounded-full border-2 border-surface-0 bg-success" aria-hidden />
          <button
            type="button"
            onClick={onWatch}
            className="flex cursor-pointer items-center gap-1.5 py-1 text-[12px] text-success hover:underline"
          >
            <AgentName name={working} className="font-semibold" />
            is working now · watch the terminal
          </button>
        </li>
      )}
      {[...steps].reverse().map((step, i) => {
        const turn = turnOf.get(step.turn);
        const yours = step.to === "you" || step.from === "you";
        return (
          <li key={steps.length - i} className="relative pl-6">
            <span
              className={cn("absolute top-3 left-0 size-[11px] rounded-full border-2 border-surface-0", yours && "bg-warning")}
              style={yours ? undefined : { background: colorOf(step.from) }}
              aria-hidden
            />
            <button
              type="button"
              disabled={!turn}
              onClick={() => turn && onOpen(turn.id)}
              className="group block w-full cursor-pointer rounded-md border border-divider bg-surface-0 text-left transition-colors duration-(--dur-fast) hover:border-divider-2 hover:bg-surface-1 disabled:cursor-default disabled:hover:border-divider disabled:hover:bg-surface-0"
            >
              <div className="flex flex-wrap items-center gap-1.5 px-3 pt-2 text-[12px]">
                <AgentName name={step.from} className="font-semibold text-ink-primary" />
                <ArrowRight className="size-3 text-ink-faint" aria-hidden />
                <AgentName name={step.to} className="font-semibold text-ink-primary" />
                <Mono className="ml-auto text-[11px] text-ink-faint">{clockTime(step.at)}</Mono>
              </div>
              {step.text && (
                <p className="line-clamp-6 px-3 pt-1.5 pb-2 text-[13px] leading-5 break-words whitespace-pre-wrap text-ink-secondary">
                  {step.text}
                </p>
              )}
              {turn && <TurnLine turn={turn} actions={actionsIn(turn.id)} ledTo={step.to !== "you"} />}
            </button>
          </li>
        );
      })}
    </ol>
  );
}

// The turn a step belongs to, in one line under it.
function TurnLine({ turn, actions, ledTo }: { turn: Turn; actions: number; ledTo: boolean }) {
  return (
    <div className="flex items-center gap-2.5 border-t border-divider px-3 py-1.5 text-[11px] text-ink-muted">
      <span className="text-ink-faint">{ledTo ? "Led to" : "From"}</span>
      <Mono className="text-[11px]">{turn.id}</Mono>
      <AgentName name={turn.agent} className="text-[11px]" />
      {turn.issue ? (
        <Badge variant="danger">issue</Badge>
      ) : (
        !turn.ended && (turn.limited ? <Badge variant="warning">usage limit</Badge> : <Badge variant="info">running</Badge>)
      )}
      {turn.ended && <Mono className="text-[11px]">{duration(turn.startedAt, turn.lastEventAt)}</Mono>}
      <span>
        {actions} {actions === 1 ? "action" : "actions"}
      </span>
      {turn.tokens > 0 && <Mono className="text-[11px]">{tokens(turn.tokens)} tokens</Mono>}
      <span className="ml-auto inline-flex items-center gap-0.5 text-ink-faint group-hover:text-ink-primary">
        Activity
        <ChevronRight className="size-3.5" aria-hidden />
      </span>
    </div>
  );
}

import { useState } from "react";
import { ChevronRight, ListChecks, Square, SquareCheck } from "lucide-react";
import type { Turn } from "@/api/types";
import { AgentName } from "@/components/molecules/AgentName";
import { Mono } from "@/components/molecules/Mono";
import { PlanEntry, planSections } from "@/domains/tasks/plan";
import { clockTime } from "@/lib/time";
import { cn } from "@/lib/cn";

// The plans on a task, down the left of its page on every tab (lib/plan
// groups them). The task's plan, the coordinator's, is pinned at the top;
// under it each agent's plan still to be finished, open while its agent
// works and closed while it doesn't; then Earlier, greyed: plans that are
// finished or were replaced.

interface PlanRailProps {
  turns: Turn[];
  coordinator: string;
  onOpenTurn: (turn: string) => void;
}

export function PlanRail({ turns, coordinator, onOpenTurn }: PlanRailProps) {
  const { task, current, earlier, writing } = planSections(turns, coordinator);
  const nothing = !task && current.length === 0 && earlier.length === 0 && !writing;
  return (
    <div className="flex flex-col">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-divider px-4 text-[13px] font-semibold text-ink-primary [&>svg]:size-4 [&>svg]:text-ink-muted">
        <ListChecks />
        Plan
      </div>
      {nothing && (
        <p className="px-4 py-3 text-[12px] leading-5 text-ink-muted">
          No plan yet. Each agent writes its plan before it starts, and it shows here as it works.
        </p>
      )}
      {task && (
        <>
          <Heading>Task</Heading>
          <PlanBlock key={`${task.turn.id}-${task.open}`} entry={task} onOpenTurn={onOpenTurn} />
        </>
      )}
      {(writing || current.length > 0) && <Heading>Agents</Heading>}
      {writing && (
        <div className="flex items-center gap-2 border-b border-divider px-4 py-2.5 text-[12px] text-ink-muted">
          <span className="size-2 animate-pulse rounded-full bg-success" aria-hidden />
          <AgentName name={writing.agent} className="font-semibold text-ink-primary" />
          is writing its plan
        </div>
      )}
      {current.map((entry) => (
        <PlanBlock key={`${entry.turn.id}-${entry.open}`} entry={entry} onOpenTurn={onOpenTurn} />
      ))}
      {earlier.length > 0 && <Heading>Earlier</Heading>}
      {earlier.map((entry) => (
        <PlanBlock key={entry.turn.id} entry={entry} onOpenTurn={onOpenTurn} greyed />
      ))}
    </div>
  );
}

function Heading({ children }: { children: string }) {
  return <div className="px-4 pt-4 pb-1 text-[11px] font-medium tracking-wider text-ink-faint uppercase">{children}</div>;
}

function PlanBlock({ entry, onOpenTurn, greyed = false }: { entry: PlanEntry; onOpenTurn: (turn: string) => void; greyed?: boolean }) {
  const [open, setOpen] = useState(entry.open);
  const { turn, plan, live, note } = entry;
  const next = live ? plan.lines.findIndex((line) => line.step && !line.done) : -1; // the step it is on
  const share = plan.steps ? plan.done / plan.steps : 0;
  const stopped = note.startsWith("stopped");
  return (
    <section className={cn("border-b border-divider", greyed && "opacity-60 hover:opacity-100")}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex w-full cursor-pointer flex-col gap-0.5 px-4 py-2.5 text-left text-[12px] hover:bg-surface-1"
      >
        <span className="flex w-full items-center gap-2">
          <ChevronRight
            className={cn("size-3.5 shrink-0 text-ink-faint transition-transform duration-(--dur-fast)", open && "rotate-90")}
          />
          {live && <span className="size-2 shrink-0 animate-pulse rounded-full bg-success" aria-label="being worked on" />}
          <AgentName name={turn.agent} className="font-semibold text-ink-primary" />
          <Mono className="text-[11px] text-ink-faint">{turn.id}</Mono>
          <Mono className={cn("ml-auto text-[11px]", plan.done === plan.steps && plan.steps > 0 ? "text-success" : "text-ink-muted")}>
            {plan.done}/{plan.steps}
          </Mono>
        </span>
        {note && (
          <span title={stopped ? turn.issue : undefined} className={cn("pl-5.5 text-[11px]", stopped ? "text-warning" : "text-ink-muted")}>
            {note}
          </span>
        )}
      </button>
      {open && (
        <div className="px-4 pb-3">
          <div className="mb-2.5 h-1 overflow-hidden rounded-sm bg-surface-3">
            <div className={cn("h-full", share >= 1 ? "bg-success" : "bg-info")} style={{ width: `${share * 100}%` }} />
          </div>
          <ol className="flex flex-col gap-1.5">
            {plan.lines.map((line, i) => (
              <li key={i} className="flex gap-2 text-[12px] leading-[18px]" style={{ paddingLeft: line.depth * 14 }}>
                {line.step ? (
                  i === next ? (
                    <span className="mt-[5px] ml-[3px] size-2 shrink-0 animate-pulse rounded-full bg-success" aria-label="in progress" />
                  ) : line.done ? (
                    <SquareCheck className="mt-0.5 size-3.5 shrink-0 text-success" aria-label="done" />
                  ) : (
                    <Square className="mt-0.5 size-3.5 shrink-0 text-ink-faint" aria-label="to do" />
                  )
                ) : null}
                <span
                  className={cn(
                    "min-w-0 break-words",
                    !line.step && "font-semibold text-ink-secondary",
                    line.step && (line.done ? "text-ink-muted" : i === next ? "font-medium text-ink-primary" : "text-ink-secondary"),
                  )}
                >
                  {line.text}
                </span>
              </li>
            ))}
          </ol>
          <button
            type="button"
            onClick={() => onOpenTurn(turn.id)}
            className="mt-3 inline-flex cursor-pointer items-center gap-1 text-[11px] text-ink-muted hover:text-ink-primary"
          >
            Written in {turn.id} at {clockTime(turn.startedAt)} · activity
            <ChevronRight className="size-3" />
          </button>
        </div>
      )}
    </section>
  );
}

import { DragEvent } from "react";
import type { TaskSummary } from "@/api/types";
import { cn } from "@/lib/cn";
import { LabelList } from "./LabelChip";
import { AgentName } from "./AgentName";
import { timeAgo } from "@/lib/time";
import { tokens } from "@/lib/tokens";

interface TaskCardProps {
  task: TaskSummary;
  onPickLabel: (label: string) => void;
  coordinator: string;
  onOpen: () => void;
  draggable: boolean;
  dragging: boolean;
  onDragStart: (event: DragEvent) => void;
  onDragEnd: () => void;
}

// One task on the board: what it is, and what is happening to it now.
export function TaskCard({ task, onPickLabel, coordinator, onOpen, draggable, dragging, onDragStart, onDragEnd }: TaskCardProps) {
  return (
    // A div acting as a button: browsers don't all start a drag from a <button>.
    <div
      role="button"
      tabIndex={0}
      data-task={task.id}
      onClick={onOpen}
      onKeyDown={(event) => event.key === "Enter" && onOpen()}
      draggable={draggable}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      className={cn(
        "flex w-full cursor-pointer flex-col gap-1.5 rounded-md border border-divider bg-surface-0 px-3 py-2.5 text-left",
        "transition-colors duration-(--dur-fast) hover:border-divider-2",
        draggable && "active:cursor-grabbing",
        dragging && "opacity-40",
      )}
    >
      <span className="line-clamp-2 text-[13px] leading-[18px] font-medium text-ink-primary">{task.title}</span>
      <span className="line-clamp-2 text-[12px] leading-4">{nowLine(task, coordinator)}</span>
      <LabelList labels={task.labels} onPick={onPickLabel} />
      <span className="flex items-center gap-2 font-mono text-[11px] whitespace-nowrap text-ink-faint">
        {task.id}
        <span className={cn("ml-auto", task.overBudget && "text-danger")}>{task.tokensUsed > 0 && tokens(task.tokensUsed)}</span>
        <span>{task.turns} turns</span>
        <span title={task.lastActiveAt}>{timeAgo(task.lastActiveAt)}</span>
      </span>
    </div>
  );
}

function nowLine(task: TaskSummary, coordinator: string) {
  if (task.status === "done") return <span className="text-ink-muted">Done</span>;
  if (task.status === "needs you") return <span className="text-warning">{task.needsYou}</span>;
  if (task.status === "held") return <span className="text-ink-muted">Waiting until you start it</span>;
  if (task.currentAgent === coordinator) return <span className="text-ink-muted">The coordinator is deciding</span>;
  if (task.currentAgent)
    return (
      <span className="inline-flex items-center gap-1.5 text-ink-secondary">
        <AgentName name={task.currentAgent} dotOnly />
        {task.currentAgent} is working
      </span>
    );
  return <span className="text-ink-muted">Setting up</span>;
}

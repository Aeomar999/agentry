import { DragEvent, Fragment, useState } from "react";
import { Send } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { TextArea } from "@/components/atoms/Input";
import { Dialog } from "@/components/molecules/Dialog";
import { TaskCard } from "@/components/molecules/TaskCard";
import { AgentName } from "@/components/molecules/AgentName";
import { api } from "@/api/client";
import type { Agent, TaskSummary } from "@/api/types";
import { cn } from "@/lib/cn";
import { useToast } from "./Toasts";
import { rankAt } from "@/lib/rank";

// The board's columns are the stages of the work, the same whichever agents
// you have: Backlog (shown when you ask), To do, In progress, Review, Done.
// Agents jump in where their work belongs: one whose column is "review"
// (a reviewer, a tester) puts the task in Review, any other in In progress.
// A card goes onto any column, and that tells the coordinator what you want
// (direct in internal/web/board_api.go): it still makes every move. Where in
// the column you drop it is its place in your priority order. A task waiting
// for you stays where it is, marked, and its question is in the dock.

interface Column {
  id: string; // also what a card dropped here asks for
  title: string;
}

const BACKLOG = "backlog";
const DONE = "done";

const COLUMNS: Column[] = [
  { id: BACKLOG, title: "Backlog" },
  { id: "todo", title: "To do" },
  { id: "in-progress", title: "In progress" },
  { id: "review", title: "Review" },
  { id: DONE, title: "Done" },
];

function columnOf(task: TaskSummary, agents: Agent[]): string {
  if (task.status === "done") return DONE;
  if (task.parked) return task.parked;
  return agents.find((a) => a.name === task.stage)?.column === "review" ? "review" : "in-progress";
}

// slotAt is where in a column a card being moved would land: the number of
// the other cards above the pointer.
function slotAt(column: HTMLElement, y: number, movingId: string): number {
  const cards = [...column.querySelectorAll<HTMLElement>("[data-task]")].filter((card) => card.dataset.task !== movingId);
  return cards.filter((card) => {
    const box = card.getBoundingClientRect();
    return box.top + box.height / 2 < y;
  }).length;
}

// slotIndex is the slot just above the card at index, or -1 above the card
// being moved, which has no slot of its own.
function slotIndex(cards: TaskSummary[], movingId: string, index: number): number {
  if (cards[index].id === movingId) return -1;
  return cards.slice(0, index).filter((card) => card.id !== movingId).length;
}

function DropLine() {
  return <div className="h-px shrink-0 bg-brand" aria-hidden />;
}

interface TaskBoardProps {
  tasks: TaskSummary[];
  agents: Agent[];
  showBacklog: boolean;
  onOpen: (task: TaskSummary) => void;
  onPickLabel: (label: string) => void;
}

export function TaskBoard({ tasks, agents, showBacklog, onOpen, onPickLabel }: TaskBoardProps) {
  const notify = useToast();
  const columns = COLUMNS.filter((column) => showBacklog || column.id !== BACKLOG);
  const coordinator = agents.find((a) => a.coordinates)?.name ?? "coordinator";
  const [dragged, setDragged] = useState<TaskSummary | null>(null);
  const [over, setOver] = useState("");
  const [dropped, setDropped] = useState<{ task: TaskSummary; to: string; rank: number | null } | null>(null);
  const [slot, setSlot] = useState<{ column: string; at: number } | null>(null); // where the card would land

  // Onto another column: an instruction to the coordinator, after the dialog.
  // Within its own column: a new place in your priority order, at once.
  const isHome = (column: Column) => dragged !== null && column.id === columnOf(dragged, agents);
  const canDrop = (column: Column) => dragged !== null && !(dragged.status === "done" && column.id === DONE);

  function onDragOver(event: DragEvent<HTMLElement>, column: Column) {
    if (!canDrop(column)) return;
    event.preventDefault();
    setOver(isHome(column) ? "" : column.id);
    setSlot({ column: column.id, at: slotAt(event.currentTarget, event.clientY, dragged!.id) });
  }

  function onDrop(event: DragEvent, column: Column, cards: TaskSummary[]) {
    event.preventDefault();
    if (dragged && slot && canDrop(column)) {
      const rank = rankAt(cards, dragged, slot.at);
      if (isHome(column)) {
        if (rank !== null) saveRank(dragged, rank);
      } else {
        setDropped({ task: dragged, to: column.id, rank });
      }
    }
    setOver("");
    setDragged(null);
    setSlot(null);
  }

  async function saveRank(task: TaskSummary, rank: number) {
    try {
      await api("PUT", `/api/tasks/${encodeURIComponent(task.id)}/rank`, { rank });
    } catch (failure) {
      notify("danger", "The order wasn't saved", (failure as Error).message);
    }
  }

  return (
    <>
      <div className="flex min-h-0 flex-1 scrollbar-thin gap-3 overflow-x-auto p-4">
        {columns.map((column) => {
          const cards = tasks.filter((task) => columnOf(task, agents) === column.id);
          const droppable = canDrop(column) && !isHome(column);
          return (
            <section
              key={column.id}
              aria-label={column.title}
              onDragOver={(event) => onDragOver(event, column)}
              onDragLeave={(event) => {
                if (event.currentTarget.contains(event.relatedTarget as Node)) return;
                setOver((current) => (current === column.id ? "" : current));
                setSlot((current) => (current?.column === column.id ? null : current));
              }}
              onDrop={(event) => onDrop(event, column, cards)}
              className={cn(
                "flex min-w-56 flex-1 flex-col rounded-lg border bg-surface-1 transition-colors duration-(--dur-fast)",
                over === column.id ? "border-dashed border-brand" : droppable ? "border-dashed border-divider-2" : "border-divider",
              )}
            >
              <h2 className="flex h-10 shrink-0 items-center gap-2 border-b border-divider px-3 text-[12px] font-semibold">
                <span className="text-ink-primary">{column.title}</span>
                <span className="font-mono text-[11px] font-normal text-ink-faint">{cards.length}</span>
                {/* who is working in this column right now */}
                <span className="ml-auto flex items-center gap-1">
                  {[...new Set(cards.map((task) => task.currentAgent).filter((agent) => agent && agent !== coordinator))].map((agent) => (
                    <span key={agent} title={`${agent} is working here`}>
                      <AgentName name={agent} dotOnly />
                    </span>
                  ))}
                </span>
              </h2>
              <div className="flex min-h-24 scrollbar-thin flex-col gap-2 overflow-y-auto p-2">
                {cards.map((task, index) => (
                  <Fragment key={task.id}>
                    {slot?.column === column.id && slot.at === slotIndex(cards, dragged!.id, index) && <DropLine />}
                    <TaskCard
                      task={task}
                      onPickLabel={onPickLabel}
                      coordinator={coordinator}
                      onOpen={() => onOpen(task)}
                      draggable
                      dragging={dragged?.id === task.id}
                      onDragStart={(event) => {
                        event.dataTransfer.effectAllowed = "move";
                        event.dataTransfer.setData("text/plain", task.id);
                        setDragged(task);
                      }}
                      onDragEnd={() => {
                        setDragged(null);
                        setOver("");
                        setSlot(null);
                      }}
                    />
                  </Fragment>
                ))}
                {slot?.column === column.id && slot.at === cards.filter((card) => card.id !== dragged!.id).length && <DropLine />}
                {cards.length === 0 && <p className="px-1 py-3 text-center text-[11px] text-ink-faint">Nothing here</p>}
              </div>
            </section>
          );
        })}
      </div>
      {dropped && (
        <DirectDialog
          task={dropped.task}
          to={dropped.to}
          onClose={() => setDropped(null)}
          onSent={() => {
            if (dropped.rank !== null) saveRank(dropped.task, dropped.rank);
            const parked = dropped.to === "todo" || dropped.to === "backlog";
            notify(
              "success",
              parked ? `Parked ${dropped.task.id}` : `Told the coordinator about ${dropped.task.id}`,
              parked ? "It waits there until you move it on." : "It acts on it at its next turn.",
            );
            setDropped(null);
          }}
          onFailed={(reason) => notify("danger", "That wasn't sent", reason)}
        />
      )}
    </>
  );
}

interface DirectDialogProps {
  task: TaskSummary;
  to: string; // an agent's name, or "done"
  onClose: () => void;
  onSent: () => void;
  onFailed: (reason: string) => void;
}

// What a drop asks, before it is sent: you can add a note for the coordinator.
function DirectDialog({ task, to, onClose, onSent, onFailed }: DirectDialogProps) {
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const reopen = task.status === "done" ? "Reopen and " : "";
  const titles: Record<string, string> = {
    backlog: `${reopen}park ${task.id} in the Backlog?`,
    todo: `${reopen}park ${task.id} in To do?`,
    "in-progress": task.parked ? `Start ${task.id}?` : `${reopen}send ${task.id} back for more work?`,
    review: `${reopen}have ${task.id} reviewed next?`,
    done: `Ask the coordinator to wrap up ${task.id}?`,
  };
  const title = titles[to].charAt(0).toUpperCase() + titles[to].slice(1);

  async function send() {
    setBusy(true);
    try {
      await api("POST", `/api/tasks/${encodeURIComponent(task.id)}/direct`, { to, note });
      onSent();
    } catch (failure) {
      onFailed((failure as Error).message);
      setBusy(false);
    }
  }

  return (
    <Dialog title={title} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <p className="text-[12px] text-ink-muted">
          {to === "todo" || to === "backlog"
            ? "The coordinator leaves a parked task alone until you move it on. A turn already running finishes first."
            : task.status === "done"
              ? "It goes back to work in its worktree, kept or made again, with your words to the coordinator."
              : task.hasQuestion
                ? "The coordinator asked you something, so this goes as your answer."
                : task.parked && to === "in-progress"
                  ? "The coordinator picks it up at once."
                  : task.currentAgent && task.currentAgent !== "coordinator"
                    ? `${task.currentAgent} finishes its turn first; the coordinator reads this next.`
                    : "The coordinator reads this at its next turn."}
        </p>
        <TextArea
          autoFocus
          rows={3}
          value={note}
          onChange={(event) => setNote(event.target.value)}
          onKeyDown={(event) => event.key === "Enter" && (event.metaKey || event.ctrlKey) && send()}
          placeholder="A note for the coordinator (optional)"
        />
        <div className="flex gap-2">
          <Button variant="primary" icon={<Send />} disabled={busy} onClick={send}>
            Send
          </Button>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </div>
    </Dialog>
  );
}

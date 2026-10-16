import { DragEvent, useState } from "react";
import { Send } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { TextArea } from "@/components/atoms/Input";
import { Dialog } from "@/components/molecules/Dialog";
import { TaskCard } from "@/components/molecules/TaskCard";
import { api } from "@/api/client";
import type { Agent, TaskSummary } from "@/api/types";
import { cn } from "@/lib/cn";
import { useToast } from "./Toasts";

// The board: To do, Needs you, a column per agent that takes work, Done. A
// task sits in the column of the agent the coordinator last handed it to,
// so new agents get a column of their own. The coordinator moves the
// cards: dragging one onto an agent or Done tells the coordinator what you
// want, and it acts on it at its next turn.

interface Column {
  id: string;
  title: string;
  dropTo?: string; // where a dropped card asks the coordinator to send it
}

const TODO = "todo";
const NEEDS_YOU = "needs-you";
const DONE = "done";

function columnsFor(agents: Agent[]): Column[] {
  return [
    { id: TODO, title: "To do" },
    { id: NEEDS_YOU, title: "Needs you" }, // second, so it is always in view
    ...agents.filter((a) => !a.coordinates).map((a) => ({ id: a.name, title: a.name, dropTo: a.name })),
    { id: DONE, title: "Done", dropTo: "done" },
  ];
}

function columnOf(task: TaskSummary, columns: Column[]): string {
  if (task.status === "done") return DONE;
  if (task.status === "needs you") return NEEDS_YOU;
  return columns.some((c) => c.id === task.stage) ? task.stage : TODO;
}

interface TaskBoardProps {
  tasks: TaskSummary[];
  agents: Agent[];
  onOpen: (task: TaskSummary) => void;
}

export function TaskBoard({ tasks, agents, onOpen }: TaskBoardProps) {
  const notify = useToast();
  const columns = columnsFor(agents);
  const coordinator = agents.find((a) => a.coordinates)?.name ?? "coordinator";
  const [dragged, setDragged] = useState<TaskSummary | null>(null);
  const [over, setOver] = useState("");
  const [dropped, setDropped] = useState<{ task: TaskSummary; to: string } | null>(null);

  const canDrop = (column: Column) => dragged !== null && column.dropTo !== undefined && column.id !== columnOf(dragged, columns);

  function onDrop(event: DragEvent, column: Column) {
    event.preventDefault();
    setOver("");
    if (dragged && canDrop(column)) setDropped({ task: dragged, to: column.dropTo! });
    setDragged(null);
  }

  return (
    <>
      <div className="flex min-h-0 flex-1 scrollbar-thin gap-3 overflow-x-auto p-4">
        {columns.map((column) => {
          const cards = tasks.filter((task) => columnOf(task, columns) === column.id);
          const droppable = canDrop(column);
          return (
            <section
              key={column.id}
              aria-label={column.title}
              onDragOver={(event) => {
                if (!droppable) return;
                event.preventDefault();
                setOver(column.id);
              }}
              onDragLeave={() => setOver((current) => (current === column.id ? "" : current))}
              onDrop={(event) => onDrop(event, column)}
              className={cn(
                "flex min-w-56 flex-1 flex-col rounded-lg border bg-surface-1 transition-colors duration-(--dur-fast)",
                over === column.id ? "border-dashed border-brand" : droppable ? "border-dashed border-divider-2" : "border-divider",
              )}
            >
              <h2 className="flex h-10 shrink-0 items-center gap-2 border-b border-divider px-3 text-[12px] font-semibold">
                <span
                  className={cn(
                    column.id === NEEDS_YOU && cards.length > 0 ? "text-warning" : "text-ink-primary",
                    column.dropTo && column.id !== DONE && "font-mono",
                  )}
                >
                  {column.title}
                </span>
                <span className="font-mono text-[11px] font-normal text-ink-faint">{cards.length}</span>
              </h2>
              <div className="flex min-h-24 scrollbar-thin flex-col gap-2 overflow-y-auto p-2">
                {cards.map((task) => (
                  <TaskCard
                    key={task.id}
                    task={task}
                    coordinator={coordinator}
                    onOpen={() => onOpen(task)}
                    draggable={task.status !== "done"}
                    dragging={dragged?.id === task.id}
                    onDragStart={(event) => {
                      event.dataTransfer.effectAllowed = "move";
                      event.dataTransfer.setData("text/plain", task.id);
                      setDragged(task);
                    }}
                    onDragEnd={() => {
                      setDragged(null);
                      setOver("");
                    }}
                  />
                ))}
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
            notify("success", `Told the coordinator about ${dropped.task.id}`, "It acts on it at its next turn.");
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
  const title = to === "done" ? `Ask the coordinator to wrap up ${task.id}?` : `Hand ${task.id} to ${to}?`;

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
          {task.hasQuestion
            ? "The coordinator asked you something, so this goes as your answer."
            : task.status === "needs you"
              ? "It is waiting on you for something else first; the coordinator reads this after."
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

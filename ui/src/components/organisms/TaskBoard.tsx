import { DragEvent, Fragment, useState } from "react";
import { Send } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { TextArea } from "@/components/atoms/Input";
import { Dialog } from "@/components/molecules/Dialog";
import { TaskCard } from "@/components/molecules/TaskCard";
import { AgentName } from "@/components/molecules/AgentName";
import { api } from "@/api/client";
import type { Agent, BoardColumn, TaskSummary } from "@/api/types";
import { cn } from "@/lib/cn";
import { useToast } from "./Toasts";
import { rankAt } from "@/lib/rank";
import { DropLine, slotAt, slotIndex } from "@/lib/drag";
import { DONE_DAYS, onBoard } from "@/lib/epics";
import { COLUMN_TITLES } from "@/lib/columns";

// The board's columns are the stages of the work: Backlog (shown when you
// ask), To do, In progress, Review, Shipping, Done. The server says which
// columns there are (Shipping only while some agent ships work or a task is
// in it) and which column a task is in. Moving a card onto another column is
// final and takes effect at once (direct in internal/web/board_api.go): Done
// finishes it, To do and the Backlog park it, In progress, Review and
// Shipping send it to an agent that builds, checks or ships it; a team with
// no agent that ships work refuses Shipping, so its cards can't be dropped
// there.
// The card shows in its new column as soon as you send the move. Where in
// the column you drop it is its place in your priority order. A task waiting
// for you stays where it is, marked, and its question is in the dock.

interface Column {
  id: BoardColumn; // also what a card dropped here asks for
  title: string;
}

const BACKLOG = "backlog";
const SHIPPING = "shipping";
const DONE = "done";

// A move you sent, shown until fresh data for the task comes after it.
interface Move {
  to: BoardColumn;
  sent: boolean; // the server has it
}

interface TaskBoardProps {
  columns: BoardColumn[]; // in order: GET /api/features's board
  tasks: TaskSummary[];
  agents: Agent[];
  showBacklog: boolean;
  onOpen: (task: TaskSummary) => void;
  onPickLabel: (label: string) => void;
  onPickEpic: (epic: string) => void;
  everyTask: TaskSummary[]; // unfiltered: for the epics' titles
}

export function TaskBoard({ columns: listed, tasks, agents, showBacklog, onOpen, onPickLabel, onPickEpic, everyTask }: TaskBoardProps) {
  const byId = new Map(everyTask.map((task) => [task.id, task]));
  const onTheBoard = tasks.filter((task) => task.kind !== "epic" && onBoard(task, byId));
  const older = tasks.filter((task) => task.kind !== "epic").length - onTheBoard.length;
  const notify = useToast();
  const columns: Column[] = listed.filter((id) => showBacklog || id !== BACKLOG).map((id) => ({ id, title: COLUMN_TITLES[id] }));
  const coordinator = agents.find((a) => a.coordinates)?.name ?? "coordinator";
  const [dragged, setDragged] = useState<TaskSummary | null>(null);
  const [over, setOver] = useState("");
  const [dropped, setDropped] = useState<{ task: TaskSummary; to: BoardColumn; rank: number | null } | null>(null);
  const [slot, setSlot] = useState<{ column: string; at: number } | null>(null); // where the card would land
  const [moves, setMoves] = useState<Record<string, Move>>({});
  const [seen, setSeen] = useState(everyTask);
  if (seen !== everyTask) {
    // Fresh data: it shows the moves the server has.
    setSeen(everyTask);
    setMoves((current) => Object.fromEntries(Object.entries(current).filter(([, move]) => !move.sent)));
  }
  const columnOf = (task: TaskSummary) => moves[task.id]?.to ?? task.column;

  // Onto another column: a move, after the dialog, shown at once.
  // Within its own column: a new place in your priority order, at once.
  const isHome = (column: Column) => dragged !== null && column.id === columnOf(dragged);
  const canDrop = (column: Column) =>
    dragged !== null && !(dragged.status === "done" && column.id === DONE) && !(!dragged.canShip && column.id === SHIPPING);

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

  async function move(task: TaskSummary, to: BoardColumn, rank: number | null, note: string) {
    const mine: Move = { to, sent: false };
    // Settles this move only: a newer move of the same card is left as it is.
    const settle = (keep: boolean) =>
      setMoves((current) => {
        if (current[task.id] !== mine) return current;
        const next = { ...current };
        if (keep) next[task.id] = { ...mine, sent: true };
        else delete next[task.id];
        return next;
      });
    setMoves((current) => ({ ...current, [task.id]: mine }));
    try {
      await api("POST", `/api/tasks/${encodeURIComponent(task.id)}/direct`, { to, note });
    } catch (failure) {
      settle(false);
      notify("danger", "That wasn't sent", (failure as Error).message);
      return;
    }
    settle(true);
    if (rank !== null) saveRank(task, rank);
    const toasts: Record<BoardColumn, [string, string]> = {
      backlog: [`Parked ${task.id} in the Backlog`, "It waits there until you move it on."],
      todo: [`Parked ${task.id} in To do`, "It waits there until you move it on."],
      "in-progress": [`Moved ${task.id} to In progress`, "The coordinator hands it to an agent that builds it."],
      review: [`Moved ${task.id} to Review`, "The coordinator has it checked next."],
      shipping: [`Moved ${task.id} to Shipping`, "The coordinator hands it to an agent that ships it."],
      done: [`${task.id} is done`, "The coordinator writes its summary."],
    };
    notify("success", ...toasts[to]);
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
      <div className="flex min-h-0 min-w-0 flex-1 scrollbar-thin gap-3 overflow-x-auto p-4">
        {columns.map((column) => {
          let cards = onTheBoard.filter((task) => columnOf(task) === column.id);
          if (column.id === BACKLOG) cards = byEpic(cards, everyTask);
          const droppable = canDrop(column) && !isHome(column);
          // browsers hide title tooltips during a drag, so the note is shown too
          const cantShip = dragged && !dragged.canShip && column.id === SHIPPING ? "No agent in this team ships work" : undefined;
          return (
            <section
              key={column.id}
              aria-label={column.title}
              title={cantShip}
              onDragOver={(event) => onDragOver(event, column)}
              onDragLeave={(event) => {
                if (event.currentTarget.contains(event.relatedTarget as Node)) return;
                setOver((current) => (current === column.id ? "" : current));
                setSlot((current) => (current?.column === column.id ? null : current));
              }}
              onDrop={(event) => onDrop(event, column, cards)}
              className={cn(
                "flex min-w-76 flex-1 flex-col rounded-lg border bg-surface-1 transition-colors duration-(--dur-fast)",
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
              {cantShip && <p className="border-b border-divider px-3 py-1.5 text-[11px] text-ink-muted">{cantShip}</p>}
              <div className="flex min-h-24 scrollbar-thin flex-col gap-2 overflow-y-auto p-2">
                {cards.map((task, index) => (
                  <Fragment key={task.id}>
                    {column.id === BACKLOG && (index === 0 || cards[index - 1].epic !== task.epic) && (
                      <EpicHeading
                        epic={byId.get(task.epic)}
                        count={cards.filter((card) => card.epic === task.epic).length}
                        onPick={onPickEpic}
                      />
                    )}
                    {slot?.column === column.id && slot.at === slotIndex(cards, dragged!.id, index) && <DropLine />}
                    <TaskCard
                      task={task}
                      onPickLabel={onPickLabel}
                      onPickEpic={onPickEpic}
                      epicTitle={byId.get(task.epic)?.title ?? ""}
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
                {column.id === DONE && older > 0 && (
                  <p className="px-1 py-2 text-center text-[11px] text-ink-faint">
                    {older} more, done over {DONE_DAYS} days ago or in a finished epic, are in the list
                  </p>
                )}
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
          onSend={(note) => {
            move(dropped.task, dropped.to, dropped.rank, note);
            setDropped(null);
          }}
        />
      )}
    </>
  );
}

// byEpic groups the Backlog's cards by epic, in the epics' order, keeping
// your order within each; work of no epic comes last.
function byEpic(cards: TaskSummary[], everyTask: TaskSummary[]): TaskSummary[] {
  const rank = new Map(everyTask.filter((task) => task.kind === "epic").map((epic) => [epic.id, epic.rank]));
  const order = (task: TaskSummary) => (task.epic ? (rank.get(task.epic) ?? 0) : Infinity);
  return [...cards].sort((a, b) => order(a) - order(b) || a.epic.localeCompare(b.epic));
}

// EpicHeading starts an epic's stories in the Backlog.
function EpicHeading({ epic, count, onPick }: { epic?: TaskSummary; count: number; onPick: (epic: string) => void }) {
  if (!epic) return <p className="px-1 pt-2 text-[11px] font-semibold tracking-wide text-ink-faint uppercase">No epic · {count}</p>;
  return (
    <button
      type="button"
      onClick={() => onPick(epic.id)}
      title={`Show only ${epic.id}'s stories`}
      className="flex cursor-pointer items-center gap-1.5 px-1 pt-2 text-left text-[11px] font-semibold text-ink-secondary hover:text-ink-primary"
    >
      <span className="truncate">{epic.title}</span>
      <span className="shrink-0 font-mono font-normal text-ink-faint">{count}</span>
    </button>
  );
}

interface DirectDialogProps {
  task: TaskSummary;
  to: BoardColumn;
  onClose: () => void;
  onSend: (note: string) => void;
}

// What a move does, before you make it: you can add a note for the coordinator.
function DirectDialog({ task, to, onClose, onSend }: DirectDialogProps) {
  const [note, setNote] = useState("");
  const reopen = task.status === "done" ? "Reopen and " : "";
  const titles: Record<BoardColumn, string> = {
    backlog: `${reopen}park ${task.id} in the Backlog?`,
    todo: `${reopen}park ${task.id} in To do?`,
    "in-progress": task.parked ? `Start ${task.id}?` : `${reopen}send ${task.id} back for more work?`,
    review: `${reopen}have ${task.id} reviewed next?`,
    shipping: `${reopen}ship ${task.id} now?`,
    done: `Move ${task.id} to Done?`,
  };
  const title = titles[to].charAt(0).toUpperCase() + titles[to].slice(1);
  const send = () => onSend(note);

  return (
    <Dialog title={title} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <p className="text-[12px] text-ink-muted">{whatItDoes(task, to)}</p>
        <TextArea
          autoFocus
          rows={3}
          value={note}
          onChange={(event) => setNote(event.target.value)}
          onKeyDown={(event) => event.key === "Enter" && (event.metaKey || event.ctrlKey) && send()}
          placeholder="A note for the coordinator (optional)"
        />
        <div className="flex gap-2">
          <Button variant="primary" icon={<Send />} onClick={send}>
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

// whatItDoes says what moving the task to a column does, at once.
function whatItDoes(task: TaskSummary, to: BoardColumn): string {
  if (to === "done") return "It is done now. The coordinator gets one last turn to write its summary and tidy up; nothing else runs.";
  if (to === "todo" || to === "backlog") {
    return task.status === "done"
      ? "It reopens, parked: nothing runs on it until you move it on."
      : "It is parked now: a turn in progress stops and the coordinator gets a short wrap-up. Nothing more runs until you move it on.";
  }
  const agent =
    to === "review"
      ? "an agent that checks work, if its team has one"
      : to === "shipping"
        ? "an agent of its team that ships work. Checked or not, it ships as it is, and that agent still runs its usual checks first"
        : "an agent that builds it";
  if (task.status === "done") return `It reopens now, in its worktree kept or made again, and the coordinator must hand it to ${agent}.`;
  const answer = task.hasQuestion ? " The coordinator asked you something, so your note goes as your answer." : "";
  return `The card moves now, and the coordinator must hand it to ${agent}.${answer}`;
}

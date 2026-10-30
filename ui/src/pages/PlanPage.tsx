import { DragEvent, Fragment, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  ArrowUpRight,
  ChevronDown,
  ChevronRight,
  Circle,
  CircleAlert,
  CircleCheck,
  CircleDot,
  Map as MapIcon,
  Play,
  Plus,
} from "lucide-react";
import { useBreadcrumb } from "@/lib/breadcrumb";
import { PageHeader } from "@/components/molecules/PageHeader";
import { Banner } from "@/components/molecules/Banner";
import { EmptyState } from "@/components/molecules/EmptyState";
import { AgentName } from "@/components/molecules/AgentName";
import { StoryProgress } from "@/components/molecules/StoryProgress";
import { useToast } from "@/shell/Toasts";
import { api } from "@/api/client";
import type { TaskSummary } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { fileUnder } from "@/lib/epic-api";
import { waitingLine } from "@/lib/epics";
import { shownEpic } from "@/lib/shown-id";
import { useNewTask } from "@/lib/new-task";
import { rankAt } from "@/lib/rank";
import { DropLine, slotAt, slotIndex } from "@/lib/drag";
import { cn } from "@/lib/cn";

// Plan is where work is shaped before it runs: a column for each epic with
// its stories, and one for the tasks of no epic. Drag a card within its
// column to set its priority, or to another column to file it there, at
// the place you drop it. Finished cards sit at the bottom, out of the order. A story
// that hasn't started has Start; an epic's Start the ready ones starts each
// story that isn't waiting for another. Once started, work is on the Tasks
// board. Finished epics fold away at the end.

const LOOSE = ""; // the column of tasks in no epic

export function PlanPage() {
  useBreadcrumb({ label: "Plan" });
  const navigate = useNavigate();
  const notify = useToast();
  const newTask = useNewTask();
  const { data, error } = useResource<TaskSummary[]>("/api/tasks");
  const tasks = data ?? [];
  const [showFinished, setShowFinished] = useState(false);
  const [dragged, setDragged] = useState<TaskSummary | null>(null);

  const epics = tasks.filter((task) => task.kind === "epic");
  const going = epics.filter((epic) => epic.status !== "done");
  const finished = epics.filter((epic) => epic.status === "done");
  const cardsOf = (epic: string) => {
    const cards = tasks.filter((task) => task.kind !== "epic" && task.epic === epic && (epic !== LOOSE || task.status !== "done"));
    return [...cards.filter((task) => task.status !== "done"), ...cards.filter((task) => task.status === "done")];
  };

  async function start(stories: TaskSummary[]) {
    for (const story of stories) {
      try {
        await api("POST", `/api/tasks/${encodeURIComponent(story.id)}/direct`, { to: "in-progress", note: "" });
      } catch (failure) {
        notify("danger", `${story.id} didn't start`, (failure as Error).message);
        return;
      }
    }
    notify("success", stories.length === 1 ? `Started ${stories[0].id}` : `Started ${stories.length} stories`, "On the Tasks board now.");
  }

  // A card dropped in a column at slot at (counted among its unfinished
  // cards, without the card itself): filed there if it came from another
  // column, and put at that place in your priority order.
  async function drop(column: string, cards: TaskSummary[], at: number) {
    const task = dragged;
    setDragged(null);
    if (!task) return;
    const order = cards.filter((card) => card.status !== "done");
    try {
      if (task.epic !== column) {
        await fileUnder(task.id, column);
        notify("success", column ? `${task.id} is a story of ${shownEpic(column)}` : `${task.id} is out of its epic`);
      }
      const rank = rankAt(order, task, at);
      if (rank !== null) await api("PUT", `/api/tasks/${encodeURIComponent(task.id)}/rank`, { rank });
    } catch (failure) {
      notify("danger", `${task.id} didn't move`, (failure as Error).message);
    }
  }

  const column = (epic: TaskSummary | null) => {
    const id = epic?.id ?? LOOSE;
    const cards = cardsOf(id);
    return (
      <PlanColumn
        key={id || "loose"}
        epic={epic}
        cards={cards}
        dragging={dragged}
        onDragStart={setDragged}
        onDragEnd={() => setDragged(null)}
        onDrop={(at) => drop(id, cards, at)}
        onOpen={(task) => navigate(`/tasks/${task.id}`)}
        onStart={start}
        onAdd={() => (epic ? newTask("story", epic.id) : newTask("task"))}
      />
    );
  };

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title="Plan"
        meta="Shape the work before it runs: each epic with its stories, and the tasks of no epic. Drag a card up or down to set its priority, or to another column to file it there. Started work is on the Tasks board."
        actions={[
          { label: "New task", icon: <Plus />, onClick: () => newTask("task") },
          { label: "New epic", icon: <Plus />, onClick: () => newTask("epic"), primary: true },
        ]}
      />
      {error && (
        <div className="p-6">
          <Banner title="Couldn't load the work">{error}</Banner>
        </div>
      )}
      {data && (
        <div className="flex min-h-0 flex-1 scrollbar-thin gap-3 overflow-x-auto p-4">
          {column(null)}
          {going.map(column)}
          {going.length === 0 && (
            <div className="flex w-72 shrink-0 items-start">
              <EmptyState icon={<MapIcon />} title="No epics yet">
                An epic is a goal too big for one change, split into stories. Make one with New epic, or the coordinator splits a task that
                is too big.
              </EmptyState>
            </div>
          )}
          {finished.length > 0 && (
            <button
              type="button"
              onClick={() => setShowFinished(!showFinished)}
              className="flex w-9 shrink-0 cursor-pointer flex-col items-center gap-2 rounded-lg border border-divider bg-surface-1 py-3 text-[11px] font-semibold tracking-wide text-ink-faint uppercase hover:text-ink-secondary [&>svg]:size-3.5"
              title={showFinished ? "Hide the finished epics" : "Show the finished epics"}
            >
              {showFinished ? <ChevronDown /> : <ChevronRight />}
              <span className="[writing-mode:vertical-rl]">Finished · {finished.length}</span>
            </button>
          )}
          {showFinished && finished.map(column)}
        </div>
      )}
    </div>
  );
}

interface PlanColumnProps {
  epic: TaskSummary | null; // null: the tasks of no epic
  cards: TaskSummary[];
  dragging: TaskSummary | null;
  onDragStart: (task: TaskSummary) => void;
  onDragEnd: () => void;
  onDrop: (at: number) => void; // the slot among its unfinished cards
  onOpen: (task: TaskSummary) => void;
  onStart: (stories: TaskSummary[]) => void;
  onAdd: () => void;
}

function PlanColumn({ epic, cards, dragging, onDragStart, onDragEnd, onDrop, onOpen, onStart, onAdd }: PlanColumnProps) {
  const list = useRef<HTMLDivElement>(null);
  const [slot, setSlot] = useState<number | null>(null); // where the card being dragged would land
  const done = epic?.status === "done";
  const order = cards.filter((card) => card.status !== "done"); // your priority order
  const finished = cards.filter((card) => card.status === "done");
  const ready = order.filter((card) => card.parked && card.blockedBy.length === 0);
  const canDrop = dragging !== null && !done;
  const dragOver = (event: DragEvent) => {
    if (!canDrop || !list.current) return;
    event.preventDefault();
    setSlot(slotAt(list.current, event.clientY, dragging.id));
  };
  const last = order.filter((card) => card.id !== dragging?.id).length;

  return (
    <section
      aria-label={epic ? epic.title : "No epic"}
      onDragOver={dragOver}
      onDragLeave={(event) => !event.currentTarget.contains(event.relatedTarget as Node) && setSlot(null)}
      onDrop={(event) => {
        event.preventDefault();
        if (canDrop && slot !== null) onDrop(Math.min(slot, last));
        setSlot(null);
      }}
      className={cn(
        "flex w-72 shrink-0 flex-col rounded-lg border bg-surface-1",
        slot !== null && dragging?.epic !== (epic?.id ?? LOOSE) ? "border-dashed border-brand" : "border-divider",
        done && "opacity-70",
      )}
    >
      <header className="flex shrink-0 flex-col gap-1.5 border-b border-divider px-3 py-2.5">
        <span className="flex items-start gap-2">
          <span className="line-clamp-2 flex-1 text-[13px] leading-[18px] font-semibold text-ink-primary">
            {epic ? epic.title : "No epic"}
          </span>
          {epic && (
            <button
              type="button"
              aria-label={`Open ${shownEpic(epic.id)}`}
              title={`Open ${shownEpic(epic.id)}`}
              onClick={() => onOpen(epic)}
              className="flex size-5 shrink-0 cursor-pointer items-center justify-center text-ink-faint hover:text-ink-primary [&>svg]:size-3.5"
            >
              <ArrowUpRight />
            </button>
          )}
        </span>
        {epic ? (
          <>
            <StoryProgress done={epic.storiesDone} total={epic.stories} />
            <span className="flex items-center gap-2 font-mono text-[11px] text-ink-faint">
              {shownEpic(epic.id)}
              {epic.status === "needs you" ? (
                <button type="button" onClick={() => onOpen(epic)} className="ml-auto cursor-pointer text-warning hover:underline">
                  needs you
                </button>
              ) : (
                ready.length > 0 && (
                  <button
                    type="button"
                    onClick={() => onStart(ready)}
                    className="ml-auto flex cursor-pointer items-center gap-1 text-ink-secondary hover:text-ink-primary [&>svg]:size-3"
                  >
                    <Play />
                    Start the ready ones · {ready.length}
                  </button>
                )
              )}
            </span>
          </>
        ) : (
          <span className="text-[11px] text-ink-faint">Unfinished tasks of no epic</span>
        )}
      </header>
      <div ref={list} className="flex min-h-16 scrollbar-thin flex-col gap-2 overflow-y-auto p-2">
        {order.map((card, index) => (
          <Fragment key={card.id}>
            {slot !== null && Math.min(slot, last) === slotIndex(order, dragging!.id, index) && <DropLine />}
            <PlanCard
              task={card}
              dragging={dragging?.id === card.id}
              onDragStart={() => onDragStart(card)}
              onDragEnd={() => (setSlot(null), onDragEnd())}
              onOpen={() => onOpen(card)}
              onStart={() => onStart([card])}
            />
          </Fragment>
        ))}
        {slot !== null && Math.min(slot, last) === last && <DropLine />}
        {finished.map((card) => (
          <PlanCard
            key={card.id}
            task={card}
            dragging={dragging?.id === card.id}
            onDragStart={() => onDragStart(card)}
            onDragEnd={() => (setSlot(null), onDragEnd())}
            onOpen={() => onOpen(card)}
            onStart={() => onStart([card])}
          />
        ))}
        {cards.length === 0 && <p className="px-1 py-3 text-center text-[11px] text-ink-faint">Nothing here</p>}
      </div>
      {!done && (
        <button
          type="button"
          onClick={onAdd}
          className="flex h-9 shrink-0 cursor-pointer items-center gap-1.5 border-t border-divider px-3 text-left text-[12px] text-ink-muted hover:text-ink-primary [&>svg]:size-3.5"
        >
          <Plus />
          {epic ? "Story" : "Task"}
        </button>
      )}
    </section>
  );
}

interface PlanCardProps {
  task: TaskSummary;
  dragging: boolean;
  onDragStart: () => void;
  onDragEnd: () => void;
  onOpen: () => void;
  onStart: () => void;
}

// One story or task in Plan: whether it is done, running or not started,
// what it waits for, and Start while it waits for you.
function PlanCard({ task, dragging, onDragStart, onDragEnd, onOpen, onStart }: PlanCardProps) {
  const notStarted = task.status === "held";
  const line =
    task.status === "needs you"
      ? task.needsYou
      : task.status === "done"
        ? "Done"
        : waitingLine(task) || (notStarted ? "Not started" : task.currentAgent ? `${task.currentAgent} is working` : "Under way");
  return (
    <div
      role="button"
      tabIndex={0}
      data-task={task.status === "done" ? undefined : task.id} // a place in the order (lib/drag)
      draggable
      onDragStart={(event) => {
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("text/plain", task.id);
        onDragStart();
      }}
      onDragEnd={onDragEnd}
      onClick={onOpen}
      onKeyDown={(event) => event.key === "Enter" && onOpen()}
      className={cn(
        "flex cursor-pointer flex-col gap-1 rounded-md border border-divider bg-surface-0 px-3 py-2 hover:border-divider-2",
        dragging && "opacity-40",
      )}
    >
      <span className="flex items-start gap-2">
        <StatusGlyph task={task} />
        <span
          className={cn(
            "line-clamp-2 flex-1 text-[13px] leading-[18px] font-medium",
            task.status === "done" ? "text-ink-muted" : "text-ink-primary",
          )}
        >
          {task.title}
        </span>
      </span>
      <span className={cn("line-clamp-2 pl-6 text-[12px] leading-4", task.status === "needs you" ? "text-warning" : "text-ink-muted")}>
        {line}
      </span>
      <span className="flex items-center gap-2 pl-6 font-mono text-[11px] text-ink-faint">
        {task.id}
        {task.currentAgent && <AgentName name={task.currentAgent} dotOnly />}
        {notStarted && (
          <button
            type="button"
            onClick={(event) => (event.stopPropagation(), onStart())}
            className="ml-auto flex cursor-pointer items-center gap-1 font-sans text-[12px] text-ink-secondary hover:text-ink-primary [&>svg]:size-3"
          >
            <Play />
            Start
          </button>
        )}
      </span>
    </div>
  );
}

function StatusGlyph({ task }: { task: TaskSummary }) {
  const className = "mt-0.5 size-4 shrink-0";
  if (task.status === "done") return <CircleCheck className={cn(className, "text-success")} aria-label="done" />;
  if (task.status === "needs you") return <CircleAlert className={cn(className, "text-warning")} aria-label="needs you" />;
  if (task.status === "held") return <Circle className={cn(className, "text-ink-faint")} aria-label="not started" />;
  return <CircleDot className={cn(className, "text-info")} aria-label="under way" />;
}

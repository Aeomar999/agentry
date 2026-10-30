import { ReactNode, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { ChevronDown, ChevronRight, Columns3, Inbox, List, Plus } from "lucide-react";
import { useBreadcrumb } from "@/lib/breadcrumb";
import { PageHeader } from "@/components/molecules/PageHeader";
import { FilterChip } from "@/components/molecules/FilterChip";
import { SearchField } from "@/components/molecules/SearchField";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { QueuedBadge, TaskStatusBadge } from "@/components/molecules/StatusBadges";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Banner } from "@/components/molecules/Banner";
import { Mono } from "@/components/molecules/Mono";
import { AgentName } from "@/components/molecules/AgentName";
import { LabelList } from "@/components/molecules/LabelChip";
import type { Agent, BoardColumn, TaskStatus, TaskSummary } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { useKeys, useRowCursor } from "@/lib/shortcuts";
import { readStored, writeStored } from "@/lib/stored";
import { tokens } from "@/lib/tokens";
import { rankAt } from "@/domains/tasks/rank";
import { api } from "@/api/client";
import { useToast } from "@/shell/Toasts";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { TaskBoard } from "@/domains/tasks/TaskBoard";
import { RunningLimit } from "@/domains/tasks/RunningLimit";
import { useNewTask } from "@/lib/new-task";
import { timeAgo } from "@/lib/time";
import { cn } from "@/lib/cn";
import { grouped, waitingLine } from "@/lib/epics";
import { shownEpic, shownId } from "@/lib/shown-id";
import { stageOf } from "@/lib/columns";
import { EpicBadge, StoryProgress } from "@/components/molecules/StoryProgress";
import { useExtensions, withTheirCards } from "@/extensions";

// The filter lives in the address (?status=needs-you), so a link can open
// the list already filtered: the bell in the top bar does. Review and
// Shipping are the board's columns of work being checked or shipped, each
// shown while the board has it.
interface Filter {
  id: string;
  label: string;
  status?: TaskStatus;
  column?: BoardColumn;
}

const FILTERS: Filter[] = [
  { id: "all", label: "All" },
  { id: "needs-you", label: "Needs you", status: "needs you" },
  { id: "working", label: "Working", status: "working" },
  { id: "review", label: "Review", column: "review" },
  { id: "shipping", label: "Shipping", column: "shipping" },
  { id: "held", label: "Held", status: "held" },
  { id: "done", label: "Done", status: "done" },
];

const fits = (filter: Filter, task: TaskSummary) =>
  (!filter.status || task.status === filter.status) && (!filter.column || task.column === filter.column);

// Epics show their stories under them, folded or not by a click on the
// chevron; a finished epic starts folded.
interface Folding {
  depth: (task: TaskSummary) => number;
  folded: (epic: string) => boolean;
  toggle: (epic: string) => void;
}

function columnsFor(pickLabel: (label: string) => void, folding: Folding): Column<TaskSummary>[] {
  return [
    {
      header: "Task",
      className: "w-28",
      render: (task) => (
        <span className={cn("flex items-center gap-1", folding.depth(task) && "pl-4")}>
          {task.kind === "epic" ? (
            <button
              type="button"
              aria-label={folding.folded(task.id) ? "Show its stories" : "Hide its stories"}
              onClick={(event) => (event.stopPropagation(), folding.toggle(task.id))}
              className="-ml-1 flex size-4 cursor-pointer items-center justify-center text-ink-faint hover:text-ink-primary [&>svg]:size-3.5"
            >
              {folding.folded(task.id) ? <ChevronRight /> : <ChevronDown />}
            </button>
          ) : (
            <span className="w-3" />
          )}
          <Mono className="text-ink-muted">{shownId(task.id, task.kind)}</Mono>
        </span>
      ),
    },
    {
      header: "Title",
      className: "w-full max-w-0",
      render: (task) => (
        <div className="min-w-0 py-0.5">
          <p className="flex items-center gap-2">
            {task.kind === "epic" && <EpicBadge />}
            <span className="truncate font-medium text-ink-primary">{task.title}</span>
            {task.queued && <QueuedBadge />}
            {task.kind === "epic" && <StoryProgress done={task.storiesDone} total={task.stories} className="shrink-0" />}
            <LabelList labels={task.labels} onPick={pickLabel} />
          </p>
          <p className="truncate text-[12px] text-ink-muted">{describe(task)}</p>
        </div>
      ),
    },
    { header: "Status", className: "w-28", render: (task) => <TaskStatusBadge status={task.status} /> },
    {
      header: "Agent",
      className: "w-32 max-lg:hidden",
      render: (task) => (task.currentAgent ? <AgentName name={task.currentAgent} /> : <Mono className="text-ink-faint">none</Mono>),
    },
    { header: "Turns", className: "w-16 text-right max-lg:hidden", render: (task) => <Mono>{task.turns}</Mono> },
    {
      header: "Tokens",
      className: "w-24 text-right max-lg:hidden",
      render: (task) => <Mono className={task.overBudget ? "text-danger" : undefined}>{tokens(task.tokensUsed)}</Mono>,
    },
    {
      header: "Active",
      className: "w-24 text-right max-sm:hidden",
      render: (task) => (
        <Mono className="text-ink-muted">
          <span title={task.lastActiveAt}>{timeAgo(task.lastActiveAt)}</span>
        </Mono>
      ),
    },
  ];
}

// describe is the line under a task's title: its stage first, when it is
// past building, then what it is waiting for or who is working on it.
function describe(task: TaskSummary): ReactNode {
  if (task.status === "done") return "Done";
  const stage = stageOf(task) ? `${stageOf(task)} · ` : "";
  if (task.status === "needs you")
    return (
      <>
        {stage}
        <span className="text-warning">{task.needsYou}</span>
      </>
    );
  return stage + (waitingLine(task) || (task.currentAgent ? `${task.currentAgent} is working` : "The coordinator is next"));
}

type View = "list" | "board";
const viewKey = "agentry-tasks-view";
const backlogKey = "agentry-show-backlog";
const foldKey = "agentry-folded-epics";

function readFolds(): Record<string, boolean> {
  try {
    return JSON.parse(readStored(foldKey) ?? "{}");
  } catch {
    return {};
  }
}

export function TasksPage() {
  useBreadcrumb({ label: "Tasks" });
  const navigate = useNavigate();
  const notify = useToast();
  const newTask = useNewTask();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState("");
  const { data, error } = useResource<TaskSummary[]>("/api/tasks");
  const agents = useResource<Agent[]>("/api/agents").data ?? [];
  const tasks = data ?? [];

  // The table by default, or the board; ?view= picks one, and the last one
  // picked is remembered in this browser. Both keep the filter.
  const asked = params.get("view");
  const view: View = asked === "board" || asked === "list" ? asked : readStored(viewKey) === "board" ? "board" : "list";
  const showView = (next: View) => {
    writeStored(viewKey, next);
    setParam("view", next);
  };
  const setParam = (name: string, value: string | null) =>
    setParams((current) => {
      const next = new URLSearchParams(current);
      if (value === null) next.delete(name);
      else next.set(name, value);
      return next;
    });
  useKeys({ "1": () => showView("list"), "2": () => showView("board") });

  const columns = withTheirCards(useExtensions().columns, tasks);
  const filters = FILTERS.filter((f) => !f.column || columns.includes(f.column));
  const filter = filters.find((f) => f.id === params.get("status")) ?? filters[0];
  // The Backlog shows when you ask; remembered here.
  const [showBacklog, setShowBacklog] = useState(() => readStored(backlogKey) === "true"); // Plan is where unstarted work is shaped
  const toggleBacklog = () => {
    writeStored(backlogKey, String(!showBacklog));
    setShowBacklog(!showBacklog);
  };
  const inBacklog = tasks.filter((task) => task.parked === "backlog").length;
  const label = params.get("label");
  const pickLabel = (picked: string) => setParam("label", picked);
  const epic = params.get("epic");
  const pickEpic = (picked: string) => setParam("epic", picked);
  const epicTitle = tasks.find((task) => task.id === epic)?.title;
  const [folds, setFolds] = useState(readFolds);
  const shown = useMemo(() => {
    const words = search.toLowerCase().split(/\s+/).filter(Boolean);
    return tasks.filter(
      (task) =>
        fits(filter, task) &&
        (showBacklog || task.parked !== "backlog") &&
        (!label || task.labels.includes(label)) &&
        (!epic || task.id === epic || task.epic === epic) &&
        words.every((word) =>
          `${task.id} ${shownId(task.id, task.kind)} ${task.title} ${task.repo} ${task.currentAgent} ${task.labels.join(" ")}`
            .toLowerCase()
            .includes(word),
        ),
    );
  }, [tasks, filter, label, epic, search, showBacklog]);

  const byId = new Map(tasks.map((task) => [task.id, task]));
  const shownIds = new Set(shown.map((task) => task.id));
  const folding: Folding = {
    depth: (task) => (task.epic && shownIds.has(task.epic) ? 1 : 0),
    folded: (id) => !epic && (folds[id] ?? byId.get(id)?.status === "done"),
    toggle: (id) => {
      const next = { ...folds, [id]: !folding.folded(id) };
      writeStored(foldKey, JSON.stringify(next));
      setFolds(next);
    },
  };
  const rows = grouped(shown, folding.folded).map((row) => row.task);

  const open = (task: TaskSummary) => navigate(`/tasks/${task.id}`);
  // Dragging a row sets its place in your priority order, as on the board.
  const reorder = async (task: TaskSummary, at: number) => {
    const rank = rankAt(rows, task, at);
    if (rank === null) return;
    try {
      await api("PUT", `/api/tasks/${encodeURIComponent(task.id)}/rank`, { rank });
    } catch (failure) {
      notify("danger", "The order wasn't saved", (failure as Error).message);
    }
  };
  const cursor = useRowCursor(view === "list" ? rows.length : 0, (index) => open(rows[index]));

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title="Tasks"
        badges={<RunningLimit queued={tasks.filter((task) => task.queued).length} />}
        meta="The work under way: each task goes to its coordinator, which hands it from agent to agent until it is done. Shape epics and their stories in Plan. Drag tasks to put them in your order."
        actions={[{ label: "New task", icon: <Plus />, onClick: () => newTask(), primary: true }]}
      />
      <div className="flex flex-wrap items-center gap-2 border-b border-divider px-6 py-3">
        {filters.map((f) => (
          <FilterChip
            key={f.id}
            label={f.label}
            count={tasks.filter((task) => fits(f, task)).length}
            active={f === filter}
            onClick={() => setParam("status", f.id === "all" ? null : f.id)}
          />
        ))}
        {label && <FilterChip label={`label: ${label}  ×`} active onClick={() => setParam("label", null)} />}
        {epic && <FilterChip label={`epic: ${epicTitle ?? shownEpic(epic)}  ×`} active onClick={() => setParam("epic", null)} />}
        <div className="ml-auto flex items-center gap-2" data-page-search>
          <FilterChip
            label={showBacklog ? "Hide the backlog" : "Show the backlog"}
            count={inBacklog}
            active={showBacklog}
            onClick={toggleBacklog}
          />
          <SegmentedControl
            segments={[
              { id: "list", label: "List", icon: <List /> },
              { id: "board", label: "Board", icon: <Columns3 /> },
            ]}
            active={view}
            onChange={(id) => showView(id as View)}
          />
          <SearchField value={search} onChange={setSearch} placeholder="Filter tasks" shortcut="/" />
        </div>
      </div>
      {error && (
        <div className="p-6">
          <Banner title="Couldn't load the tasks">{error}</Banner>
        </div>
      )}
      {!data && !error && <p className="px-6 py-4 text-[12px] text-ink-muted">Loading the tasks…</p>}
      {view === "board" && data && (
        <TaskBoard
          columns={columns}
          tasks={shown}
          everyTask={tasks}
          agents={agents}
          showBacklog={showBacklog}
          onOpen={open}
          onPickLabel={pickLabel}
          onPickEpic={pickEpic}
        />
      )}
      {view === "list" && data && (
        <ResourceTable
          columns={columnsFor(pickLabel, folding)}
          rows={rows}
          rowKey={(task) => task.id}
          onOpen={open}
          activeKey={rows[cursor]?.id}
          onReorder={reorder}
          empty={
            <EmptyState icon={<Inbox />} title={tasks.length ? "No task matches" : "No tasks yet"}>
              {tasks.length ? "Try another filter." : "Add one and the coordinator picks it up. Press N."}
            </EmptyState>
          }
        />
      )}
    </div>
  );
}

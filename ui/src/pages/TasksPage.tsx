import { useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Columns3, Inbox, List, Plus } from "lucide-react";
import { useBreadcrumb } from "@/components/templates/AdminShell";
import { PageHeader } from "@/components/molecules/PageHeader";
import { FilterChip } from "@/components/molecules/FilterChip";
import { SearchField } from "@/components/molecules/SearchField";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { TaskStatusBadge } from "@/components/molecules/StatusBadges";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Banner } from "@/components/molecules/Banner";
import { Mono } from "@/components/molecules/Mono";
import { LabelList } from "@/components/molecules/LabelChip";
import type { Agent, TaskStatus, TaskSummary } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { useKeys, useRowCursor } from "@/lib/shortcuts";
import { readStored, writeStored } from "@/lib/stored";
import { tokens } from "@/lib/tokens";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { TaskBoard } from "@/components/organisms/TaskBoard";
import { timeAgo } from "@/lib/time";

// The filter lives in the address (?status=needs-you), so a link can open
// the list already filtered: the bell in the top bar does.
const FILTERS: { id: string; label: string; status?: TaskStatus }[] = [
  { id: "all", label: "All" },
  { id: "needs-you", label: "Needs you", status: "needs you" },
  { id: "working", label: "Working", status: "working" },
  { id: "done", label: "Done", status: "done" },
];

function columnsFor(pickLabel: (label: string) => void): Column<TaskSummary>[] {
  return [
    { header: "Task", className: "w-24", render: (task) => <Mono className="text-ink-muted">{task.id}</Mono> },
    {
      header: "Title",
      className: "w-full max-w-0",
      render: (task) => (
        <div className="min-w-0 py-0.5">
          <p className="flex items-center gap-2">
            <span className="truncate font-medium text-ink-primary">{task.title}</span>
            <LabelList labels={task.labels} onPick={pickLabel} />
          </p>
          <p className="truncate text-[12px] text-ink-muted">
            {task.status === "needs you" ? <span className="text-warning">{task.needsYou}</span> : describe(task)}
          </p>
        </div>
      ),
    },
    { header: "Status", className: "w-28", render: (task) => <TaskStatusBadge status={task.status} /> },
    { header: "Agent", className: "w-32 max-lg:hidden", render: (task) => <Mono>{task.currentAgent || "none"}</Mono> },
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

function describe(task: TaskSummary): string {
  if (task.status === "done") return "Done";
  return task.currentAgent ? `${task.currentAgent} is working` : "The coordinator is next";
}

type View = "list" | "board";
const viewKey = "agentry-tasks-view";

export function TasksPage() {
  useBreadcrumb({ label: "Tasks" });
  const navigate = useNavigate();
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

  const filter = FILTERS.find((f) => f.id === params.get("status")) ?? FILTERS[0];
  const label = params.get("label");
  const pickLabel = (picked: string) => setParam("label", picked);
  const shown = useMemo(() => {
    const words = search.toLowerCase().split(/\s+/).filter(Boolean);
    return tasks.filter(
      (task) =>
        (!filter.status || task.status === filter.status) &&
        (!label || task.labels.includes(label)) &&
        words.every((word) =>
          `${task.id} ${task.title} ${task.repo} ${task.currentAgent} ${task.labels.join(" ")}`.toLowerCase().includes(word),
        ),
    );
  }, [tasks, filter, label, search]);

  const open = (task: TaskSummary) => navigate(`/tasks/${task.id}`);
  const cursor = useRowCursor(view === "list" ? shown.length : 0, (index) => open(shown[index]));

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title="Tasks"
        meta="Each task goes to the coordinator, which hands it from agent to agent until it says done."
        actions={[{ label: "New task", icon: <Plus />, to: "/tasks/new", primary: true }]}
      />
      <div className="flex flex-wrap items-center gap-2 border-b border-divider px-6 py-3">
        {FILTERS.map((f) => (
          <FilterChip
            key={f.id}
            label={f.label}
            count={f.status ? tasks.filter((task) => task.status === f.status).length : tasks.length}
            active={f === filter}
            onClick={() => setParam("status", f.id === "all" ? null : f.id)}
          />
        ))}
        {label && <FilterChip label={`label: ${label}  ×`} active onClick={() => setParam("label", null)} />}
        <div className="ml-auto flex items-center gap-2" data-page-search>
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
      {view === "board" && data && <TaskBoard tasks={shown} agents={agents} onOpen={open} onPickLabel={pickLabel} />}
      {view === "list" && (
        <ResourceTable
          columns={columnsFor(pickLabel)}
          rows={shown}
          rowKey={(task) => task.id}
          onOpen={open}
          activeKey={shown[cursor]?.id}
          empty={
            data && (
              <EmptyState icon={<Inbox />} title={tasks.length ? "No task matches" : "No tasks yet"}>
                {tasks.length ? "Try another filter." : "Add one and the coordinator picks it up. Press N."}
              </EmptyState>
            )
          }
        />
      )}
    </div>
  );
}

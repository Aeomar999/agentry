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
import type { Agent, TaskStatus, TaskSummary } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { useKeys, useRowCursor } from "@/lib/shortcuts";
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

const COLUMNS: Column<TaskSummary>[] = [
  { header: "Task", className: "w-24", render: (task) => <Mono className="text-ink-muted">{task.id}</Mono> },
  {
    header: "Title",
    className: "w-full max-w-0",
    render: (task) => (
      <div className="min-w-0 py-0.5">
        <p className="truncate font-medium text-ink-primary">{task.title}</p>
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
    header: "Active",
    className: "w-24 text-right max-sm:hidden",
    render: (task) => (
      <Mono className="text-ink-muted">
        <span title={task.lastActiveAt}>{timeAgo(task.lastActiveAt)}</span>
      </Mono>
    ),
  },
];

function describe(task: TaskSummary): string {
  if (task.status === "done") return "Done";
  return task.currentAgent ? `${task.currentAgent} is working` : "The coordinator is next";
}

export function TasksPage() {
  useBreadcrumb({ label: "Tasks" });
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState("");
  const { data, error } = useResource<TaskSummary[]>("/api/tasks");
  const agents = useResource<Agent[]>("/api/agents").data ?? [];
  const tasks = data ?? [];

  // The board is the default; ?view=list shows the table. Both keep the filter.
  const view = params.get("view") === "list" ? "list" : "board";
  const setParam = (name: string, value: string | null) =>
    setParams((current) => {
      const next = new URLSearchParams(current);
      if (value === null) next.delete(name);
      else next.set(name, value);
      return next;
    });
  useKeys({ "1": () => setParam("view", null), "2": () => setParam("view", "list") });

  const filter = FILTERS.find((f) => f.id === params.get("status")) ?? FILTERS[0];
  const shown = useMemo(() => {
    const words = search.toLowerCase().split(/\s+/).filter(Boolean);
    return tasks.filter(
      (task) =>
        (!filter.status || task.status === filter.status) &&
        words.every((word) => `${task.id} ${task.title} ${task.repo} ${task.currentAgent}`.toLowerCase().includes(word)),
    );
  }, [tasks, filter, search]);

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
        <div className="ml-auto flex items-center gap-2" data-page-search>
          <SegmentedControl
            segments={[
              { id: "board", label: "Board", icon: <Columns3 /> },
              { id: "list", label: "List", icon: <List /> },
            ]}
            active={view}
            onChange={(id) => setParam("view", id === "list" ? "list" : null)}
          />
          <SearchField value={search} onChange={setSearch} placeholder="Filter tasks" shortcut="/" />
        </div>
      </div>
      {error && (
        <div className="p-6">
          <Banner title="Couldn't load the tasks">{error}</Banner>
        </div>
      )}
      {view === "board" && data && <TaskBoard tasks={shown} agents={agents} onOpen={open} />}
      {view === "list" && (
        <ResourceTable
          columns={COLUMNS}
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

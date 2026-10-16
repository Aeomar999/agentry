import { useParams, useSearchParams } from "react-router-dom";
import { CircleCheck, FileQuestion, SquareTerminal } from "lucide-react";
import { useBreadcrumb, useTerminal } from "@/components/templates/AdminShell";
import { PageHeader } from "@/components/molecules/PageHeader";
import { Panel } from "@/components/molecules/Panel";
import { Tabs } from "@/components/molecules/Tabs";
import { HandoffTimeline } from "@/components/molecules/HandoffTimeline";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { ActionBadge, TaskStatusBadge } from "@/components/molecules/StatusBadges";
import { KeyValueList } from "@/components/molecules/KeyValueList";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Banner } from "@/components/molecules/Banner";
import { Mono } from "@/components/molecules/Mono";
import { NeedsYouPanel } from "@/components/organisms/NeedsYouPanel";
import { LabelEditor } from "@/components/organisms/LabelEditor";
import type { Action, LogEvent, TaskDetail, Turn } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { clockTime, dateTime, timeAgo } from "@/lib/time";

const ACTION_COLUMNS: Column<Action>[] = [
  { header: "Time", className: "w-20", render: (a) => <Mono className="text-ink-muted">{clockTime(a.requestedAt)}</Mono> },
  { header: "Agent", className: "w-28", render: (a) => <Mono>{a.agent}</Mono> },
  { header: "Tool", className: "w-24", render: (a) => <Mono className="text-ink-muted">{a.tool}</Mono> },
  {
    header: "Command",
    className: "w-full max-w-0",
    render: (a) => (
      <p className="line-clamp-3 font-mono text-[12px] break-all whitespace-pre-wrap text-ink-primary" title={a.command}>
        {a.command}
      </p>
    ),
  },
  {
    header: "Decision",
    className: "w-56",
    render: (a) => (
      <div className="flex flex-col items-start gap-0.5 py-0.5">
        <ActionBadge action={a} />
        {a.reason && <span className="line-clamp-2 text-[11px] text-ink-muted">{a.reason}</span>}
      </div>
    ),
  },
];

const EVENT_COLUMNS: Column<LogEvent>[] = [
  { header: "#", className: "w-14 text-right", render: (e) => <Mono className="text-ink-muted">{e.position}</Mono> },
  { header: "Time", className: "w-20", render: (e) => <Mono className="text-ink-muted">{clockTime(e.at)}</Mono> },
  { header: "Type", className: "w-44", render: (e) => <Mono className="text-info">{e.type}</Mono> },
  { header: "Actor", className: "w-36", render: (e) => <Mono>{e.actor}</Mono> },
  {
    header: "Data",
    className: "w-full max-w-0",
    render: (e) => {
      const data = JSON.stringify(e.data ?? {});
      return (
        <p className="line-clamp-2 font-mono text-[11px] break-all text-ink-muted" title={data}>
          {data}
        </p>
      );
    },
  },
];

const TABS = ["activity", "actions", "events"] as const;
type TabId = (typeof TABS)[number];

export function TaskPage() {
  const { taskId = "" } = useParams();
  useBreadcrumb({ label: "Tasks", to: "/tasks" }, { label: taskId });
  const [params, setParams] = useSearchParams();
  const { data: task, error, reload } = useResource<TaskDetail>(`/api/tasks/${encodeURIComponent(taskId)}`);
  const openTerminal = useTerminal();

  if (error && !task) {
    return (
      <EmptyState icon={<FileQuestion />} title={`No task ${taskId}`}>
        {error}
      </EmptyState>
    );
  }
  if (!task) return null;

  const working = task.status !== "done" && Boolean(task.currentAgent);
  const asked = params.get("tab") as TabId | null;
  const tab: TabId = asked && TABS.includes(asked) ? asked : "activity";
  const show = (id: string) => setParams({ tab: id }, { replace: true });
  const w = task.worktree;

  return (
    <>
      <PageHeader
        title={task.title}
        badges={<TaskStatusBadge status={task.status} />}
        actions={
          working ? [{ label: `${task.currentAgent}'s terminal`, icon: <SquareTerminal />, onClick: () => openTerminal(task.id) }] : []
        }
        meta={
          <>
            <Mono>{task.id}</Mono>
            <span>·</span>
            <Mono>{task.repo}</Mono>
            {w ? (
              <>
                <span>·</span>
                <Mono>{w.branch}</Mono>
              </>
            ) : (
              task.status !== "done" && <span>· setting up the worktree</span>
            )}
            {task.firstTitle && <span title={task.firstTitle}>· renamed by the coordinator from "{task.firstTitle.slice(0, 60)}"</span>}
          </>
        }
      />
      {error && (
        <div className="px-6 pt-4">
          <Banner tone="warning" title="Showing the last copy: couldn't refresh">
            {error}
          </Banner>
        </div>
      )}
      <div className="flex flex-col gap-6 p-6 xl:flex-row xl:items-start">
        <div className="flex min-w-0 flex-1 flex-col gap-4">
          <LabelEditor key={task.labels.join(",")} taskId={task.id} labels={task.labels} />
          {task.description && <p className="text-[13px] leading-5 whitespace-pre-wrap text-ink-secondary">{task.description}</p>}
          <NeedsYouPanel task={task} onChanged={reload} onOpenTerminal={() => openTerminal(task.id)} />
          {task.status === "done" && (
            <Panel
              title={
                <span className="flex items-center gap-1.5 text-success">
                  <CircleCheck className="size-4" />
                  Done
                </span>
              }
            >
              <p className="text-[13px] whitespace-pre-wrap text-ink-secondary">{task.summary}</p>
            </Panel>
          )}
          <div className="-mx-6">
            <Tabs
              active={tab}
              onChange={show}
              tabs={[
                { id: "activity", label: "Handoffs", count: task.trail?.length ?? 0 },
                { id: "actions", label: "Actions", count: task.actions?.length ?? 0 },
                { id: "events", label: "Events", count: task.events?.length ?? 0 },
              ]}
            />
          </div>
          {tab === "activity" &&
            (task.trail?.length ? (
              <HandoffTimeline steps={task.trail} />
            ) : (
              <EmptyState title="No handoffs yet">The coordinator reads the task first.</EmptyState>
            ))}
          {tab === "actions" && (
            <Panel flush>
              <ResourceTable
                columns={ACTION_COLUMNS}
                rows={task.actions ?? []}
                rowKey={(a) => a.id}
                empty={
                  <EmptyState title="No actions yet">Every tool call an agent makes shows here with the policy's decision.</EmptyState>
                }
              />
            </Panel>
          )}
          {tab === "events" && (
            <Panel flush>
              <ResourceTable columns={EVENT_COLUMNS} rows={task.events ?? []} rowKey={(e) => String(e.position)} />
            </Panel>
          )}
        </div>

        <aside className="flex w-full shrink-0 flex-col gap-4 xl:sticky xl:top-6 xl:w-inspector">
          <Panel title="Details">
            <KeyValueList
              items={[
                { label: "Status", value: <TaskStatusBadge status={task.status} /> },
                { label: "Agent", value: <Mono>{task.currentAgent || "none"}</Mono> },
                { label: "Coordinator", value: <Mono>{task.coordinator}</Mono> },
                { label: "Turns", value: <Mono>{task.turns}</Mono> },
                { label: "Worktree", value: w ? <Mono className="break-all">{w.path}</Mono> : "not yet" },
                ...(w
                  ? [
                      { label: "Branch", value: <Mono className="break-all">{w.branch}</Mono> },
                      { label: "Base", value: <Mono>{w.base.slice(0, 10)}</Mono> },
                      { label: "Port", value: <Mono>{w.port}</Mono> },
                    ]
                  : []),
                { label: "Created", value: dateTime(task.createdAt) },
                { label: "Last active", value: timeAgo(task.lastActiveAt) },
              ]}
            />
          </Panel>
          <Panel title="Turns" aside={<Mono>{task.turnList?.length ?? 0}</Mono>} flush>
            <TurnList turns={task.turnList ?? []} />
          </Panel>
        </aside>
      </div>
    </>
  );
}

function TurnList({ turns }: { turns: Turn[] }) {
  if (turns.length === 0) return <p className="px-4 py-3 text-[12px] text-ink-muted">No turns yet.</p>;
  return (
    <ol>
      {[...turns].reverse().map((turn) => (
        <li key={turn.id} className="flex flex-col gap-0.5 border-b border-divider px-4 py-2 last:border-b-0">
          <div className="flex items-center gap-2 text-[12px]">
            <Mono className="font-semibold text-ink-primary">{turn.agent}</Mono>
            <Mono className="text-ink-faint">{clockTime(turn.startedAt)}</Mono>
            <span className="ml-auto text-[11px] text-ink-muted">{turn.ended ? (turn.issue ? "issue" : "ended") : "running"}</span>
          </div>
          {turn.issue && <p className="text-[11px] break-words text-danger">{turn.issue}</p>}
        </li>
      ))}
    </ol>
  );
}

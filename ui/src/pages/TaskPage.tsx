import { ReactNode } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { CircleCheck, FileQuestion, FolderGit2, SquareTerminal } from "lucide-react";
import { useBreadcrumb } from "@/components/templates/AdminShell";
import { Badge } from "@/components/atoms/Badge";
import { PageHeader } from "@/components/molecules/PageHeader";
import { Panel } from "@/components/molecules/Panel";
import { Clamp } from "@/components/molecules/Clamp";
import { Tabs } from "@/components/molecules/Tabs";
import { HandoffTimeline } from "@/components/molecules/HandoffTimeline";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { ActionBadge, TaskStatusBadge } from "@/components/molecules/StatusBadges";
import { KeyValueList } from "@/components/molecules/KeyValueList";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Banner } from "@/components/molecules/Banner";
import { Mono } from "@/components/molecules/Mono";
import { AgentName } from "@/components/molecules/AgentName";
import { TokenMeter } from "@/components/molecules/TokenMeter";
import { ActivityFeed, Now } from "@/components/organisms/ActivityFeed";
import { NeedsYouPanel } from "@/components/organisms/NeedsYouPanel";
import { LabelEditor } from "@/components/organisms/LabelEditor";
import { TerminalView } from "@/components/organisms/TerminalView";
import { PlanRail } from "@/components/organisms/PlanRail";
import { CoordinatorChat } from "@/components/organisms/CoordinatorChat";
import { HandoffPath } from "@/components/molecules/HandoffPath";
import { terminalURL } from "@/api/client";
import type { Action, LogEvent, TaskDetail, Turn } from "@/api/types";
import { SHOWS, Show, activityOf, rowFor } from "@/lib/activity";
import { LOCAL, machineName } from "@/lib/machines";
import { tokens } from "@/lib/tokens";
import { clockTime, dateTime, duration, timeAgo } from "@/lib/time";
import { cn } from "@/lib/cn";
import { useResource } from "@/live/use-resource";
import { useNow } from "@/live/use-now";

// A task in full. The header and a strip of facts stay put. Under them, on
// the left, the agents' plans stay on every tab; on the right, the path of
// handoffs, then the tabs share the rest of the screen: your chat with the
// coordinator, where the task opens, everything that happened, the live
// terminal, the handoffs, and the raw actions, events and details, every
// list but the chat newest first. A handoff opens the activity of its
// turn; the URL keeps the tab, the turn and the rows shown, so any view can
// be linked to and Back returns from it. On a phone the page scrolls
// instead, and the tab keeps most of the screen.

const ACTION_COLUMNS: Column<Action>[] = [
  { header: "Time", className: "w-20", render: (a) => <Mono className="text-ink-muted">{clockTime(a.requestedAt)}</Mono> },
  { header: "Turn", className: "w-20", render: (a) => <Mono className="text-ink-muted">{a.turn}</Mono> },
  { header: "Agent", className: "w-28", render: (a) => <AgentName name={a.agent} /> },
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

const TURN_COLUMNS: Column<Turn>[] = [
  { header: "Turn", className: "w-20", render: (t) => <Mono className="text-ink-muted">{t.id}</Mono> },
  { header: "Agent", className: "w-32", render: (t) => <AgentName name={t.agent} /> },
  { header: "Machine", className: "w-28", render: (t) => <Mono className="text-ink-muted">{machineName(t.worker || LOCAL)}</Mono> },
  { header: "Started", className: "w-24", render: (t) => <Mono className="text-ink-muted">{clockTime(t.startedAt)}</Mono> },
  { header: "Took", className: "w-24", render: (t) => <Mono>{t.ended ? duration(t.startedAt, t.lastEventAt) : "running"}</Mono> },
  { header: "Tokens", className: "w-20 text-right", render: (t) => <Mono>{t.tokens > 0 ? tokens(t.tokens) : ""}</Mono> },
  {
    header: "Outcome",
    className: "w-full max-w-0",
    render: (t) =>
      t.issue ? (
        <span className="flex min-w-0 items-center gap-2">
          <Badge variant="danger">issue</Badge>
          <span className="truncate text-[12px] text-danger" title={t.issue}>
            {t.issue}
          </span>
        </span>
      ) : t.ended ? (
        <Badge>ended</Badge>
      ) : (
        <Badge variant="info">running</Badge>
      ),
  },
];

// Most used first: talk to the coordinator, watch the work, then the records.
const TABS = ["coordinator", "activity", "terminal", "handoffs", "actions", "events", "details"] as const;
type TabId = (typeof TABS)[number];

export function TaskPage() {
  const { taskId = "" } = useParams();
  useBreadcrumb({ label: "Tasks", to: "/tasks" }, { label: taskId });
  const [params, setParams] = useSearchParams();
  const { data: task, error, reload } = useResource<TaskDetail>(`/api/tasks/${encodeURIComponent(taskId)}`);
  const clock = useNow(Boolean(task && task.status !== "done"));

  if (error && !task) {
    return (
      <EmptyState icon={<FileQuestion />} title={`No task ${taskId}`}>
        {error}
      </EmptyState>
    );
  }
  if (!task) return null;

  const asked = params.get("tab") as TabId | null;
  const tab: TabId = asked && TABS.includes(asked) ? asked : TABS[0];
  const turn = params.get("turn") ?? "";
  const row = params.get("row") ?? "";
  const askedShow = params.get("show") as Show | null;
  const show: Show = askedShow && SHOWS.includes(askedShow) ? askedShow : "all";
  const go = (next: { tab: TabId; turn?: string; show?: Show; row?: string }, push = false) => {
    const query: Record<string, string> = { tab: next.tab };
    if (next.turn) query.turn = next.turn;
    if (next.row) query.row = next.row;
    if (next.show && next.show !== "all") query.show = next.show;
    setParams(query, { replace: !push });
  };
  const openTurn = (id: string) => go({ tab: "activity", turn: id }, true);
  // An action or event opens its turn's activity with its own row picked
  // out, and every kind of row shown, so the row is there to see.
  const openRow = (target: { action?: string; event?: LogEvent }, turn = "") =>
    go({ tab: "activity", turn, row: rowFor(items, target) }, true);
  const watch = () => go({ tab: "terminal" });

  const working = task.status !== "done" && task.currentAgent ? task.currentAgent : "";
  const actions = task.actions ?? [];
  const turns = task.turnList ?? [];
  const approvals = task.approvals?.length ?? 0;
  const items = activityOf(task);
  const now: Now | null =
    task.status === "needs you"
      ? { label: "Waiting for you", tone: "warning" }
      : working && task.status === "working"
        ? { label: `${working} is working`, tone: "success" }
        : null;
  const w = task.worktree;
  const turnOfEvent = (e: LogEvent) =>
    e.correlation && turns.some((t) => t.id === e.correlation) ? e.correlation : actions.find((a) => a.id === e.correlation)?.turn;

  const top = error || (task.status === "needs you" && !task.hasQuestion) || approvals > 0 || task.status === "done";
  return (
    <div className="flex flex-col md:h-full md:min-h-0">
      <PageHeader
        title={task.title}
        badges={<TaskStatusBadge status={task.status} />}
        actions={[
          ...(w ? [{ label: "Workspace", icon: <FolderGit2 />, onClick: () => window.open(`/tasks/${task.id}/workspace`, "_blank") }] : []),
          ...(working ? [{ label: "Terminal", icon: <SquareTerminal />, onClick: watch }] : []),
        ]}
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

      <div className="flex shrink-0 flex-wrap items-stretch border-b border-divider">
        <Fact label="Agent">
          {working ? <AgentName name={working} className="text-[12px]" /> : <Mono className="text-ink-muted">none</Mono>}
        </Fact>
        <Fact label="Tokens" className="w-48">
          <TokenMeter used={task.tokensUsed} budget={task.budget} />
        </Fact>
        <Fact label="Turns">
          <Mono>{task.turns}</Mono>
        </Fact>
        <Fact label="Actions">
          <Mono>{actions.length}</Mono>
          {approvals > 0 && <span className="ml-1.5 text-[12px] text-warning">{approvals} waiting</span>}
        </Fact>
        <Fact label="Machine">
          <Mono>{w ? machineName(w.worker) : "none yet"}</Mono>
        </Fact>
        <Fact label="Last active">
          <span className="text-[12px]">{timeAgo(task.lastActiveAt, clock)}</span>
        </Fact>
        <div className="ml-auto flex items-center px-6 py-2">
          <LabelEditor key={task.labels.join(",")} taskId={task.id} labels={task.labels} />
        </div>
      </div>

      {top && (
        <div className="flex max-h-[45%] shrink-0 scrollbar-thin flex-col gap-4 overflow-y-auto border-b border-divider px-6 py-4">
          {error && (
            <Banner tone="warning" title="Showing the last copy: couldn't refresh">
              {error}
            </Banner>
          )}
          <NeedsYouPanel task={task} onChanged={reload} onOpenTerminal={watch} />
          {task.status === "done" && (
            <div className="flex gap-3 rounded-lg border border-success/40 px-4 py-3">
              <span className="flex shrink-0 items-center gap-1.5 self-start text-[13px] font-semibold text-success">
                <CircleCheck className="size-4" />
                Done
              </span>
              <Clamp text={task.summary} lines={2} className="min-w-0 flex-1" />
            </div>
          )}
        </div>
      )}

      <div className="flex flex-col md:min-h-0 md:flex-1 lg:flex-row">
        <aside
          aria-label="Plan"
          className="max-h-64 shrink-0 scrollbar-thin overflow-y-auto border-b border-divider lg:max-h-none lg:w-80 lg:border-r lg:border-b-0"
        >
          <PlanRail turns={turns} coordinator={task.coordinator} onOpenTurn={openTurn} />
        </aside>
        <div className="flex min-w-0 flex-1 flex-col md:min-h-0">
          <HandoffPath turns={turns} coordinator={task.coordinator} done={task.status === "done"} onOpenTurn={openTurn} />
          <div className="shrink-0 pt-1">
            <Tabs
              active={tab}
              onChange={(id) => go({ tab: id as TabId })}
              tabs={[
                {
                  id: "coordinator",
                  label: task.hasQuestion ? "Coordinator · asks you" : "Coordinator",
                  live: turns.some((t) => !t.ended && t.agent === task.coordinator),
                },
                { id: "activity", label: "Activity", count: items.length, live: Boolean(now) },
                { id: "terminal", label: "Terminal", live: Boolean(working) },
                { id: "handoffs", label: "Handoffs", count: task.trail?.length ?? 0 },
                { id: "actions", label: "Actions", count: actions.length },
                { id: "events", label: "Events", count: task.events?.length ?? 0 },
                { id: "details", label: "Details" },
              ]}
            />
          </div>

          <div className="h-[80dvh] min-h-0 md:h-auto md:flex-1">
            {tab === "handoffs" && (
              <Scroll>
                <div className="max-w-4xl px-6 py-5">
                  {task.trail?.length ? (
                    <HandoffTimeline
                      steps={task.trail}
                      turns={turns}
                      actionsIn={(id) => actions.filter((a) => a.turn === id).length}
                      working={task.status === "working" ? working : ""}
                      onOpen={openTurn}
                      onWatch={watch}
                    />
                  ) : (
                    <EmptyState title="No handoffs yet">The coordinator reads the task first.</EmptyState>
                  )}
                </div>
              </Scroll>
            )}
            {tab === "coordinator" && <CoordinatorChat task={task} onSent={reload} />}
            {tab === "activity" && (
              <ActivityFeed
                items={items}
                turn={turn}
                show={show}
                row={row}
                now={now}
                onTurn={(id) => go({ tab: "activity", turn: id, show }, Boolean(id))}
                onShow={(next) => go({ tab: "activity", turn, show: next })}
              />
            )}
            {tab === "terminal" &&
              (working ? (
                // A new key each turn: every turn is a new session.
                <TerminalView key={`${task.id}-${task.turns}`} url={terminalURL(task.id)} title={working} />
              ) : (
                <EmptyState icon={<SquareTerminal />} title="No agent is working on this task">
                  The terminal of the agent at work shows here, live.
                </EmptyState>
              ))}
            {tab === "actions" && (
              <Scroll>
                <ResourceTable
                  columns={ACTION_COLUMNS}
                  rows={[...actions].reverse()}
                  rowKey={(a) => a.id}
                  onOpen={(a) => openRow({ action: a.id }, a.turn)}
                  empty={
                    <EmptyState title="No actions yet">Every tool call an agent makes shows here with the policy's decision.</EmptyState>
                  }
                />
              </Scroll>
            )}
            {tab === "events" && (
              <Scroll>
                <ResourceTable
                  columns={EVENT_COLUMNS}
                  rows={[...(task.events ?? [])].reverse()}
                  rowKey={(e) => String(e.position)}
                  onOpen={(e) => openRow({ event: e }, turnOfEvent(e))}
                />
              </Scroll>
            )}
            {tab === "details" && (
              <Scroll>
                <div className="grid gap-4 p-6 lg:grid-cols-2">
                  <Panel title="Task">
                    <KeyValueList
                      items={[
                        { label: "Id", value: <Mono>{task.id}</Mono> },
                        { label: "Status", value: <TaskStatusBadge status={task.status} /> },
                        { label: "Repo", value: <Mono className="break-all">{task.repo}</Mono> },
                        { label: "Coordinator", value: <AgentName name={task.coordinator} /> },
                        { label: "Budget", value: <Mono>{task.budget > 0 ? `${tokens(task.budget)} tokens` : "no limit"}</Mono> },
                        { label: "Created", value: dateTime(task.createdAt) },
                        { label: "Last active", value: dateTime(task.lastActiveAt) },
                        ...(task.firstTitle ? [{ label: "First title", value: task.firstTitle }] : []),
                      ]}
                    />
                  </Panel>
                  <Panel title="Worktree">
                    {w ? (
                      <KeyValueList
                        items={[
                          { label: "Machine", value: <Mono>{machineName(w.worker)}</Mono> },
                          { label: "Path", value: <Mono className="break-all">{w.path}</Mono> },
                          { label: "Branch", value: <Mono className="break-all">{w.branch}</Mono> },
                          { label: "Base", value: <Mono>{w.base.slice(0, 10)}</Mono> },
                          { label: "Port", value: <Mono>{w.port}</Mono> },
                        ]}
                      />
                    ) : (
                      <p className="text-[12px] text-ink-muted">
                        {task.status === "done" ? "Removed when the task was done." : "Not set up yet."}
                      </p>
                    )}
                  </Panel>
                  <Panel title="Turns" aside={<Mono>{turns.length}</Mono>} flush className="lg:col-span-2">
                    <ResourceTable
                      columns={TURN_COLUMNS}
                      rows={[...turns].reverse()}
                      rowKey={(t) => t.id}
                      onOpen={(t) => openTurn(t.id)}
                      empty={<p className="px-4 py-3 text-[12px] text-ink-muted">No turns yet.</p>}
                    />
                  </Panel>
                </div>
              </Scroll>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// One fact in the strip under the header.
function Fact({ label, className, children }: { label: string; className?: string; children: ReactNode }) {
  return (
    <div className={cn("flex min-w-0 flex-col justify-center gap-1 border-r border-divider px-6 py-2", className)}>
      <span className="text-[11px] text-ink-muted">{label}</span>
      <div className="flex min-h-5 items-center">{children}</div>
    </div>
  );
}

function Scroll({ children }: { children: ReactNode }) {
  return <div className="h-full scrollbar-thin overflow-y-auto">{children}</div>;
}

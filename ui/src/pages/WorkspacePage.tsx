import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { FileQuestion, GitBranch, MessagesSquare } from "lucide-react";
import { LogoTile } from "@/components/atoms/LogoTile";
import { Banner } from "@/components/molecules/Banner";
import { EmptyState } from "@/components/molecules/EmptyState";
import { CHANGE_COLOR, FileTree, WorkspaceFile } from "@/components/molecules/FileTree";
import { FileView } from "@/components/molecules/FileView";
import { Mono } from "@/components/molecules/Mono";
import { TaskStatusBadge } from "@/components/molecules/StatusBadges";
import { WorkspaceDock } from "@/components/organisms/WorkspaceDock";
import { Commit, Section, SourceControl } from "@/components/organisms/SourceControl";
import type { Agent, TaskDetail } from "@/api/types";
import { AgentColorsProvider } from "@/live/agent-colors";
import { useResource } from "@/live/use-resource";
import { modKey } from "@/lib/shortcuts";
import { readStored, writeStored } from "@/lib/stored";
import { cn } from "@/lib/cn";

// A task's Workspace, in its own tab, laid out like an editor's
// git view: source control (what isn't committed, then the branch's commits,
// newest first) and the explorer, every file with its git status. A file
// opens in Monaco next to its last committed version; in a commit, next to
// the version before it. Ctrl+J opens the dock: the coordinator, the agent's
// terminal, and who has worked here. Read only: the agents do the work.

interface Contents {
  path: string;
  now: string;
  before: string;
  binary: boolean;
  tooBig: boolean;
}

const dockKey = "agentry-workspace-dock";

export function WorkspacePage() {
  const { taskId = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const selected = params.get("file") ?? "";
  const commit = params.get("commit") ?? "";
  const task_ = encodeURIComponent(taskId);
  const taskResource = useResource<TaskDetail>(`/api/tasks/${task_}`);
  const task = taskResource.data;
  const agents = useResource<Agent[]>("/api/agents").data ?? [];
  const listing = useResource<{ base: string; files: WorkspaceFile[] }>(`/api/tasks/${task_}/files`);
  const commits = useResource<Commit[]>(`/api/tasks/${task_}/commits`).data ?? [];
  const files = listing.data?.files ?? [];
  const changes = files.filter((f) => f.change);
  const open = (path: string, inCommit: string) =>
    setParams(inCommit ? { file: path, commit: inCommit } : { file: path }, { replace: true });

  const inCommit = commits.find((c) => c.hash === commit);
  const file = inCommit ? inCommit.files.find((f) => f.path === selected) : files.find((f) => f.path === selected);
  const contents = useResource<Contents>(
    selected ? `/api/tasks/${task_}/file?path=${encodeURIComponent(selected)}${commit ? `&commit=${commit}` : ""}` : null,
  ).data;

  // The dock: Ctrl+J, from anywhere on the page.
  const column = useRef<HTMLDivElement>(null);
  const [dockOpen, setDockOpen] = useState(false);
  const [dockHeight, setDockHeight] = useState(() => Number(readStored(dockKey)) || 280);
  const resize = useCallback((height: number) => {
    setDockHeight(Math.round(Math.max(120, Math.min(column.current?.clientHeight ?? height, height))));
  }, []);
  useEffect(() => writeStored(dockKey, String(dockHeight)), [dockHeight]);
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "j") {
        event.preventDefault();
        event.stopPropagation();
        setDockOpen((open) => !open);
      }
    }
    window.addEventListener("keydown", onKey, { capture: true });
    return () => window.removeEventListener("keydown", onKey, { capture: true });
  }, []);
  useEffect(() => {
    document.title = `${taskId} workspace · Agentry`;
  }, [taskId]);

  const counts = { M: 0, A: 0, D: 0, U: 0 };
  for (const f of changes) if (f.change) counts[f.change]++;
  const showsDiff = file?.change === "M" || file?.change === "D";

  return (
    <AgentColorsProvider agents={agents}>
      <div className="flex h-full flex-col">
        <header className="flex h-12 shrink-0 items-center gap-3 border-b border-divider bg-surface-1 px-3">
          <Link to={`/tasks/${taskId}`} className="flex items-center gap-2" title="Back to the task">
            <LogoTile />
          </Link>
          <Mono className="text-ink-muted">{taskId}</Mono>
          <span className="min-w-0 truncate text-[13px] font-medium text-ink-primary">{task?.title}</span>
          {task && <TaskStatusBadge status={task.status} />}
          <span className="ml-auto flex items-center gap-3 font-mono text-[12px]">
            {task?.worktree && (
              <span className="flex items-center gap-1.5 text-ink-muted max-md:hidden">
                <GitBranch className="size-3.5" />
                {task.worktree.branch}
              </span>
            )}
            {changes.length === 0 ? (
              <span className="text-ink-faint">nothing to commit</span>
            ) : (
              (["M", "A", "D", "U"] as const).map(
                (c) =>
                  counts[c] > 0 && (
                    <span key={c} className={CHANGE_COLOR[c]}>
                      {c} {counts[c]}
                    </span>
                  ),
              )
            )}
          </span>
          <button
            type="button"
            onClick={() => setDockOpen((open) => !open)}
            title={`The coordinator, Claude and the agent's terminal (${modKey} J)`}
            className={cn(
              "flex h-7 cursor-pointer items-center gap-1.5 rounded-sm px-2 text-[12px] text-ink-muted hover:bg-surface-2 hover:text-ink-primary",
              dockOpen && "bg-surface-3 text-ink-primary",
            )}
          >
            <MessagesSquare className="size-3.5" /> Dock
          </button>
        </header>

        <div className="flex min-h-0 flex-1">
          <aside className="flex w-80 shrink-0 scrollbar-thin flex-col overflow-y-auto border-r border-divider bg-surface-1 py-1">
            {listing.error ? (
              <p className="px-3 py-4 text-[12px] text-ink-muted">{listing.error}</p>
            ) : (
              <>
                <SourceControl changes={changes} commits={commits} selected={{ path: selected, commit }} onOpen={open} />
                <Section title="Explorer" count={files.length} startOpen>
                  <FileTree files={files} selected={commit ? "" : selected} onSelect={(path) => open(path, "")} changedOnly={false} />
                </Section>
              </>
            )}
          </aside>

          <div ref={column} className="flex min-w-0 flex-1 flex-col">
            <main className="flex min-h-0 flex-1 flex-col">
              {!selected ? (
                <EmptyState icon={<FileQuestion />} title="Pick a file">
                  A changed file opens next to its last committed version; a file in a commit, next to the version before it.
                </EmptyState>
              ) : (
                <>
                  <div className="flex h-9 shrink-0 items-center gap-2 border-b border-divider px-3">
                    <Mono className="truncate text-ink-primary">{selected}</Mono>
                    {file?.change && (
                      <span className={cn("font-mono text-[11px]", CHANGE_COLOR[file.change])}>{CHANGE_NAME[file.change]}</span>
                    )}
                    {inCommit ? (
                      <span className="min-w-0 truncate text-[11px] text-ink-muted">
                        in <Mono className="text-[11px]">{inCommit.hash.slice(0, 7)}</Mono> {inCommit.subject}
                      </span>
                    ) : (
                      file?.change && <span className="text-[11px] text-ink-faint">not committed</span>
                    )}
                  </div>
                  <div className="min-h-0 flex-1">
                    {!contents ? null : contents.binary || contents.tooBig ? (
                      <div className="p-6">
                        <Banner tone="info" title={contents.binary ? "Not a text file" : "Too big to show"} />
                      </div>
                    ) : (
                      <FileView
                        key={`${commit}:${selected}`}
                        path={selected}
                        now={contents.now}
                        before={showsDiff ? contents.before : undefined}
                      />
                    )}
                  </div>
                </>
              )}
            </main>
            {dockOpen && task && (
              <WorkspaceDock
                task={task}
                height={dockHeight}
                onResize={resize}
                onClose={() => setDockOpen(false)}
                onChanged={taskResource.reload}
              />
            )}
          </div>
        </div>
      </div>
    </AgentColorsProvider>
  );
}

const CHANGE_NAME = { M: "modified", A: "added", D: "deleted", U: "untracked" };

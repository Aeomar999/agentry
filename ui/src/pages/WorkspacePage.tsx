import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { FileQuestion, SquareTerminal } from "lucide-react";
import { LogoTile } from "@/components/atoms/LogoTile";
import { Banner } from "@/components/molecules/Banner";
import { EmptyState } from "@/components/molecules/EmptyState";
import { FileTree, WorkspaceFile } from "@/components/molecules/FileTree";
import { FileView } from "@/components/molecules/FileView";
import { FilterChip } from "@/components/molecules/FilterChip";
import { Mono } from "@/components/molecules/Mono";
import { TaskStatusBadge } from "@/components/molecules/StatusBadges";
import { WorkspaceDock } from "@/components/organisms/WorkspaceDock";
import type { Agent, TaskDetail } from "@/api/types";
import { AgentColorsProvider } from "@/live/agent-colors";
import { useResource } from "@/live/use-resource";
import { modKey } from "@/lib/shortcuts";
import { readStored, writeStored } from "@/lib/stored";
import { cn } from "@/lib/cn";

// A peek into a task's workspace, in its own tab: the folder tree with what
// changed since the task started, each file in Monaco (changed ones as a
// diff), and Ctrl+J for the dock with the agent's terminal and who has
// worked here. Read only: the agents do the work.

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
  const task = useResource<TaskDetail>(`/api/tasks/${encodeURIComponent(taskId)}`).data;
  const agents = useResource<Agent[]>("/api/agents").data ?? [];
  const listing = useResource<{ base: string; files: WorkspaceFile[] }>(`/api/tasks/${encodeURIComponent(taskId)}/files`);
  const files = listing.data?.files ?? [];
  const [changedOnly, setChangedOnly] = useState(true);
  const file = files.find((f) => f.path === selected);
  const contents = useResource<Contents>(
    selected ? `/api/tasks/${encodeURIComponent(taskId)}/file?path=${encodeURIComponent(selected)}` : null,
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

  const counts = { M: 0, A: 0, D: 0 };
  for (const f of files) if (f.change) counts[f.change]++;

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
          {task?.worktree && <Mono className="text-ink-faint max-md:hidden">{task.worktree.branch}</Mono>}
          <span className="ml-auto flex items-center gap-3 font-mono text-[12px]">
            <span className="text-warning">M {counts.M}</span>
            <span className="text-success">A {counts.A}</span>
            <span className="text-danger">D {counts.D}</span>
          </span>
          <button
            type="button"
            onClick={() => setDockOpen((open) => !open)}
            title={`The agent's terminal (${modKey} J)`}
            className={cn(
              "flex h-7 cursor-pointer items-center gap-1.5 rounded-sm px-2 text-[12px] text-ink-muted hover:bg-surface-2 hover:text-ink-primary",
              dockOpen && "bg-surface-3 text-ink-primary",
            )}
          >
            <SquareTerminal className="size-3.5" /> Terminal
          </button>
        </header>

        <div className="flex min-h-0 flex-1">
          <aside className="flex w-72 shrink-0 flex-col border-r border-divider bg-surface-1">
            <div className="flex h-9 shrink-0 items-center gap-2 border-b border-divider px-2">
              <FilterChip
                label="Changed"
                count={counts.M + counts.A + counts.D}
                active={changedOnly}
                onClick={() => setChangedOnly(true)}
              />
              <FilterChip label="All files" count={files.length} active={!changedOnly} onClick={() => setChangedOnly(false)} />
            </div>
            <div className="min-h-0 flex-1 scrollbar-thin overflow-y-auto">
              {listing.error ? (
                <p className="px-3 py-4 text-[12px] text-ink-muted">{listing.error}</p>
              ) : (
                <FileTree
                  files={files}
                  selected={selected}
                  onSelect={(path) => setParams({ file: path }, { replace: true })}
                  changedOnly={changedOnly}
                />
              )}
            </div>
          </aside>

          <div ref={column} className="flex min-w-0 flex-1 flex-col">
            <main className="flex min-h-0 flex-1 flex-col">
              {!selected ? (
                <EmptyState icon={<FileQuestion />} title="Pick a file">
                  Changed files open side by side with how they were when the task started.
                </EmptyState>
              ) : (
                <>
                  <div className="flex h-9 shrink-0 items-center gap-2 border-b border-divider px-3">
                    <Mono className="truncate text-ink-primary">{selected}</Mono>
                    {file?.change && (
                      <span className="font-mono text-[11px] text-ink-muted">
                        {{ M: "modified", A: "added", D: "deleted" }[file.change]}
                      </span>
                    )}
                  </div>
                  <div className="min-h-0 flex-1">
                    {!contents ? null : contents.binary || contents.tooBig ? (
                      <div className="p-6">
                        <Banner tone="info" title={contents.binary ? "Not a text file" : "Too big to show"} />
                      </div>
                    ) : (
                      <FileView
                        key={selected}
                        path={selected}
                        now={contents.now}
                        before={file?.change === "M" || file?.change === "D" ? contents.before : undefined}
                      />
                    )}
                  </div>
                </>
              )}
            </main>
            {dockOpen && task && <WorkspaceDock task={task} height={dockHeight} onResize={resize} onClose={() => setDockOpen(false)} />}
          </div>
        </div>
      </div>
    </AgentColorsProvider>
  );
}

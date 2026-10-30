import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { Columns2, FileQuestion, GitBranch, MessagesSquare } from "lucide-react";
import { LogoTile } from "@/components/atoms/LogoTile";
import { Banner } from "@/components/molecules/Banner";
import { EmptyState } from "@/components/molecules/EmptyState";
import { CHANGE_COLOR, FileTree, WorkspaceFile } from "@/components/molecules/FileTree";
import { FileView } from "@/components/molecules/FileView";
import { Mono } from "@/components/molecules/Mono";
import { TaskStatusBadge } from "@/components/molecules/StatusBadges";
import { WorkspaceDock } from "@/domains/tasks/workspace/WorkspaceDock";
import { Commit, FileContext, Section, SourceControl } from "@/domains/tasks/workspace/SourceControl";
import { SvgPreview, WorkspacePreview, previewable } from "@/domains/tasks/workspace/WorkspacePreview";
import { ContextMenu, MenuItem } from "@/components/molecules/ContextMenu";
import type { Agent, TaskDetail } from "@/api/types";
import { AgentColorsProvider } from "@/live/agent-colors";
import { useResource } from "@/live/use-resource";
import { modKey } from "@/lib/shortcuts";
import { readStored, writeStored } from "@/lib/stored";
import { cn } from "@/lib/cn";
import { shownId } from "@/lib/shown-id";

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
  data?: string; // a binary file's bytes, in base64
  tooBig: boolean;
}

// How a file is shown: as it reads (Markdown, SVG, images), as its code, as
// its changes, or its code with its preview to the side. In the address.
type View = "preview" | "code" | "changes" | "split";
const IMAGE = /\.(png|jpe?g|gif|webp)$/i;

const dockKey = "agentry-workspace-dock";

export function WorkspacePage() {
  const { taskId = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const selected = params.get("file") ?? "";
  const commit = params.get("commit") ?? "";
  const askedView = params.get("view") as View | null;
  const [menu, setMenu] = useState<{ x: number; y: number; path: string; commit: string; change: string } | null>(null);
  const task_ = encodeURIComponent(taskId);
  const taskResource = useResource<TaskDetail>(`/api/tasks/${task_}`);
  const task = taskResource.data;
  const agents = useResource<Agent[]>("/api/agents").data ?? [];
  const listing = useResource<{ base: string; files: WorkspaceFile[] }>(`/api/tasks/${task_}/files`);
  const commits = useResource<Commit[]>(`/api/tasks/${task_}/commits`).data ?? [];
  const files = listing.data?.files ?? [];
  const changes = files.filter((f) => f.change);
  const open = (path: string, inCommit: string, view?: View) =>
    setParams({ file: path, ...(inCommit ? { commit: inCommit } : {}), ...(view ? { view } : {}) }, { replace: true });
  const showView = (view: View) => open(selected, commit, view);

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
    document.title = `${shownId(taskId, task?.kind)} workspace · Agentry`;
  }, [taskId, task?.kind]);

  const counts = { M: 0, A: 0, D: 0, U: 0 };
  for (const f of changes) if (f.change) counts[f.change]++;
  const showsDiff = file?.change === "M" || file?.change === "D";
  const canPreview = previewable(selected) || IMAGE.test(selected);
  const views: View[] = [...(canPreview ? (["preview"] as View[]) : []), "code", ...(showsDiff ? (["changes"] as View[]) : [])];
  const view: View =
    askedView && (views.includes(askedView) || (askedView === "split" && previewable(selected)))
      ? askedView
      : canPreview
        ? "preview"
        : showsDiff
          ? "changes"
          : "code";

  // The right-click menu on a file, wherever it is listed.
  const onContext: FileContext = (event, path, inCommit, change) =>
    setMenu({ x: event.clientX, y: event.clientY, path, commit: inCommit, change });
  const menuItems = (m: NonNullable<typeof menu>): MenuItem[] => {
    const changed = m.change === "M" || m.change === "D";
    const full = `${task?.worktree?.path ?? ""}/${m.path}`;
    return [
      ...(previewable(m.path) || IMAGE.test(m.path)
        ? [
            { label: "Open Preview", onClick: () => open(m.path, m.commit, "preview") },
            ...(previewable(m.path) ? [{ label: "Open Preview to the Side", onClick: () => open(m.path, m.commit, "split") }] : []),
          ]
        : []),
      { label: "Open Code", onClick: () => open(m.path, m.commit, "code") },
      { label: "Open Changes", disabled: !changed, onClick: () => open(m.path, m.commit, "changes") },
      "separator",
      { label: "Copy Path", onClick: () => navigator.clipboard?.writeText(m.path) },
      { label: "Copy Full Path", disabled: !task?.worktree, onClick: () => navigator.clipboard?.writeText(full) },
    ];
  };

  return (
    <AgentColorsProvider agents={agents}>
      <div className="flex h-full flex-col">
        <header className="flex h-12 shrink-0 items-center gap-3 border-b border-divider bg-surface-1 px-3">
          <Link to={`/tasks/${taskId}`} className="flex items-center gap-2" title="Back to the task">
            <LogoTile />
          </Link>
          <Mono className="text-ink-muted">{shownId(taskId, task?.kind)}</Mono>
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
                <SourceControl
                  changes={changes}
                  commits={commits}
                  selected={{ path: selected, commit }}
                  onOpen={(p, c) => open(p, c)}
                  onContext={onContext}
                />
                <Section title="Explorer" count={files.length} startOpen>
                  <FileTree
                    files={files}
                    selected={commit ? "" : selected}
                    onSelect={(path) => open(path, "")}
                    changedOnly={false}
                    onContext={(event, path, change) => onContext(event, path, "", change)}
                  />
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
                    <span className="ml-auto flex items-center gap-2">
                      {views.length > 1 && (
                        <span role="radiogroup" aria-label="View" className="flex rounded-md border border-divider p-0.5">
                          {views.map((v) => (
                            <button
                              key={v}
                              type="button"
                              role="radio"
                              aria-checked={view === v}
                              onClick={() => showView(v)}
                              className={cn(
                                "h-6 cursor-pointer rounded-sm px-2 text-[11px] capitalize",
                                view === v ? "bg-surface-3 text-ink-primary" : "text-ink-muted hover:text-ink-primary",
                              )}
                            >
                              {v}
                            </button>
                          ))}
                        </span>
                      )}
                      {previewable(selected) && (
                        <button
                          type="button"
                          onClick={() => showView(view === "split" ? "preview" : "split")}
                          aria-pressed={view === "split"}
                          title="Open Preview to the Side"
                          className={cn(
                            "inline-flex size-7 cursor-pointer items-center justify-center rounded-sm text-ink-muted hover:bg-surface-2 hover:text-ink-primary [&>svg]:size-4",
                            view === "split" && "bg-surface-3 text-ink-primary",
                          )}
                        >
                          <Columns2 />
                        </button>
                      )}
                    </span>
                  </div>
                  <div className="min-h-0 flex-1">
                    {!contents ? null : contents.binary && IMAGE.test(selected) && contents.data ? (
                      <SvgPreview
                        src={`data:image/${selected.split(".").pop()!.toLowerCase().replace("jpg", "jpeg")};base64,${contents.data}`}
                        name={selected}
                      />
                    ) : contents.binary || contents.tooBig ? (
                      <div className="p-6">
                        <Banner tone="info" title={contents.binary ? "Not a text file" : "Too big to show"} />
                      </div>
                    ) : view === "preview" ? (
                      <WorkspacePreview
                        key={`${commit}:${selected}`}
                        taskId={taskId}
                        path={selected}
                        commit={commit}
                        text={contents.now}
                        onOpen={(p) => open(p, "")}
                      />
                    ) : view === "split" ? (
                      <div className="grid h-full grid-cols-2">
                        <div className="min-w-0 border-r border-divider">
                          <FileView key={`${commit}:${selected}:code`} path={selected} now={contents.now} />
                        </div>
                        <div className="min-w-0">
                          <WorkspacePreview
                            taskId={taskId}
                            path={selected}
                            commit={commit}
                            text={contents.now}
                            onOpen={(p) => open(p, "")}
                            outline={false}
                          />
                        </div>
                      </div>
                    ) : (
                      <FileView
                        key={`${commit}:${selected}:${view}`}
                        path={selected}
                        now={contents.now}
                        before={view === "changes" ? contents.before : undefined}
                      />
                    )}
                  </div>
                </>
              )}
            </main>
            {menu && <ContextMenu x={menu.x} y={menu.y} items={menuItems(menu)} onClose={() => setMenu(null)} />}
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

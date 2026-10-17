import { useState } from "react";
import { ChevronRight, GitCommitHorizontal } from "lucide-react";
import { CHANGE_COLOR, WorkspaceFile } from "@/components/molecules/FileTree";
import { Mono } from "@/components/molecules/Mono";
import { cn } from "@/lib/cn";
import { timeAgo } from "@/lib/time";

// The workspace's git, as an editor's source control view shows it: the
// changes not committed yet, then the branch's commits since the task
// started, newest first. A commit opens to the files it changed; a file
// opens next to its last committed version, or, in a commit, next to the
// version before it. A new round of work is the commits at the top. Both
// sections start closed, with their counts showing.

export interface Commit {
  hash: string;
  author: string;
  at: string;
  subject: string;
  files: WorkspaceFile[];
}

interface SourceControlProps {
  changes: WorkspaceFile[]; // not committed yet
  commits: Commit[];
  selected: { path: string; commit: string };
  onOpen: (path: string, commit: string) => void;
  onContext?: FileContext; // right-click on a file
}

export type FileContext = (event: React.MouseEvent, path: string, commit: string, change: WorkspaceFile["change"]) => void;

export function SourceControl({ changes, commits, selected, onOpen, onContext }: SourceControlProps) {
  return (
    <div className="flex flex-col">
      <Section title="Changes" count={changes.length}>
        {changes.length === 0 ? (
          <p className="px-3 pb-2 text-[12px] text-ink-faint">Nothing waiting to be committed.</p>
        ) : (
          changes.map((file) => (
            <FileRow
              key={file.path}
              file={file}
              active={selected.path === file.path && !selected.commit}
              onClick={() => onOpen(file.path, "")}
              onContext={onContext && ((event) => onContext(event, file.path, "", file.change))}
            />
          ))
        )}
      </Section>
      <Section title="Commits" count={commits.length}>
        {commits.length === 0 && <p className="px-3 pb-2 text-[12px] text-ink-faint">No commits on the branch yet.</p>}
        {commits.map((commit) => (
          <CommitRow key={commit.hash} commit={commit} selected={selected} onOpen={onOpen} onContext={onContext} />
        ))}
      </Section>
    </div>
  );
}

// A sidebar section: a heading that opens and closes it, with its count.
export function Section({
  title,
  count,
  startOpen = false, // source control starts closed, its counts showing; the explorer starts open
  children,
}: {
  title: string;
  count: number;
  startOpen?: boolean;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(startOpen);
  return (
    <section>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex h-7 w-full cursor-pointer items-center gap-1 px-1.5 text-[11px] font-semibold tracking-wider text-ink-muted uppercase hover:text-ink-primary"
      >
        <ChevronRight className={cn("size-3.5 transition-transform duration-(--dur-fast)", open && "rotate-90")} />
        {title}
        <span className="ml-auto rounded-sm bg-surface-3 px-1.5 font-mono text-[10px] leading-4 text-ink-secondary">{count}</span>
      </button>
      {open && <div className="pb-1">{children}</div>}
    </section>
  );
}

// A changed file: its name, then its folder faint, then its letter.
function FileRow({
  file,
  active,
  onClick,
  onContext,
  inset = false,
}: {
  file: WorkspaceFile;
  active: boolean;
  onClick: () => void;
  onContext?: (event: React.MouseEvent) => void;
  inset?: boolean;
}) {
  const slash = file.path.lastIndexOf("/");
  const name = file.path.slice(slash + 1);
  const folder = slash > 0 ? file.path.slice(0, slash) : "";
  return (
    <button
      type="button"
      onClick={onClick}
      onContextMenu={onContext && ((event) => (event.preventDefault(), onContext(event)))}
      title={file.path}
      className={cn(
        "flex h-6 w-full cursor-pointer items-center gap-1.5 pr-2 text-left text-[12.5px]",
        inset ? "pl-9" : "pl-5",
        active ? "bg-surface-3 text-ink-primary" : "text-ink-secondary hover:bg-surface-2",
      )}
    >
      <span
        className={cn(
          "shrink-0 truncate",
          file.change === "D" && "line-through decoration-ink-faint",
          file.change && CHANGE_COLOR[file.change],
        )}
      >
        {name}
      </span>
      <span className="min-w-0 flex-1 truncate text-[11px] text-ink-faint">{folder}</span>
      {file.change && <span className={cn("font-mono text-[11px]", CHANGE_COLOR[file.change])}>{file.change}</span>}
    </button>
  );
}

function CommitRow({
  commit,
  selected,
  onOpen,
  onContext,
}: {
  commit: Commit;
  selected: { path: string; commit: string };
  onOpen: (path: string, commit: string) => void;
  onContext?: FileContext;
}) {
  const [open, setOpen] = useState(selected.commit === commit.hash); // closed until you open it
  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        title={`${commit.hash.slice(0, 10)} · ${commit.author}`}
        className="flex w-full cursor-pointer items-start gap-1.5 py-1 pr-2 pl-1.5 text-left hover:bg-surface-2"
      >
        <ChevronRight
          className={cn("mt-0.5 size-3.5 shrink-0 text-ink-faint transition-transform duration-(--dur-fast)", open && "rotate-90")}
        />
        <GitCommitHorizontal className="mt-0.5 size-3.5 shrink-0 text-ink-faint" />
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="truncate text-[12.5px] text-ink-primary">{commit.subject}</span>
          <span className="flex gap-2 text-[11px] text-ink-faint">
            <Mono className="text-[11px]">{commit.hash.slice(0, 7)}</Mono>
            {timeAgo(commit.at)}
            <span className="ml-auto">
              {commit.files.length} {commit.files.length === 1 ? "file" : "files"}
            </span>
          </span>
        </span>
      </button>
      {open &&
        commit.files.map((file) => (
          <FileRow
            key={file.path}
            file={file}
            inset
            active={selected.commit === commit.hash && selected.path === file.path}
            onClick={() => onOpen(file.path, commit.hash)}
            onContext={onContext && ((event) => onContext(event, file.path, commit.hash, file.change))}
          />
        ))}
    </div>
  );
}

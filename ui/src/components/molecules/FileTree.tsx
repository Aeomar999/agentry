import { useMemo, useState } from "react";
import { ChevronRight, File, Folder, FolderOpen } from "lucide-react";
import { cn } from "@/lib/cn";

export interface WorkspaceFile {
  path: string;
  change: Change;
}

// A file's git status, as an editor shows it (internal/platform/workspace): modified,
// added, deleted, untracked, or "" as committed.
export type Change = "" | "M" | "A" | "D" | "U";

interface Node {
  name: string;
  path: string;
  file?: WorkspaceFile;
  children: Node[];
  changes: number; // changed files at or below this node
  change: Change; // the one its folder shows, from the files below it
}

export const CHANGE_COLOR = { M: "text-warning", A: "text-success", D: "text-danger", U: "text-success" };

// Which change a folder shows when the files below it differ: a modified
// file before a deleted one, before a new one.
const WEIGHT: Record<Change, number> = { "": 0, U: 1, A: 2, D: 3, M: 4 };

function build(files: WorkspaceFile[]): Node {
  const root: Node = { name: "", path: "", children: [], changes: 0, change: "" };
  for (const file of files) {
    let node = root;
    const parts = file.path.split("/");
    parts.forEach((name, i) => {
      if (file.change) node.changes++;
      if (WEIGHT[file.change] > WEIGHT[node.change]) node.change = file.change;
      let child = node.children.find((c) => c.name === name && (i === parts.length - 1) === Boolean(c.file));
      if (!child) {
        child = { name, path: parts.slice(0, i + 1).join("/"), children: [], changes: 0, change: "" };
        node.children.push(child);
      }
      if (i === parts.length - 1) {
        child.file = file;
        if (file.change) child.changes++;
      }
      node = child;
    });
  }
  const sort = (node: Node) => {
    node.children.sort((a, b) => Number(Boolean(a.file)) - Number(Boolean(b.file)) || a.name.localeCompare(b.name));
    node.children.forEach(sort);
  };
  sort(root);
  return root;
}

interface FileTreeProps {
  files: WorkspaceFile[];
  selected: string;
  onSelect: (path: string) => void;
  changedOnly: boolean;
  empty?: string; // what to say when there is nothing to show
  onContext?: (event: React.MouseEvent, path: string, change: Change) => void; // right-click on a file
}

// The workspace's folders and files, each with its git status against the
// last commit, as an editor's explorer shows it: M modified, A added, D
// deleted, U untracked; a committed file has none. A folder holding changes
// shows them too, as an editor does: its name in their colour and a dot, all
// the way up. Such folders are open from the start.
export function FileTree({ files, selected, onSelect, changedOnly, empty, onContext }: FileTreeProps) {
  const shown = changedOnly ? files.filter((f) => f.change) : files;
  const root = useMemo(() => build(shown), [shown]);
  const [toggled, setToggled] = useState<Record<string, boolean>>({});
  const isOpen = (node: Node) => toggled[node.path] ?? (node.changes > 0 || changedOnly);

  function render(node: Node, depth: number) {
    return node.children.map((child) => {
      const pad = { paddingLeft: 8 + depth * 12 };
      if (child.file) {
        const change = child.file.change;
        return (
          <button
            key={child.path}
            type="button"
            onClick={() => onSelect(child.path)}
            onContextMenu={onContext ? (event) => (event.preventDefault(), onContext(event, child.path, change)) : undefined}
            style={pad}
            className={cn(
              "flex h-6 w-full cursor-pointer items-center gap-1.5 pr-2 text-left text-[12.5px] [&>svg]:size-3.5 [&>svg]:shrink-0",
              child.path === selected ? "bg-surface-3 text-ink-primary" : "text-ink-secondary hover:bg-surface-2",
              change === "D" && "line-through decoration-ink-faint",
            )}
          >
            <span className="w-3.5 shrink-0" />
            <File className="text-ink-faint" />
            <span className={cn("min-w-0 flex-1 truncate", change && CHANGE_COLOR[change])}>{child.name}</span>
            {change && <span className={cn("font-mono text-[11px]", CHANGE_COLOR[change])}>{change}</span>}
          </button>
        );
      }
      const open = isOpen(child);
      return (
        <div key={child.path}>
          <button
            type="button"
            onClick={() => setToggled((t) => ({ ...t, [child.path]: !open }))}
            style={pad}
            className="flex h-6 w-full cursor-pointer items-center gap-1.5 pr-2 text-left text-[12.5px] text-ink-secondary hover:bg-surface-2 [&>svg]:size-3.5 [&>svg]:shrink-0"
          >
            <ChevronRight className={cn("text-ink-faint transition-transform duration-(--dur-fast)", open && "rotate-90")} />
            {open ? <FolderOpen className="text-ink-faint" /> : <Folder className="text-ink-faint" />}
            <span className={cn("min-w-0 flex-1 truncate", child.change && CHANGE_COLOR[child.change])}>{child.name}</span>
            {child.change && (
              <span
                className={cn("mr-0.5 size-1.5 shrink-0 rounded-full bg-current", CHANGE_COLOR[child.change])}
                title={`${child.changes} ${child.changes === 1 ? "change" : "changes"} inside`}
                aria-label={`${child.changes} changes inside`}
              />
            )}
          </button>
          {open && render(child, depth + 1)}
        </div>
      );
    });
  }

  if (shown.length === 0)
    return <p className="px-3 py-4 text-[12px] text-ink-muted">{empty ?? (changedOnly ? "Nothing has changed yet." : "No files.")}</p>;
  return <div className="py-1">{render(root, 0)}</div>;
}

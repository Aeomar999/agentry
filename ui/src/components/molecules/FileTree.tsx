import { useMemo, useState } from "react";
import { ChevronRight, File, Folder, FolderOpen } from "lucide-react";
import { cn } from "@/lib/cn";

export interface WorkspaceFile {
  path: string;
  change: "" | "M" | "A" | "D"; // since the task started: modified, added, deleted
}

interface Node {
  name: string;
  path: string;
  file?: WorkspaceFile;
  children: Node[];
  changes: number; // changed files at or below this node
}

const CHANGE_COLOR = { M: "text-warning", A: "text-success", D: "text-danger" };

function build(files: WorkspaceFile[]): Node {
  const root: Node = { name: "", path: "", children: [], changes: 0 };
  for (const file of files) {
    let node = root;
    const parts = file.path.split("/");
    parts.forEach((name, i) => {
      if (file.change) node.changes++;
      let child = node.children.find((c) => c.name === name && (i === parts.length - 1) === Boolean(c.file));
      if (!child) {
        child = { name, path: parts.slice(0, i + 1).join("/"), children: [], changes: 0 };
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
}

// The workspace's folders and files, with what changed since the task
// started: M modified, A added, D deleted. Folders holding changes are
// open from the start.
export function FileTree({ files, selected, onSelect, changedOnly }: FileTreeProps) {
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
            <span className="min-w-0 flex-1 truncate">{child.name}</span>
            {child.changes > 0 && <span className="font-mono text-[11px] text-ink-faint">{child.changes}</span>}
          </button>
          {open && render(child, depth + 1)}
        </div>
      );
    });
  }

  if (shown.length === 0)
    return <p className="px-3 py-4 text-[12px] text-ink-muted">{changedOnly ? "Nothing has changed yet." : "No files."}</p>;
  return <div className="py-1">{render(root, 0)}</div>;
}

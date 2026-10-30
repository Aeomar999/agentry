import { DragEvent, useRef, useState } from "react";
import { Download, FileUp, Paperclip, Trash2 } from "lucide-react";
import { api } from "@/api/client";
import { Button } from "@/components/atoms/Button";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Mono } from "@/components/molecules/Mono";
import { ResourceTable, Column } from "@/components/molecules/ResourceTable";
import { useToast } from "@/shell/Toasts";
import { downloadURL, share, sizeOf, SharedFile } from "@/lib/shared";
import { timeAgo } from "@/lib/time";
import { cn } from "@/lib/cn";
import { shownId } from "@/lib/shown-id";

// A task's shared files: drop files here (or choose them) to share them,
// download any, take one away. Agents find them in .agentry/files of their
// worktree, on whichever machine works next, and leave theirs there too.
// An archived task's only show: it takes no events.
export function SharedFiles({
  task,
  kind,
  files,
  readOnly,
  onChanged,
}: {
  task: string;
  kind?: string;
  files: SharedFile[];
  readOnly?: boolean;
  onChanged: () => void;
}) {
  const notify = useToast();
  const picker = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const [sending, setSending] = useState(0);

  async function send(list: FileList | File[]) {
    const chosen = [...list];
    setSending(chosen.length);
    let failed = 0;
    for (const file of chosen) {
      try {
        // A folder dropped or chosen keeps its layout.
        await share(task, file, (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name);
      } catch (failure) {
        failed++;
        notify("danger", `${file.name} wasn't shared`, (failure as Error).message);
      }
    }
    setSending(0);
    if (chosen.length > failed)
      notify("success", `Shared ${chosen.length - failed} with ${shownId(task, kind)}`, "The next agent finds them in .agentry/files.");
    onChanged();
  }

  async function remove(file: SharedFile) {
    try {
      await api("DELETE", `/api/tasks/${encodeURIComponent(task)}/shared?name=${encodeURIComponent(file.name)}`);
      onChanged();
    } catch (failure) {
      notify("danger", `${file.name} is still shared`, (failure as Error).message);
    }
  }

  const columns: Column<SharedFile>[] = [
    { header: "File", className: "w-full max-w-0", render: (f) => <Mono className="truncate text-ink-primary">{f.name}</Mono> },
    { header: "Size", className: "w-24 text-right", render: (f) => <Mono className="text-ink-muted">{sizeOf(f.size)}</Mono> },
    {
      header: "From",
      className: "w-56 max-lg:hidden",
      render: (f) => <span className="truncate text-[12px] text-ink-secondary">{f.from ? `copied from ${f.from}` : f.by}</span>,
    },
    {
      header: "Added",
      className: "w-24 max-md:hidden",
      render: (f) => <span className="text-[12px] text-ink-muted">{timeAgo(f.at)}</span>,
    },
    {
      header: "",
      className: "w-28 text-right",
      render: (f) => (
        <span className="inline-flex items-center gap-3">
          <a
            href={downloadURL(task, f.name)}
            title="Download"
            className="text-ink-faint hover:text-ink-primary [&>svg]:size-4"
            onClick={(e) => e.stopPropagation()}
          >
            <Download />
          </a>
          {!readOnly && (
            <button
              type="button"
              title="Stop sharing it"
              onClick={(e) => (e.stopPropagation(), remove(f))}
              className="cursor-pointer text-ink-faint hover:text-danger [&>svg]:size-4"
            >
              <Trash2 />
            </button>
          )}
        </span>
      ),
    },
  ];

  const drop = (event: DragEvent) => {
    event.preventDefault();
    setOver(false);
    if (event.dataTransfer.files.length) send(event.dataTransfer.files);
  };

  return (
    <div
      className="flex h-full flex-col"
      onDragOver={(event) => {
        if (readOnly || !event.dataTransfer.types.includes("Files")) return;
        event.preventDefault();
        setOver(true);
      }}
      onDragLeave={(event) => !event.currentTarget.contains(event.relatedTarget as Node) && setOver(false)}
      onDrop={drop}
    >
      <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-divider px-6 py-3 text-[12px] text-ink-muted">
        <span>
          Shared with the task's agents, on whichever machine works on it: they're in <Mono>.agentry/files</Mono>.
        </span>
        {!readOnly && (
          <span className="ml-auto">
            <Button size="sm" icon={<FileUp />} disabled={sending > 0} onClick={() => picker.current?.click()}>
              {sending ? `Sharing ${sending}…` : "Share files"}
            </Button>
          </span>
        )}
        <input ref={picker} type="file" multiple hidden onChange={(e) => e.target.files && send(e.target.files)} />
      </div>
      <div
        className={cn("min-h-0 flex-1 scrollbar-thin overflow-y-auto", over && "outline-2 -outline-offset-2 outline-brand outline-dashed")}
      >
        {files.length ? (
          <ResourceTable columns={columns} rows={files} rowKey={(f) => f.name} />
        ) : (
          <EmptyState icon={<Paperclip />} title="No shared files">
            {readOnly
              ? "The task had none when it was archived."
              : "Drop files here to share them with the task's agents. What they leave in .agentry/files shows here too."}
          </EmptyState>
        )}
      </div>
    </div>
  );
}

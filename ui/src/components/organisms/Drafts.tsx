import { useState } from "react";
import { Sparkles } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Mono } from "@/components/molecules/Mono";
import { api } from "@/api/client";
import type { Draft, TaskDetail } from "@/api/types";
import { clockTime } from "@/lib/time";
import { useToast } from "./Toasts";

// Messages the Claude session in the Workspace drafted for the coordinator. Nothing reaches
// it until you send one, as it is or changed, because that starts work.

export function Drafts({ task, onChanged }: { task: TaskDetail; onChanged: () => void }) {
  if (task.drafts.length === 0) return null;
  return (
    <div className="flex flex-col gap-2">
      {task.drafts.map((draft) => (
        <DraftCard key={draft.id} task={task} draft={draft} onChanged={onChanged} />
      ))}
    </div>
  );
}

function DraftCard({ task, draft, onChanged }: { task: TaskDetail; draft: Draft; onChanged: () => void }) {
  const notify = useToast();
  const [text, setText] = useState(draft.text);
  const [busy, setBusy] = useState(false);
  const working = task.turnList?.find((t) => !t.ended)?.agent;
  const effect =
    task.status === "done"
      ? "Sending opens the task again."
      : working
        ? `Sending stops ${working}'s turn so the coordinator reads it now.`
        : "The coordinator reads it at once.";

  async function act(what: "send" | "discard") {
    setBusy(true);
    try {
      await api("POST", `/api/tasks/${encodeURIComponent(task.id)}/drafts/${draft.id}/${what}`, what === "send" ? { text } : undefined);
      notify("success", what === "send" ? "Sent to the coordinator" : "Draft discarded");
      onChanged();
    } catch (failure) {
      notify("danger", what === "send" ? "The draft wasn't sent" : "The draft wasn't discarded", (failure as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="flex flex-col gap-2 rounded-lg border border-info/50 bg-surface-1 px-3.5 py-3">
      <span className="flex items-center gap-1.5 text-[12px] font-medium text-info [&>svg]:size-3.5">
        <Sparkles />
        Claude drafted this for the coordinator
        <Mono className="ml-auto text-[11px] font-normal text-ink-faint">{clockTime(draft.at)}</Mono>
      </span>
      <textarea
        value={text}
        onChange={(event) => setText(event.target.value)}
        rows={Math.min(8, Math.max(2, text.split("\n").length))}
        aria-label="The draft, to change before you send it"
        className="resize-y rounded-md border border-divider bg-surface-0 px-3 py-2 text-[13px] leading-5 text-ink-primary outline-none focus:border-brand"
      />
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="primary" disabled={busy || !text.trim()} onClick={() => act("send")}>
          Send to the coordinator
        </Button>
        <Button size="sm" variant="ghost" disabled={busy} onClick={() => act("discard")}>
          Discard
        </Button>
        <span className="text-[11px] text-ink-muted">{effect}</span>
      </div>
    </section>
  );
}

import { ReactNode, useState } from "react";
import { Check, Pencil, X } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { IconButton } from "@/components/atoms/IconButton";
import { LimitSelect } from "@/components/molecules/LimitSelect";
import { Mono } from "@/components/molecules/Mono";
import { api } from "@/api/client";
import type { TaskSummary } from "@/api/types";
import { LimitChoice, LimitKind, SCALES, choiceOf, described, limitOf, sent } from "@/lib/limits";
import { cn } from "@/lib/cn";
import { useToast } from "./Toasts";

type LimitTask = Pick<TaskSummary, "id" | "status" | "budget" | "tokensUsed" | "maxTurns" | "turns" | "turnsSinceAnswer">;

interface LimitEditorProps {
  task: LimitTask;
  kind: LimitKind;
  compact?: boolean; // the strip under the header: what it shows and a pencil, then the editor in their place
  children?: ReactNode; // compact: what it shows until you edit
  onChanged: () => void;
}

// A task's budget or max turns, and changing it. Raising the limit a task is
// stopped at answers its stop, and it carries on. A done task's limits only
// show: the rules keep them as they were.
export function LimitEditor({ task, kind, compact, children, onChanged }: LimitEditorProps) {
  const scale = SCALES[kind];
  const current = kind === "budget" ? task.budget : task.maxTurns;
  const used = kind === "budget" ? task.tokensUsed : task.turnsSinceAnswer;
  const [editing, setEditing] = useState<LimitChoice | null>(null);
  const save = useSaveLimit(task.id, kind, () => {
    setEditing(null);
    onChanged();
  });

  if (editing === null) {
    const edit = () => setEditing(choiceOf(scale, current));
    const done = task.status === "done";
    if (compact && done) return <>{children}</>;
    if (compact) {
      return (
        <span className="flex w-full items-center gap-1">
          <span className="min-w-0 flex-1">{children}</span>
          <IconButton
            icon={<Pencil />}
            label={`Change the ${scale.name.toLowerCase()}`}
            onClick={edit}
            className="size-6 [&>svg]:size-3.5"
          />
        </span>
      );
    }
    return (
      <span className="flex flex-wrap items-center gap-2">
        <Mono>{kind === "budget" ? budgetLine(task.budget) : turnsLine(task)}</Mono>
        {!done && (
          <Button size="sm" variant="ghost" onClick={edit}>
            Change
          </Button>
        )}
      </span>
    );
  }

  const limit = limitOf(scale, editing);
  const stops = limit > 0 && limit <= used;
  const valid = !Number.isNaN(limit);
  return (
    <span className={cn("flex flex-col", compact ? "gap-1 py-1" : "gap-2")}>
      <span className={cn("flex items-start", compact ? "gap-1" : "flex-wrap gap-2")}>
        <LimitSelect scale={scale} choice={editing} onChange={setEditing} autoFocus />
        {compact && (
          <>
            <IconButton icon={<Check />} label="Save" disabled={!valid || save.busy} onClick={() => save.run(limit)} />
            <IconButton icon={<X />} label="Cancel" onClick={() => setEditing(null)} />
          </>
        )}
      </span>
      {stops && (
        <span className="text-[11px] text-warning">
          The task has used {scale.show(used)}
          {kind === "turns" ? " turns since your answer" : " tokens"}: it stops at its next check.
        </span>
      )}
      {!compact && (
        <span className="flex gap-2">
          <Button size="sm" variant="primary" disabled={!valid || save.busy} onClick={() => save.run(limit)}>
            Save
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setEditing(null)}>
            Cancel
          </Button>
        </span>
      )}
    </span>
  );
}

function budgetLine(budget: number): string {
  return budget > 0 ? `${SCALES.budget.show(budget)} tokens` : "no limit";
}

// "18 of 24 turns" since your last answer, or with no limit all its turns:
// "30 turns (no limit)".
export function turnsLine(task: LimitTask): string {
  return task.maxTurns > 0 ? `${task.turnsSinceAnswer} of ${task.maxTurns} turns` : `${task.turns} turns (no limit)`;
}

// What a saved limit replies: whether it answered the task's stop, and why
// not if the rules refused that answer.
interface LimitReply {
  answered: boolean;
  refused?: string;
}

// Sends a new limit, and calls onSaved once it is kept. It is kept though its
// answer to the task's stop was refused, as when you answered the stop
// elsewhere first. An error kept nothing, so what you chose stays.
export function useSaveLimit(taskId: string, kind: LimitKind, onSaved: () => void) {
  const notify = useToast();
  const [busy, setBusy] = useState(false);
  const scale = SCALES[kind];

  async function run(limit: number) {
    setBusy(true);
    const saved = `${scale.name} set to ${described(scale, limit)}`;
    try {
      const body = kind === "budget" ? { budget: sent(limit) } : { maxTurns: sent(limit) };
      const reply = await api<LimitReply>("PUT", `/api/tasks/${encodeURIComponent(taskId)}/${kind}`, body);
      if (reply.refused) notify("info", saved, `Its stop wasn't answered: ${reply.refused}`);
      else notify("success", saved, reply.answered ? "The task carries on." : undefined);
      onSaved();
    } catch (error) {
      notify("danger", "That didn't go through", (error as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return { run, busy };
}

import { ReactNode, useState } from "react";
import { Check, Pencil, X } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { IconButton } from "@/components/atoms/IconButton";
import { LimitSelect } from "@/components/molecules/LimitSelect";
import { Mono } from "@/components/molecules/Mono";
import { api, RequestError } from "@/api/client";
import type { TaskSummary } from "@/api/types";
import { LimitChoice, LimitKind, SCALES, choiceOf, described, limitOf, sent } from "@/lib/limits";
import { cn } from "@/lib/cn";
import { useToast } from "./Toasts";

type LimitTask = Pick<TaskSummary, "id" | "budget" | "tokensUsed" | "maxTurns" | "turnsSinceAnswer">;

interface LimitEditorProps {
  task: LimitTask;
  kind: LimitKind;
  compact?: boolean; // the strip under the header: what it shows and a pencil, then the editor in their place
  children?: ReactNode; // compact: what it shows until you edit
  onChanged: () => void;
}

// A task's budget or max turns, and changing it. Raising the limit a task is
// stopped at answers its stop, and it carries on.
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
        <Mono>{kind === "budget" ? budgetLine(task.budget) : turnsLine(task.turnsSinceAnswer, task.maxTurns)}</Mono>
        <Button size="sm" variant="ghost" onClick={edit}>
          Change
        </Button>
      </span>
    );
  }

  const limit = limitOf(scale, editing);
  const stops = limit > 0 && limit <= used;
  const valid = !Number.isNaN(limit);
  return (
    <span className={cn("flex flex-col", compact ? "gap-1 py-1" : "gap-2")}>
      <span className={cn("flex items-start", compact ? "gap-1" : "flex-wrap gap-2")}>
        <LimitSelect scale={scale} choice={editing} onChange={setEditing} />
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

// "18 of 24 turns", or "18 turns (no limit)": since your last answer.
export function turnsLine(since: number, maxTurns: number): string {
  return maxTurns > 0 ? `${since} of ${maxTurns} turns` : `${since} turns (no limit)`;
}

// Sends a new limit, and calls onSaved once it is kept. A 409 kept it too:
// only its answer to the task's stop was refused, as when you answered the
// stop elsewhere first. Any other error kept nothing, so what you chose stays.
export function useSaveLimit(taskId: string, kind: LimitKind, onSaved: () => void) {
  const notify = useToast();
  const [busy, setBusy] = useState(false);
  const scale = SCALES[kind];

  async function run(limit: number) {
    setBusy(true);
    const saved = `${scale.name} set to ${described(scale, limit)}`;
    try {
      const body = kind === "budget" ? { budget: sent(limit) } : { maxTurns: sent(limit) };
      const reply = await api<{ answered: boolean }>("PUT", `/api/tasks/${encodeURIComponent(taskId)}/${kind}`, body);
      notify("success", saved, reply.answered ? "The task carries on." : undefined);
      onSaved();
    } catch (error) {
      if (error instanceof RequestError && error.status === 409) {
        notify("info", saved, `Its stop wasn't answered: ${error.message}`);
        onSaved();
      } else {
        notify("danger", "That didn't go through", (error as Error).message);
      }
    } finally {
      setBusy(false);
    }
  }
  return { run, busy };
}

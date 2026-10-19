import { useEffect, useState } from "react";
import { Ban, Check, Gauge, MessageSquare, SquareTerminal } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Input, TextArea } from "@/components/atoms/Input";
import { LimitKind, SCALES } from "@/lib/limits";
import { CodeBlock } from "@/components/molecules/CodeBlock";
import { Panel } from "@/components/molecules/Panel";
import { api } from "@/api/client";
import type { Action, TaskDetail } from "@/api/types";
import { useSaveLimit } from "./LimitEditor";
import { useToast } from "./Toasts";

// What a task's agents are waiting on you for: actions the policy held, or
// an agent stuck on a prompt in its terminal. A question from the
// coordinator (or Agentry) isn't here: it is in the task's chat, and
// answered in the dock's Questions.

interface NeedsYouPanelProps {
  task: TaskDetail;
  onChanged: () => void;
  onOpenTerminal: () => void;
}

export function NeedsYouPanel({ task, onChanged, onOpenTerminal }: NeedsYouPanelProps) {
  const approvals = task.approvals ?? [];
  const waiting = task.status === "needs you" && approvals.length === 0 && !task.hasQuestion ? task.needsYou : "";
  if (approvals.length === 0 && !waiting) return null;

  return (
    <Panel tone="warning" title={<span className="text-warning">Needs you</span>}>
      <div className="flex flex-col gap-4">
        {approvals.map((action) => (
          <Approval key={action.id} taskId={task.id} action={action} onChanged={onChanged} />
        ))}
        {waiting && <p className="text-[13px] whitespace-pre-wrap text-ink-primary">{waiting}</p>}
        {waiting && (
          <div className="flex flex-wrap items-center gap-3 text-[12px] text-ink-muted">
            The agent is waiting on something in its terminal.
            <Button size="sm" icon={<SquareTerminal />} onClick={onOpenTerminal}>
              Open the terminal
            </Button>
          </div>
        )}
      </div>
    </Panel>
  );
}

export function Approval({
  taskId,
  action,
  onChanged,
}: {
  taskId: string;
  action: Pick<Action, "id" | "agent" | "tool" | "command" | "reason">;
  onChanged: () => void;
}) {
  const notify = useToast();
  const [busy, setBusy] = useState(false);

  async function decide(verdict: "approve" | "deny") {
    setBusy(true);
    try {
      await api("POST", `/api/tasks/${taskId}/actions/${encodeURIComponent(action.id)}/${verdict}`, { reason: "decided on the page" });
      notify("success", verdict === "approve" ? "Approved" : "Denied", action.command);
      onChanged();
    } catch (error) {
      notify("danger", "Couldn't record that", (error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <p className="text-[13px] text-ink-secondary">
        <span className="font-semibold text-ink-primary">{action.agent}</span> asks to run{" "}
        <span className="font-mono text-[12px]">{action.tool}</span>:
      </p>
      <CodeBlock text={action.command} className="max-h-60" />
      {action.reason && <p className="text-[12px] text-ink-muted">Rule: {action.reason}</p>}
      <div className="flex gap-2">
        <Button variant="primary" icon={<Check />} disabled={busy} onClick={() => decide("approve")}>
          Approve
        </Button>
        <Button variant="danger" icon={<Ban />} disabled={busy} onClick={() => decide("deny")}>
          Deny
        </Button>
      </div>
    </div>
  );
}

interface AnswerProps {
  taskId: string;
  onChanged: () => void;
  options?: string[]; // answers the coordinator offered: one click sends one
  prefill?: string; // an answer to check before sending, as from a question's page
}

export function Answer({ taskId, onChanged, options = [], prefill = "" }: AnswerProps) {
  const notify = useToast();
  const [answer, setAnswer] = useState(prefill);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (prefill) setAnswer(prefill);
  }, [prefill]);

  async function send(text = answer) {
    if (!text.trim()) return;
    setBusy(true);
    try {
      await api("POST", `/api/tasks/${taskId}/answer`, { answer: text });
      notify("success", "Answer sent to the coordinator");
      setAnswer("");
      onChanged();
    } catch (error) {
      notify("danger", "Couldn't send the answer", (error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-col gap-2">
      {options.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {options.map((option) => (
            <Button key={option} disabled={busy} onClick={() => send(option)}>
              {option}
            </Button>
          ))}
        </div>
      )}
      <TextArea
        value={answer}
        onChange={(event) => setAnswer(event.target.value)}
        onKeyDown={(event) => event.key === "Enter" && (event.metaKey || event.ctrlKey) && send()}
        placeholder="Your answer goes to the coordinator"
      />
      <div>
        <Button variant="primary" icon={<MessageSquare />} disabled={busy || !answer.trim()} onClick={() => send()}>
          Answer
        </Button>
      </div>
    </div>
  );
}

// A task stopped at its budget or its max turns: raise the limit, or make it
// unlimited, and the task goes on; or have the coordinator wrap up with what
// is done.
export function LimitStopChoice({
  taskId,
  kind,
  used,
  limit,
  onChanged,
}: {
  taskId: string;
  kind: LimitKind;
  used: number; // tokens, or turns since your answer
  limit: number;
  onChanged: () => void;
}) {
  const notify = useToast();
  const scale = SCALES[kind];
  const [raiseTo, setRaiseTo] = useState(scale.show(Math.max(limit * 2, used + limit)));
  const [busy, setBusy] = useState(false);
  const save = useSaveLimit(taskId, kind, onChanged);
  const thing = kind === "budget" ? "budget" : "turn limit";

  const raise = () => {
    const raised = scale.parse(raiseTo);
    if (Number.isNaN(raised) || raised <= used) {
      notify(
        "danger",
        "Raise it above what the task has used",
        `It has used ${scale.show(used)} ${kind === "budget" ? "tokens" : "turns"}.`,
      );
      return;
    }
    save.run(raised);
  };
  const wrapUp = async () => {
    setBusy(true);
    try {
      await api("POST", `/api/tasks/${taskId}/answer`, { answer: "Stop here: wrap the task up with what is done, and say what is left." });
      notify("success", "Asked the coordinator to wrap up");
      onChanged();
    } catch (error) {
      notify("danger", "That didn't go through", (error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input mono value={raiseTo} onChange={(e) => setRaiseTo(e.target.value)} className="w-28" aria-label={`New ${thing}`} />
      <Button variant="primary" icon={<Gauge />} disabled={busy || save.busy} onClick={raise}>
        Raise the {thing}
      </Button>
      <Button disabled={busy || save.busy} onClick={() => save.run(0)}>
        Make it unlimited
      </Button>
      <Button disabled={busy || save.busy} onClick={wrapUp}>
        Wrap up
      </Button>
    </div>
  );
}

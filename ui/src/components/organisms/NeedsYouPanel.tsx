import { useEffect, useState } from "react";
import { Ban, Check, Gauge, MessageSquare, SquareTerminal } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Input, TextArea } from "@/components/atoms/Input";
import { tokens, parseTokens } from "@/lib/tokens";
import { CodeBlock } from "@/components/molecules/CodeBlock";
import { Panel } from "@/components/molecules/Panel";
import { api } from "@/api/client";
import type { Action, TaskDetail } from "@/api/types";
import { useToast } from "./Toasts";
import { QuestionPage } from "@/components/molecules/QuestionPage";

// What a task is waiting on you for: actions the policy held, a question
// from the coordinator, or an agent stuck on a prompt in its terminal.

interface NeedsYouPanelProps {
  task: TaskDetail;
  onChanged: () => void;
  onOpenTerminal: () => void;
}

export function NeedsYouPanel({ task, onChanged, onOpenTerminal }: NeedsYouPanelProps) {
  const [fromPage, setFromPage] = useState("");
  const approvals = task.approvals ?? [];
  const question = task.status === "needs you" && approvals.length === 0 ? task.needsYou : "";
  if (approvals.length === 0 && !question) return null;

  const trail = task.trail ?? [];
  const askedByCoordinator = trail.length > 0 && trail[trail.length - 1].to === "you";
  return (
    <Panel tone="warning" title={<span className="text-warning">Needs you</span>}>
      <div className="flex flex-col gap-4">
        {approvals.map((action) => (
          <Approval key={action.id} taskId={task.id} action={action} onChanged={onChanged} />
        ))}
        {question && <p className="text-[13px] whitespace-pre-wrap text-ink-primary">{question}</p>}
        {question && task.overBudget && <BudgetChoice task={task} onChanged={onChanged} />}
        {question && task.page && <QuestionPage page={task.page} onAnswer={setFromPage} />}
        {question && askedByCoordinator && <Answer taskId={task.id} onChanged={onChanged} options={task.options} prefill={fromPage} />}
        {question && !askedByCoordinator && (
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

// A task stopped at its budget: raise it and the task goes on, or have the
// coordinator wrap up with what is done.
export function BudgetChoice({ task, onChanged }: { task: { id: string; budget: number; tokensUsed: number }; onChanged: () => void }) {
  const notify = useToast();
  const [raiseTo, setRaiseTo] = useState(tokens(Math.max(task.budget * 2, task.tokensUsed + task.budget)));
  const [busy, setBusy] = useState(false);

  async function run(work: () => Promise<unknown>, done: string) {
    setBusy(true);
    try {
      await work();
      notify("success", done);
      onChanged();
    } catch (error) {
      notify("danger", "That didn't go through", (error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const raise = () => {
    const budget = parseTokens(raiseTo);
    if (Number.isNaN(budget) || budget <= task.tokensUsed) {
      notify("danger", "Raise it above what the task has used", `It has used ${tokens(task.tokensUsed)} tokens.`);
      return;
    }
    run(() => api("PUT", `/api/tasks/${task.id}/budget`, { budget }), `Budget raised to ${tokens(budget)}`);
  };
  const wrapUp = () =>
    run(
      () => api("POST", `/api/tasks/${task.id}/answer`, { answer: "Stop here: wrap the task up with what is done, and say what is left." }),
      "Asked the coordinator to wrap up",
    );

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input mono value={raiseTo} onChange={(e) => setRaiseTo(e.target.value)} className="w-28" aria-label="New budget" />
      <Button variant="primary" icon={<Gauge />} disabled={busy} onClick={raise}>
        Raise the budget
      </Button>
      <Button disabled={busy} onClick={wrapUp}>
        Wrap up
      </Button>
    </div>
  );
}

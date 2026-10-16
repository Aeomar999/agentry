import { useState } from "react";
import { Link } from "react-router-dom";
import { CheckCheck, Gauge, MessageCircleQuestionMark, ShieldAlert, SquareTerminal } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { AgentName } from "@/components/molecules/AgentName";
import { EmptyState } from "@/components/molecules/EmptyState";
import { QuestionPage } from "@/components/molecules/QuestionPage";
import type { Question } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { cn } from "@/lib/cn";
import { timeAgo } from "@/lib/time";
import { Answer, Approval, BudgetChoice } from "./NeedsYouPanel";

// Everything waiting for you, from every task, in the dock: the list on the
// left, oldest first, and the one you pick on the right with what led to it
// and the way to answer: approve or deny, pick an option, write an answer,
// or use the page the coordinator made to ask.

const KINDS: Record<Question["kind"], { label: string; icon: React.ReactNode }> = {
  approval: { label: "Approve", icon: <ShieldAlert /> },
  question: { label: "Question", icon: <MessageCircleQuestionMark /> },
  budget: { label: "Budget", icon: <Gauge /> },
  waiting: { label: "Waiting", icon: <SquareTerminal /> },
};

export function QuestionsPanel({ onOpenTerminal }: { onOpenTerminal: (taskId: string) => void }) {
  const { data, reload } = useResource<Question[]>("/api/questions");
  const questions = data ?? [];
  const [picked, setPicked] = useState("");
  const current = questions.find((q) => q.id === picked) ?? questions[0];

  if (data && questions.length === 0) {
    return (
      <EmptyState icon={<CheckCheck />} title="Nothing waits for you">
        Approvals and questions from every task show here.
      </EmptyState>
    );
  }
  return (
    <div className="flex h-full min-h-0">
      <ul className="w-72 shrink-0 scrollbar-thin overflow-y-auto border-r border-divider">
        {questions.map((q) => (
          <li key={q.id}>
            <button
              type="button"
              onClick={() => setPicked(q.id)}
              className={cn(
                "flex w-full cursor-pointer flex-col gap-1 border-b border-divider px-3 py-2 text-left [&_svg]:size-3.5",
                q.id === current?.id ? "bg-surface-3" : "hover:bg-surface-2",
              )}
            >
              <span className="flex items-center gap-2 text-[11px] text-ink-muted">
                <span className="text-warning">{KINDS[q.kind].icon}</span>
                {KINDS[q.kind].label}
                <span className="font-mono">{q.task}</span>
                <span className="ml-auto font-mono text-ink-faint">{timeAgo(q.at)}</span>
              </span>
              <span className="line-clamp-2 text-[12px] text-ink-primary">{q.text}</span>
              <AgentName name={q.agent} className="text-[11px] text-ink-muted" />
            </button>
          </li>
        ))}
      </ul>
      {current && <QuestionDetail key={current.id} question={current} onChanged={reload} onOpenTerminal={onOpenTerminal} />}
    </div>
  );
}

function QuestionDetail({
  question: q,
  onChanged,
  onOpenTerminal,
}: {
  question: Question;
  onChanged: () => void;
  onOpenTerminal: (taskId: string) => void;
}) {
  const [fromPage, setFromPage] = useState("");
  return (
    <div className="flex min-w-0 flex-1 scrollbar-thin flex-col gap-3 overflow-y-auto p-4">
      <div className="flex flex-wrap items-center gap-2 text-[12px] text-ink-muted">
        <Link to={`/tasks/${q.task}`} className="font-mono text-ink-secondary hover:text-ink-primary">
          {q.task}
        </Link>
        <span className="truncate text-ink-primary">{q.taskTitle}</span>
      </div>
      {q.context && (
        <p className="line-clamp-4 border-l-2 border-divider pl-3 text-[12px] whitespace-pre-wrap text-ink-muted">{q.context}</p>
      )}

      {q.kind === "approval" && (
        <Approval
          taskId={q.task}
          action={{ id: q.action!, agent: q.agent, tool: q.tool!, command: q.text, reason: q.reason! }}
          onChanged={onChanged}
        />
      )}
      {(q.kind === "question" || q.kind === "budget") && <p className="text-[13px] whitespace-pre-wrap text-ink-primary">{q.text}</p>}
      {q.page && <QuestionPage page={q.page} onAnswer={setFromPage} />}
      {fromPage && <p className="text-[12px] text-ink-muted">The page filled in your answer below. Check it, then send it.</p>}
      {q.kind === "budget" && <BudgetChoice task={{ id: q.task, budget: q.budget!, tokensUsed: q.tokensUsed! }} onChanged={onChanged} />}
      {(q.kind === "question" || q.kind === "budget") && (
        <Answer taskId={q.task} onChanged={onChanged} options={q.options} prefill={fromPage} />
      )}
      {q.kind === "waiting" && (
        <>
          <p className="text-[13px] whitespace-pre-wrap text-ink-primary">{q.text}</p>
          <div>
            <Button icon={<SquareTerminal />} onClick={() => onOpenTerminal(q.task)}>
              Open {q.agent}'s terminal
            </Button>
          </div>
        </>
      )}
    </div>
  );
}

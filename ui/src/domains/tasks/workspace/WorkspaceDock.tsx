import { ReactNode, useState } from "react";
import { Inbox, MessagesSquare, Sparkles, SquareTerminal, Users, X } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";
import { Kbd } from "@/components/atoms/Kbd";
import { AgentName } from "@/components/molecules/AgentName";
import { EmptyState } from "@/components/molecules/EmptyState";
import { ResizeHandle } from "@/components/molecules/ResizeHandle";
import { helperTerminalURL, terminalURL } from "@/api/client";
import type { TaskDetail } from "@/api/types";
import { cn } from "@/lib/cn";
import { modKey } from "@/lib/shortcuts";
import { clockTime } from "@/lib/time";
import { tokens } from "@/lib/tokens";
import { TerminalView } from "@/components/organisms/TerminalView";
import { CoordinatorChat } from "@/domains/tasks/CoordinatorChat";
import { QuestionsPanel } from "@/components/organisms/QuestionsPanel";

// The Workspace page's dock: your chat with the task's coordinator, which
// can put the task back to work; your own Claude, in the worktree, for your
// questions, which sends the coordinator a change when you ask it to; the agent's terminal; the agents that have worked here, turn by
// turn; and, while the coordinator waits for you, Questions.

interface WorkspaceDockProps {
  task: TaskDetail;
  height: number;
  onResize: (height: number) => void;
  onClose: () => void;
  onChanged: () => void; // something you sent: read the task again
}

type Tab = "coordinator" | "claude" | "terminal" | "agents" | "questions";

export function WorkspaceDock({ task, height, onResize, onClose, onChanged }: WorkspaceDockProps) {
  const working = task.status !== "done" && task.currentAgent;
  const [tab, setTab] = useState<Tab>(working ? "terminal" : "coordinator");
  const [question, setQuestion] = useState({ id: "", asked: 0 });
  const turns = [...(task.turnList ?? [])].reverse();
  const theme = document.documentElement.classList.contains("light") ? "light" : "dark";
  const openQuestion = (id: string) => {
    setQuestion((last) => ({ id, asked: last.asked + 1 }));
    setTab("questions");
  };
  const tabs: [Tab, string, ReactNode][] = [
    ["coordinator", task.hasQuestion ? "Coordinator · asks you" : "Coordinator", <MessagesSquare key="c" />],
    ["claude", "Claude", <Sparkles key="y" />],
    ["terminal", "Terminal", <SquareTerminal key="t" />],
    ["agents", `Agents ${turns.length}`, <Users key="a" />],
    ...(task.hasQuestion || tab === "questions" ? [["questions", "Questions", <Inbox key="q" />] as [Tab, string, ReactNode]] : []),
  ];
  return (
    <section
      aria-label="Terminal"
      className="relative flex shrink-0 flex-col border-t border-divider bg-surface-1"
      style={{ height, maxHeight: "100%" }}
    >
      <ResizeHandle height={height} onResize={onResize} />
      <div className="flex h-9 shrink-0 items-center gap-0.5 border-b border-divider px-2">
        {tabs.map(([id, label, icon]) => (
          <button
            key={id}
            type="button"
            onClick={() => setTab(id)}
            className={cn(
              "inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-sm px-2.5 font-mono text-[12px] [&>svg]:size-3.5",
              tab === id ? "bg-surface-3 text-ink-primary" : "text-ink-muted hover:bg-surface-2 hover:text-ink-primary",
            )}
          >
            {icon}
            {label}
          </button>
        ))}
        <span className="ml-auto flex items-center gap-1.5 text-[11px] text-ink-faint">
          <Kbd>{modKey} J</Kbd>
          <IconButton icon={<X />} label="Close the dock" onClick={onClose} className="size-7" />
        </span>
      </div>
      <div className="min-h-0 flex-1">
        {tab === "coordinator" && <CoordinatorChat task={task} onSent={onChanged} onOpenQuestion={openQuestion} />}
        {tab === "questions" && <QuestionsPanel onOpenTerminal={() => setTab("terminal")} focus={question} />}
        {tab === "claude" && (
          <TerminalView key={`claude-${theme}`} url={helperTerminalURL(task.id, theme)} title="claude" takeControlOnConnect />
        )}
        {tab === "terminal" &&
          (working ? (
            <TerminalView key={`${task.id}-${task.turns}`} url={terminalURL(task.id)} title={task.currentAgent} />
          ) : (
            <EmptyState icon={<SquareTerminal />} title="No agent is working here right now" />
          ))}
        {tab === "agents" && (
          <ol className="h-full scrollbar-thin overflow-y-auto">
            {turns.map((turn) => {
              const handoff = task.trail?.find((step) => step.from === turn.agent && new Date(step.at) >= new Date(turn.startedAt));
              return (
                <li key={turn.id} className="flex flex-col gap-0.5 border-b border-divider px-4 py-2">
                  <span className="flex items-center gap-3 text-[12px]">
                    <AgentName name={turn.agent} className="font-semibold text-ink-primary" />
                    <span className="font-mono text-ink-faint">{clockTime(turn.startedAt)}</span>
                    {turn.tokens > 0 && <span className="font-mono text-ink-faint">{tokens(turn.tokens)} tokens</span>}
                    <span className="ml-auto text-[11px] text-ink-muted">
                      {turn.ended ? (turn.issue ? "issue" : "ended") : turn.limited ? "waiting for the usage limit" : "working now"}
                    </span>
                  </span>
                  {turn.issue && <span className="text-[12px] text-danger">{turn.issue}</span>}
                  {handoff && <span className="line-clamp-2 text-[12px] text-ink-muted">{handoff.text}</span>}
                </li>
              );
            })}
            {turns.length === 0 && <li className="px-4 py-3 text-[12px] text-ink-muted">No agent has worked here yet.</li>}
          </ol>
        )}
      </div>
    </section>
  );
}

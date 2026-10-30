import { ReactNode } from "react";
import { ArrowRight, ChevronRight } from "lucide-react";
import type { Step, Turn } from "@/api/types";
import { Badge } from "@/components/atoms/Badge";
import { AgentName } from "@/components/molecules/AgentName";
import { Avatar, isNeutral } from "@/components/molecules/Avatar";
import { InlineMarkdown } from "@/components/molecules/Markdown";
import { Mono } from "@/components/molecules/Mono";
import { EntryText, TaskLinks } from "@/domains/tasks/EntryText";
import { clockMinutes, duration } from "@/lib/time";
import { tokens } from "@/lib/tokens";
import { cn } from "@/lib/cn";
import { useAgentColor } from "@/live/agent-colors";

// The handoff trail: every message between you, the coordinator and the
// agents, newest first, as a vertical line of steps. Each step is a card
// whose header says who passed it to whom, and stays on screen while you
// read a long brief. Its footer opens the activity of the turn it led to:
// what the agent it went to did with it. What is said to you leads
// nowhere, so it opens the turn that said it.
//
// Like EntryText, it must not import lib/monaco.

interface HandoffTimelineProps {
  steps: Step[];
  turns: Turn[];
  coordinator: string; // the task's, whom reports go to
  links: TaskLinks; // where the task's text links to
  actionsIn: (turn: string) => number;
  working: string; // the agent at work now, "" when none
  onOpen: (turn: string) => void;
  onWatch: () => void; // the working agent's terminal
}

const BRIEF_LINES = 8;
const TEXT_LINES = 10; // reports, issues and your messages

export function HandoffTimeline({ steps, turns, coordinator, links, actionsIn, working, onOpen, onWatch }: HandoffTimelineProps) {
  const colorOf = useAgentColor(); // each step's dot in the colour of who spoke
  const turnOf = new Map(turns.map((t) => [t.id, t]));
  return (
    <ol className="relative flex flex-col gap-3 before:absolute before:top-3 before:bottom-3 before:left-[5px] before:w-px before:bg-divider">
      {working && (
        <li className="relative pl-6">
          <span className="absolute top-2 left-0 size-[11px] animate-pulse rounded-full border-2 border-surface-0 bg-success" aria-hidden />
          <button
            type="button"
            onClick={onWatch}
            aria-label={`${working} is working now. Watch the terminal`}
            className="flex cursor-pointer items-center gap-1.5 py-1 text-[12px] text-success-ink hover:underline"
          >
            <AgentName name={working} className="font-semibold" />
            is working now · watch the terminal
          </button>
        </li>
      )}
      {[...steps].reverse().map((step, i) => {
        const turn = turnOf.get(step.turn);
        return (
          <li key={steps.length - i} className="relative pl-6">
            <span
              className={cn(
                "absolute top-3.5 left-0 size-[11px] rounded-full border-2 border-surface-0",
                isNeutral(step.from) && "bg-ink-muted",
              )}
              style={isNeutral(step.from) ? undefined : { background: colorOf(step.from) }}
              aria-hidden
            />
            <StepCard step={step} kind={kindOf(step, coordinator)} links={links}>
              {turn && <TurnFooter turn={turn} actions={actionsIn(turn.id)} ledTo={step.to !== "you"} onOpen={onOpen} />}
            </StepCard>
          </li>
        );
      })}
    </ol>
  );
}

// The word a step's Show all names it by: what you say or are told is a
// message, what comes back to the coordinator a report, and the rest a brief.
function kindOf(step: Step, coordinator: string): string {
  if ([step.from, step.to].some(isNeutral)) return "message";
  return step.to === coordinator ? "report" : "brief";
}

// A step as a card: its header sticks to the top of the tab while the card
// is on screen, a fixed 40px stuck or not, so the next card's header pushes
// it out rather than covering it. On a phone the header stays one line, the
// names cut short; the card's label has them whole.
function StepCard({ step, kind, links, children }: { step: Step; kind: string; links: TaskLinks; children: ReactNode }) {
  const time = clockMinutes(step.at);
  const { title, body } = titled(step.text);
  return (
    <article aria-label={`${step.from} to ${step.to}, ${time}`} className="rounded-md border border-divider bg-surface-0">
      <div className="sticky top-0 z-10 flex h-10 items-center gap-2 rounded-t-md border-b border-divider bg-surface-0 px-3 text-[12px] whitespace-nowrap">
        <Author name={step.from} />
        <ArrowRight className="size-3 shrink-0 text-ink-muted" aria-hidden />
        <span className="sr-only"> to </span>
        <Author name={step.to} />
        <Mono className="ml-auto shrink-0 text-[11px] text-ink-muted">{time}</Mono>
      </div>
      {(title || body) && (
        <div className="px-3 pt-2 pb-3">
          {title && (
            <h3 className="mb-1 line-clamp-2 text-[14px] font-semibold text-ink-primary">
              <InlineMarkdown text={title} marks {...links} />
            </h3>
          )}
          {body && <EntryText text={body} links={links} lines={kind === "brief" ? BRIEF_LINES : TEXT_LINES} kind={kind} />}
        </div>
      )}
      {children}
    </article>
  );
}

function Author({ name }: { name: string }) {
  return (
    <span className="flex min-w-0 items-center gap-1.5">
      <Avatar name={name} size={18} />
      <span className="max-w-24 truncate font-mono font-semibold text-ink-primary md:max-w-30">{name}</span>
    </span>
  );
}

// titled takes a brief's opening heading out as the card's title, so it
// isn't said twice. The title keeps its inline Markdown, for the card to draw.
function titled(text: string): { title: string; body: string } {
  const heading = /^\s*#{1,6}[ \t]+(.+?)[ \t#]*(?:\n|$)/.exec(text);
  if (!heading) return { title: "", body: text.trim() };
  return { title: heading[1], body: text.slice(heading[0].length).trim() };
}

// The footer opens the turn a step belongs to, in one line. On a phone it
// keeps the turn, its agent and its badge; the agent's name is the one
// thing that gives way, so the badge and Activity › always show whole.
function TurnFooter({ turn, actions, ledTo, onOpen }: { turn: Turn; actions: number; ledTo: boolean; onOpen: (turn: string) => void }) {
  const lead = ledTo ? "Led to" : "From";
  const status = turn.issue ? "issue" : turn.ended ? "" : turn.limited ? "usage limit" : "running";
  return (
    <button
      type="button"
      onClick={() => onOpen(turn.id)}
      aria-label={`${lead} ${turn.id}, ${turn.agent}${status && `, ${status}`}. Open its activity`}
      className="group flex h-11 w-full cursor-pointer items-center rounded-b-md border-t border-divider px-3 text-left text-[11.5px] whitespace-nowrap text-ink-muted transition-colors duration-(--dur-fast) hover:bg-surface-1"
    >
      <span className="shrink-0">{lead}&nbsp;</span>
      <Mono className="shrink-0 text-[11px]">{turn.id}</Mono>
      <Dot />
      <Mono className="min-w-0 shrink truncate text-[11px] max-md:max-w-18">{turn.agent}</Mono>
      {status && (
        <>
          <Dot />
          <Badge variant={turn.issue ? "danger" : turn.limited ? "warning" : "info"} className="shrink-0">
            {status}
          </Badge>
        </>
      )}
      {turn.ended && (
        <Fact>
          <Mono className="text-[11px]">{duration(turn.startedAt, turn.lastEventAt)}</Mono>
        </Fact>
      )}
      <Fact>
        {actions} {actions === 1 ? "action" : "actions"}
      </Fact>
      {turn.tokens > 0 && (
        <Fact>
          <Mono className="text-[11px]">{tokens(turn.tokens)} tokens</Mono>
        </Fact>
      )}
      <span className="ml-auto inline-flex shrink-0 items-center gap-0.5 pl-3 group-hover:text-ink-primary">
        Activity
        <ChevronRight className="size-3.5" aria-hidden />
      </span>
    </button>
  );
}

function Dot() {
  return <span className="shrink-0 px-1.5">·</span>;
}

// Fact is one of the turn's counts, which a phone has no room for.
function Fact({ children }: { children: ReactNode }) {
  return (
    <span className="hidden shrink-0 md:inline">
      <Dot />
      {children}
    </span>
  );
}

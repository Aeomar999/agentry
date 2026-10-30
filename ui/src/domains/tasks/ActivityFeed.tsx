import { ReactNode, useId, useLayoutEffect, useRef, useState } from "react";
import { ArrowRight, ArrowUp, CircleAlert, CircleCheck, Hourglass, MessageSquare, TriangleAlert, Wrench, X } from "lucide-react";
import type { Agent } from "@/api/types";
import { Badge } from "@/components/atoms/Badge";
import { Button } from "@/components/atoms/Button";
import { AgentName } from "@/components/molecules/AgentName";
import { AuthorBand, authorName } from "@/components/molecules/AuthorBand";
import { Avatar } from "@/components/molecules/Avatar";
import { CodeBox } from "@/components/molecules/CodeBox";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Fold } from "@/components/molecules/Fold";
import { Mono } from "@/components/molecules/Mono";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { ActionBadge } from "@/components/molecules/StatusBadges";
import { EntryText, TaskLinks } from "@/domains/tasks/EntryText";
import { ActivityItem, ActivityTone, Section, Show, inTurn, sections, shown } from "@/domains/tasks/activity";
import { cn } from "@/lib/cn";
import { clockMinutes, duration } from "@/lib/time";
import { tokens } from "@/lib/tokens";
import { LOCAL } from "@/lib/machines";
import { keepFocusInView } from "@/lib/focus-in-view";
import { useNow } from "@/live/use-now";

// Everything that happened on a task, newest at the top and the oldest at
// the bottom, in runs by one author: each run under a band that names who
// wrote it (a turn's agent, you, or Agentry), stuck to the top while you
// read its rows. Down the right runs the time: when each row happened, how
// long it took, and a pulsing mark on whatever is still going. Narrowed to
// one turn, it shows that turn's work.
//
// Like EntryText, it must not import lib/monaco.

export interface Now {
  label: string; // what is going on right now, as "dev is working"
  tone: "success" | "warning";
}

interface ActivityFeedProps {
  items: ActivityItem[];
  agents: Agent[]; // the roster, for each author's team and role
  links: TaskLinks; // where the task's text links to
  turn: string; // "" for every turn
  show: Show;
  row: string; // the key of the row you came to see, picked out; "" for none
  now: Now | null; // null when nothing is happening
  foot?: ReactNode; // under the oldest row: a way to read earlier ones; none with an error
  onTurn: (turn: string) => void;
  onShow: (show: Show) => void;
  loading?: boolean; // the task, or its newest events, aren't here yet
  error?: string; // it couldn't be loaded
  onRetry?: () => void;
}

const NODE: Record<ActivityTone, string> = {
  default: "bg-ink-faint",
  info: "bg-info",
  success: "bg-success",
  warning: "bg-warning",
  danger: "bg-danger",
};

// The rail: time only on a phone, then room for the duration too.
const COLUMNS = "grid grid-cols-[minmax(0,1fr)_56px] sm:grid-cols-[minmax(0,1fr)_104px] md:grid-cols-[minmax(0,1fr)_120px]";

// How many lines each kind of text shows before it folds.
const NOTE_LINES = 4;
const TEXT_LINES = 10; // briefs, reports, questions, issues, your messages and the rest

export function ActivityFeed(props: ActivityFeedProps) {
  const { items, agents, links, turn, show, row, now, foot, onTurn, onShow, loading, error, onRetry } = props;
  const rows = shown(turn ? inTurn(items, turn) : items, show);
  const runs = sections(rows, show);
  const open = turn ? items.find((item) => item.kind === "turn" && item.turn === turn) : undefined;
  const starting = Boolean(runs[0]?.turn?.live && !runs[0].continued && !runs[0].rows.length);
  const live = rows.some((row) => row.live) || Boolean(now);
  const clock = useNow(live);
  const agentOf = (name: string) => agents.find((agent) => agent.name === name);

  // A new view starts at the top, where the newest rows are, or with the
  // row you came to see in the middle. Once you have scrolled down, a
  // button takes you back up.
  const scroller = useRef<HTMLDivElement>(null);
  const heading = useId();
  const [away, setAway] = useState(false);
  const view = `${turn}/${show}/${row}`;
  useLayoutEffect(() => {
    const box = scroller.current;
    if (!box) return;
    const picked = row ? box.querySelector<HTMLElement>(`[data-row="${row}"]`) : null;
    if (picked) picked.scrollIntoView({ block: "center" });
    else box.scrollTop = 0;
    setAway(box.scrollTop > 200);
  }, [view]);
  const onScroll = () => setAway(scroller.current!.scrollTop > 200);
  const toTop = () => scroller.current?.scrollTo({ top: 0 });
  // Try again takes its alert away, so focus goes to the feed first: it
  // stays there through loading, and Tab reaches the rows, or the alert's
  // button again if the read fails again.
  const retry =
    onRetry &&
    (() => {
      scroller.current?.focus();
      onRetry();
    });

  return (
    <div className="flex h-full min-h-0 flex-col">
      <h2 id={heading} className="sr-only">
        Activity
      </h2>
      <div className="flex h-11 shrink-0 scrollbar-thin items-center gap-3 overflow-x-auto border-b border-divider px-4 md:px-6">
        <SegmentedControl
          active={show}
          onChange={(id) => onShow(id as Show)}
          segments={[
            { id: "all", label: "Everything" },
            { id: "actions", label: "Actions" },
            { id: "messages", label: "Messages" },
          ]}
        />
        {turn && (
          <span className="inline-flex h-7 items-center gap-2 rounded-md border border-divider-2 bg-surface-2 pr-1 pl-2.5 text-[12px]">
            <Mono className="text-ink-muted">{turn}</Mono>
            {open?.turnInfo && <AgentName name={open.turnInfo.agent} className="font-semibold text-ink-primary" />}
            <button
              type="button"
              onClick={() => onTurn("")}
              aria-label="Show every turn"
              className="inline-flex size-5 cursor-pointer items-center justify-center rounded-sm text-ink-muted hover:bg-surface-3 hover:text-ink-primary [&>svg]:size-3.5"
            >
              <X />
            </button>
          </span>
        )}
        {!loading && !error && (
          <Mono className="ml-auto text-[11px] text-ink-muted">
            {rows.length} {rows.length === 1 ? "row" : "rows"}
          </Mono>
        )}
      </div>

      {/* The scroller's focus line is drawn again on this box's ::after, above the stuck band
          (z-10) that would cover its top and sides; this box doesn't scroll, so the line stays put. */}
      <div className="relative min-h-0 flex-1 after:pointer-events-none after:absolute after:inset-0 after:z-20 after:border after:border-transparent after:content-[''] has-[>[role=region]:focus-visible]:after:border-brand">
        <div
          ref={scroller}
          onScroll={onScroll}
          onFocus={keepFocusInView}
          // A Tab stop, so the keyboard can scroll a turn whose rows hold nothing focusable; named,
          // as anything focusable must be. One region for the feed, none per run.
          tabIndex={0}
          role="region"
          aria-labelledby={heading}
          aria-busy={loading || undefined}
          // Focus and the row you came to see stop under the stuck band, and on a phone above Newest.
          // Positioned, so screen-reader-only text (absolute) is placed in it and scrolls with it, not the page.
          // Its own focus line (the global rule, which no utility can remove) sits inside its edge, under the
          // line its wrapper draws, so the two are one.
          className="relative h-full scroll-pt-11 scrollbar-thin overflow-y-auto focus-visible:-outline-offset-1 max-md:scroll-pb-12"
        >
          {error ? (
            <LoadError error={error} onRetry={retry} />
          ) : loading ? (
            <Loading />
          ) : rows.length === 0 && !now ? (
            <Empty show={show} />
          ) : (
            <ol className="pb-2">
              {now && !starting && !(turn && open && !open.live) && <NowRow now={now} />}
              {runs.map((section, n) => (
                <SectionItem
                  key={section.key}
                  section={section}
                  agent={agentOf(section.by)}
                  links={links}
                  row={row}
                  clock={clock}
                  onTurn={onTurn}
                  narrowed={Boolean(turn)}
                  starting={starting && n === 0}
                />
              ))}
            </ol>
          )}
          {!error && foot}
        </div>
        {away && (
          <button
            type="button"
            onClick={toTop}
            aria-label="Back to the newest rows"
            // 32px tall, with a 44px hit area.
            className="absolute right-4 bottom-4 inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md border border-divider-2 bg-surface-2 px-3 text-[12px] font-medium text-ink-primary after:absolute after:-inset-1.5 after:content-[''] hover:bg-surface-3 [&>svg]:size-3.5"
          >
            <ArrowUp aria-hidden />
            Newest
          </button>
        )}
      </div>
    </div>
  );
}

interface SectionProps {
  section: Section;
  agent?: Agent; // the author's roster entry; none for you, Agentry, or an agent since removed
  links: TaskLinks;
  row: string;
  clock: number;
  onTurn: (turn: string) => void;
  narrowed: boolean;
  starting: boolean; // a turn that has just started: no rows yet
}

// The row you came to see: a tinted surface with a hairline all round, flat
// like every selection, and the text left as it is so it reads the same.
// Solid, not see-through, so a band still covers what scrolls under it.
const PICKED =
  "bg-[color-mix(in_srgb,var(--color-info)_14%,var(--color-surface-0))] outline outline-1 -outline-offset-1 outline-info/60 hover:bg-[color-mix(in_srgb,var(--color-info)_14%,var(--color-surface-0))]";

// A run of rows by one author, under its band. The band sticks inside the
// run's own item, so the next run's band pushes it out rather than
// covering it.
function SectionItem({ section, agent, links, row, clock, onTurn, narrowed, starting }: SectionProps) {
  const band = section.turn;
  return (
    <li className="mt-2 first:mt-0">
      <div
        data-row={band && !section.continued ? band.key : undefined}
        aria-current={(band && !section.continued && band.key === row) || undefined}
        className={cn(
          COLUMNS,
          "sticky top-0 z-10 h-11 border-y border-divider bg-surface-1",
          band && !section.continued && band.key === row && PICKED,
        )}
      >
        {band ? (
          <TurnBand section={section} agent={agent} clock={clock} onTurn={onTurn} narrowed={narrowed} />
        ) : (
          <h3 aria-label={authorName(section.by, agent)} className="flex h-11 min-w-0 items-center px-4 md:px-6">
            <AuthorBand name={section.by} agent={agent} />
          </h3>
        )}
        <Rail item={band ?? section.rows[section.rows.length - 1]} clock={clock} band />
      </div>
      {(starting || section.rows.length > 0) && (
        <ol>
          {starting && <NowRow now={{ label: `${section.by} is starting…`, tone: "success" }} />}
          {section.rows.map((item) => (
            <Row key={item.key} item={item} links={links} picked={item.key === row} clock={clock} onTurn={onTurn} narrowed={narrowed} />
          ))}
        </ol>
      )}
    </li>
  );
}

// A turn's band is the turn's heading and the button that narrows the
// feed to it. On a phone it keeps the chip, the name, the status and the
// duration; its name says the rest.
function TurnBand({ section, agent, clock, onTurn, narrowed }: Omit<SectionProps, "links" | "row" | "starting">) {
  const item = section.turn!;
  const turn = item.turnInfo!;
  const status = turn.issue ? "issue" : turn.stopped ? "stopped by you" : turn.ended ? "ended" : turn.limited ? "usage limit" : "running";
  const variant = turn.issue ? "danger" : turn.stopped || (turn.limited && !turn.ended) ? "warning" : turn.ended ? "default" : "info";
  const remote = turn.worker && turn.worker !== LOCAL ? turn.worker : "";
  const name = [
    authorName(section.by, agent),
    item.turn,
    section.continued && "continued",
    remote && `on ${remote}`,
    turn.issue ? `issue: ${turn.issue}` : status,
  ]
    .filter(Boolean)
    .join(", ");
  return (
    <h3 className="flex min-w-0">
      <button
        type="button"
        disabled={narrowed}
        onClick={() => onTurn(item.turn)}
        aria-label={narrowed ? name : `${name}. Show only this turn`}
        className="flex h-11 min-w-0 flex-1 cursor-pointer items-center px-4 text-left text-[12px] disabled:cursor-default md:px-6"
      >
        <AuthorBand name={section.by} agent={agent}>
          <Mono className="hidden shrink-0 text-[11px] text-ink-muted md:inline">{item.turn}</Mono>
          {section.continued && <span className="hidden shrink-0 text-ink-muted md:inline">continued</span>}
          {remote && <span className="hidden shrink-0 text-ink-muted md:inline">on {remote}</span>}
          <Badge variant={variant}>{status}</Badge>
          {turn.tokens > 0 && <Mono className="hidden shrink-0 text-[11px] text-ink-muted md:inline">{tokens(turn.tokens)} tokens</Mono>}
          {turn.issue && <span className="hidden min-w-0 truncate text-danger md:inline">{turn.issue}</span>}
          <Duration item={item} clock={clock} className="md:hidden" />
        </AuthorBand>
      </button>
    </h3>
  );
}

interface RowProps {
  item: ActivityItem;
  links: TaskLinks;
  picked: boolean; // the row you came to see
  clock: number;
  onTurn: (turn: string) => void;
  narrowed: boolean;
}

function Row({ item, links, picked, clock, onTurn, narrowed }: RowProps) {
  return (
    <li data-row={item.key} aria-current={picked || undefined} className={cn(COLUMNS, "hover:bg-surface-1", picked && PICKED)}>
      <div className={cn("flex min-w-0 gap-3 px-4 md:px-6", item.kind === "note" ? "py-1" : "py-2")}>
        <span
          className={cn("mt-0.5 flex size-4 shrink-0 items-center justify-center text-ink-faint [&>svg]:size-3.5", ICON_TONE[item.tone])}
        >
          {ICONS[item.kind]}
        </span>
        <div className="min-w-0 flex-1">
          <Content item={item} links={links} onTurn={onTurn} narrowed={narrowed} />
        </div>
      </div>
      <Rail item={item} clock={clock} />
    </li>
  );
}

const ICONS: Record<ActivityItem["kind"], ReactNode> = {
  turn: null,
  action: <Wrench aria-hidden />,
  message: <MessageSquare aria-hidden />,
  waiting: <Hourglass aria-hidden />,
  issue: <TriangleAlert aria-hidden />,
  ended: <CircleCheck aria-hidden />,
  note: <span className="size-1 rounded-full bg-ink-faint" aria-hidden />,
};

// Icons and dots keep the state's own colour; words that are green use
// success-ink, which reads at 4.5:1 in light.
const ICON_TONE: Record<ActivityTone, string> = {
  default: "",
  info: "text-info",
  success: "text-success",
  warning: "text-warning",
  danger: "text-danger",
};

const WORD_TONE: Record<ActivityTone, string> = {
  default: "",
  info: "text-info",
  success: "text-success-ink",
  warning: "text-warning",
  danger: "text-danger",
};

// The word a text's Show all names it by: a brief to an agent, a message
// to or from you, a report, a question, an issue.
const KINDS: Partial<Record<ActivityItem["kind"], string>> = { ended: "report", waiting: "question", issue: "issue", note: "note" };

function kindOf(item: ActivityItem): string {
  if (item.kind === "message") return item.from === "you" || item.to === "you" ? "message" : "brief";
  return KINDS[item.kind] ?? "message";
}

function Content({
  item,
  links,
  onTurn,
  narrowed,
}: {
  item: ActivityItem;
  links: TaskLinks;
  onTurn: (turn: string) => void;
  narrowed: boolean;
}) {
  const text = item.body && (
    <EntryText text={item.body} links={links} lines={item.kind === "note" ? NOTE_LINES : TEXT_LINES} kind={kindOf(item)} />
  );
  switch (item.kind) {
    case "action": {
      const action = item.action!;
      return (
        <div className="flex min-w-0 flex-col gap-1.5 md:flex-row md:items-start md:gap-3">
          <div className="min-w-0 flex-1">
            <Mono className="text-[11px] text-ink-muted">{action.tool}</Mono>
            <div className="mt-1">
              <Command tool={action.tool} command={action.command} />
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2 md:flex-col md:items-end md:gap-0.5 md:pt-0.5">
            {action.decision === "approved" ? (
              <Badge variant={action.finished ? "default" : "info"}>{action.finished ? "done" : "running"}</Badge>
            ) : (
              <ActionBadge action={action} />
            )}
            {action.reason && <span className="text-[11px] text-ink-muted md:max-w-56 md:text-right">{action.reason}</span>}
          </div>
        </div>
      );
    }
    case "message":
      return (
        <>
          <div className="flex flex-wrap items-center gap-1.5 text-[12px]">
            <Person name={item.from!} />
            <span className="sr-only"> to </span>
            <ArrowRight className="size-3 text-ink-muted" aria-hidden />
            <Person name={item.to!} />
            {item.leadsTo && !narrowed && (
              <button
                type="button"
                onClick={() => onTurn(item.leadsTo!)}
                aria-label={`Show only ${item.leadsTo}`}
                // At least 24 × 24, its padding over the row's, so the row doesn't grow.
                className="-my-1 ml-1 inline-flex min-h-6 min-w-6 cursor-pointer items-center px-1 font-mono text-[11px] text-ink-muted hover:text-ink-primary hover:underline"
              >
                {item.leadsTo}
              </button>
            )}
          </div>
          {text && <div className="mt-1">{text}</div>}
        </>
      );
    case "note":
      return (
        <>
          <p className="truncate text-[12px] text-ink-muted">{item.title}</p>
          {text && <div className="mt-0.5">{text}</div>}
        </>
      );
    default:
      return (
        <>
          <p className={cn("text-[12px] font-semibold", WORD_TONE[item.tone] || "text-ink-primary")}>{item.title}</p>
          {text && <div className="mt-0.5">{text}</div>}
        </>
      );
  }
}

// Command is what an action ran: a shell command in a code box, coloured
// as Bash; any other tool's input (a path, a pattern) as a line of mono.
function Command({ tool, command }: { tool: string; command: string }) {
  if (tool === "Bash") return <CodeBox code={command} language="bash" command />;
  return (
    <Fold lines={3} lineHeight={20} kind="command">
      <p className="font-mono text-[12px] leading-5 wrap-anywhere whitespace-pre-wrap text-ink-primary">{command}</p>
    </Fold>
  );
}

// Person is one end of a message: its chip, then its name in text. "engineer
// on studio" has the engineer's chip.
function Person({ name }: { name: string }) {
  return (
    <span className="inline-flex min-w-0 items-center gap-1.5">
      <Avatar name={name.split(" on ")[0]} size={20} />
      <span className="max-w-[120px] truncate font-mono font-semibold text-ink-primary md:max-w-none">{name}</span>
    </span>
  );
}

// The right-hand rail: one line down the whole feed, a mark per row, its
// time, and how long it took or has been going. On a phone, time only.
function Rail({ item, clock, band = false }: { item: ActivityItem; clock: number; band?: boolean }) {
  const timed = item.kind === "action" || item.kind === "turn";
  return (
    <div className="relative pl-5 sm:pl-7">
      <span className="absolute top-0 bottom-0 left-[10px] w-px bg-divider sm:left-[14px]" aria-hidden />
      <span
        className={cn(
          "absolute left-[6px] size-[9px] rounded-full border-2 border-surface-0 sm:left-[10px]",
          band ? "top-[17px]" : item.kind === "note" ? "top-[9px]" : "top-[12px]",
          NODE[item.tone],
          item.live && "animate-pulse bg-success",
        )}
        aria-hidden
      />
      <div
        className={cn(
          "flex flex-col font-mono text-[11px] leading-4 whitespace-nowrap text-ink-muted",
          band ? "pt-3.5" : item.kind === "note" ? "pt-1" : "pt-2",
        )}
      >
        <span>{clockMinutes(item.at)}</span>
        {timed && <Duration item={item} clock={clock} className="hidden md:inline" />}
      </div>
    </div>
  );
}

// How long a turn or an action took, or "…" while it is still going.
function Duration({ item, clock, className }: { item: ActivityItem; clock: number; className?: string }) {
  const text = item.live ? `${duration(item.at, undefined, clock)}…` : item.until ? duration(item.at, item.until) : "";
  if (!text) return null;
  return <span className={cn("shrink-0 font-mono text-[11px]", item.live ? "text-success-ink" : "text-ink-muted", className)}>{text}</span>;
}

function NowRow({ now }: { now: Now }) {
  return (
    <li className={COLUMNS}>
      <div className="flex items-center gap-3 px-4 py-3 md:px-6">
        <span className={cn("size-2 animate-pulse rounded-full", now.tone === "warning" ? "bg-warning" : "bg-success")} aria-hidden />
        <span className={cn("text-[12px] font-medium", now.tone === "warning" ? "text-warning" : "text-success-ink")}>{now.label}</span>
      </div>
      <div className="relative pl-5 sm:pl-7">
        <span className="absolute top-[16px] bottom-0 left-[10px] w-px bg-divider sm:left-[14px]" aria-hidden />
        <span
          className={cn(
            "absolute top-[16px] left-[6px] size-[9px] animate-pulse rounded-full border-2 border-surface-0 sm:left-[10px]",
            now.tone === "warning" ? "bg-warning" : "bg-success",
          )}
          aria-hidden
        />
        <span className="block pt-3 font-mono text-[11px] text-ink-muted">now</span>
      </div>
    </li>
  );
}

function Empty({ show }: { show: Show }) {
  if (show === "all") return <EmptyState title="Nothing here yet">Every message, turn and action shows here as it happens.</EmptyState>;
  return <EmptyState title={show === "actions" ? "No actions yet" : "No messages yet"}>Choose Everything to see every row.</EmptyState>;
}

// Loading: grey blocks the shape of bands with rows, still (no shimmer),
// and the words for screen readers.
function Loading() {
  return (
    <div>
      <p className="sr-only">Loading activity…</p>
      {[0, 1, 2].map((n) => (
        <div key={n} aria-hidden>
          <div className="flex h-11 items-center gap-2.5 border-y border-divider bg-surface-1 px-4 md:px-6">
            <span className="size-5 rounded-md bg-surface-2" />
            <span className="h-2.5 w-28 rounded-sm bg-surface-2" />
          </div>
          <div className="flex flex-col gap-2 px-4 py-4 pl-12 md:px-6 md:pl-14">
            <span className="h-2.5 w-3/5 rounded-sm bg-surface-2" />
            <span className="h-2.5 w-2/5 rounded-sm bg-surface-2" />
          </div>
        </div>
      ))}
    </div>
  );
}

function LoadError({ error, onRetry }: { error: string; onRetry?: () => void }) {
  return (
    <div className="p-4 md:p-6">
      <div role="alert" className="flex gap-3 rounded-md border border-divider bg-surface-0 p-4">
        <CircleAlert className="mt-0.5 size-4 shrink-0 text-danger" aria-hidden />
        <div className="min-w-0">
          <p className="text-[13px] font-semibold text-ink-primary">Couldn't load the activity</p>
          <p className="mt-1 font-mono text-[11px] wrap-anywhere text-ink-muted">{error}</p>
          <p className="mt-1 text-[12px] text-ink-muted">If it keeps failing, check that Agentry is still running.</p>
          {onRetry && (
            <Button size="sm" className="mt-3" onClick={onRetry}>
              Try again
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

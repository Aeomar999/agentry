import { ReactNode, useLayoutEffect, useRef, useState } from "react";
import { ArrowRight, ArrowUp, CircleCheck, Hourglass, MessageSquare, TriangleAlert, Wrench, X } from "lucide-react";
import { Badge } from "@/components/atoms/Badge";
import { AgentName } from "@/components/molecules/AgentName";
import { EmptyState } from "@/components/molecules/EmptyState";
import { Mono } from "@/components/molecules/Mono";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { ActionBadge } from "@/components/molecules/StatusBadges";
import { Clamp } from "@/components/molecules/Clamp";
import { ActivityItem, ActivityTone, Show, inTurn, newestFirst, shown } from "@/lib/activity";
import { cn } from "@/lib/cn";
import { clockTime, duration } from "@/lib/time";
import { tokens } from "@/lib/tokens";
import { LOCAL } from "@/lib/machines";
import { useNow } from "@/live/use-now";

// Everything that happened on a task, newest at the top and the oldest at
// the bottom; each turn's band stays above its own rows. Down the right runs
// the time: when each row happened, how long it took, and a pulsing mark on
// whatever is still going. Narrowed to one turn, it shows that turn's work.

export interface Now {
  label: string; // what is going on right now, as "dev is working"
  tone: "success" | "warning";
}

interface ActivityFeedProps {
  items: ActivityItem[];
  turn: string; // "" for every turn
  show: Show;
  row: string; // the key of the row you came to see, picked out; "" for none
  now: Now | null; // null when nothing is happening
  onTurn: (turn: string) => void;
  onShow: (show: Show) => void;
}

const NODE: Record<ActivityTone, string> = {
  default: "bg-ink-faint",
  info: "bg-info",
  success: "bg-success",
  warning: "bg-warning",
  danger: "bg-danger",
};

export function ActivityFeed({ items, turn, show, row, now, onTurn, onShow }: ActivityFeedProps) {
  const rows = newestFirst(shown(turn ? inTurn(items, turn) : items, show));
  const open = turn ? items.find((item) => item.kind === "turn" && item.turn === turn) : undefined;
  const live = rows.some((row) => row.live) || Boolean(now);
  const clock = useNow(live);

  // A new view starts at the top, where the newest rows are, or with the
  // row you came to see in the middle. Once you have scrolled down, a
  // button takes you back up.
  const scroller = useRef<HTMLDivElement>(null);
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

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-11 shrink-0 scrollbar-thin items-center gap-3 overflow-x-auto border-b border-divider px-6">
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
        <Mono className="ml-auto text-[11px] text-ink-faint">
          {rows.length} {rows.length === 1 ? "row" : "rows"}
        </Mono>
      </div>

      <div className="relative min-h-0 flex-1">
        <div ref={scroller} onScroll={onScroll} className="h-full scrollbar-thin overflow-y-auto">
          {rows.length === 0 && !now ? (
            <EmptyState title="Nothing here yet">Every message, turn and tool call shows here as it happens.</EmptyState>
          ) : (
            <ol className="pb-2">
              {now && !(turn && open && !open.live) && <NowRow now={now} />}
              {rows.map((item) => (
                <Row key={item.key} item={item} picked={item.key === row} clock={clock} onTurn={onTurn} narrowed={Boolean(turn)} />
              ))}
            </ol>
          )}
        </div>
        {away && (
          <button
            type="button"
            onClick={toTop}
            className="absolute top-3 left-1/2 inline-flex h-7 -translate-x-1/2 cursor-pointer items-center gap-1.5 rounded-md border border-divider-2 bg-surface-2 px-3 text-[12px] font-medium text-ink-primary hover:bg-surface-3 [&>svg]:size-3.5"
          >
            <ArrowUp />
            Newest
          </button>
        )}
      </div>
    </div>
  );
}

interface RowProps {
  item: ActivityItem;
  picked: boolean; // the row you came to see
  clock: number;
  onTurn: (turn: string) => void;
  narrowed: boolean;
}

// The row you came to see: a tinted surface with a hairline all round, flat
// like every selection, and the text left as it is so it reads the same.
// Solid, not see-through, so a turn's band still covers what scrolls under it.
const PICKED =
  "bg-[color-mix(in_srgb,var(--color-info)_14%,var(--color-surface-0))] outline outline-1 -outline-offset-1 outline-info/60 hover:bg-[color-mix(in_srgb,var(--color-info)_14%,var(--color-surface-0))]";

function Row({ item, picked, clock, onTurn, narrowed }: RowProps) {
  if (item.kind === "turn") return <TurnRow item={item} picked={picked} clock={clock} onTurn={onTurn} narrowed={narrowed} />;
  const icon = ICONS[item.kind];
  return (
    <li
      data-row={item.key}
      aria-current={picked || undefined}
      className={cn("grid grid-cols-[minmax(0,1fr)_104px] hover:bg-surface-1 md:grid-cols-[minmax(0,1fr)_120px]", picked && PICKED)}
    >
      <div className={cn("flex min-w-0 gap-3 px-6", item.kind === "note" ? "py-1" : "py-2")}>
        <span
          className={cn("mt-0.5 flex size-4 shrink-0 items-center justify-center text-ink-faint [&>svg]:size-3.5", TONE_TEXT[item.tone])}
        >
          {icon}
        </span>
        <div className="min-w-0 flex-1">
          <Content item={item} onTurn={onTurn} narrowed={narrowed} />
        </div>
      </div>
      <Rail item={item} clock={clock} />
    </li>
  );
}

const ICONS: Record<ActivityItem["kind"], ReactNode> = {
  turn: null,
  action: <Wrench />,
  message: <MessageSquare />,
  waiting: <Hourglass />,
  issue: <TriangleAlert />,
  ended: <CircleCheck />,
  note: <span className="size-1 rounded-full bg-ink-faint" />,
};

const TONE_TEXT: Record<ActivityTone, string> = {
  default: "",
  info: "text-info",
  success: "text-success",
  warning: "text-warning",
  danger: "text-danger",
};

function Content({ item, onTurn, narrowed }: { item: ActivityItem; onTurn: (turn: string) => void; narrowed: boolean }) {
  switch (item.kind) {
    case "action": {
      const action = item.action!;
      return (
        <div className="flex min-w-0 items-start gap-3">
          <div className="min-w-0 flex-1">
            <Mono className="text-[11px] text-ink-muted">{action.tool}</Mono>
            <Clamp text={action.command} lines={3} mono />
          </div>
          <div className="flex shrink-0 flex-col items-end gap-0.5 pt-0.5">
            {action.decision === "approved" ? (
              <Badge variant={action.finished ? "default" : "info"}>{action.finished ? "done" : "running"}</Badge>
            ) : (
              <ActionBadge action={action} />
            )}
            {action.reason && <span className="max-w-56 text-right text-[11px] text-ink-faint">{action.reason}</span>}
          </div>
        </div>
      );
    }
    case "message":
      return (
        <>
          <div className="flex flex-wrap items-center gap-1.5 text-[12px]">
            <AgentName name={item.from!} className="font-semibold text-ink-primary" />
            <ArrowRight className="size-3 text-ink-faint" aria-hidden />
            <AgentName name={item.to!} className="font-semibold text-ink-primary" />
            {item.leadsTo && !narrowed && (
              <button
                type="button"
                onClick={() => onTurn(item.leadsTo!)}
                className="ml-1 cursor-pointer font-mono text-[11px] text-ink-muted hover:text-ink-primary hover:underline"
              >
                {item.leadsTo}
              </button>
            )}
          </div>
          {item.body && <Clamp text={item.body} lines={4} className="mt-1" />}
        </>
      );
    case "note":
      return <p className="truncate text-[12px] text-ink-muted">{item.title}</p>;
    default:
      return (
        <>
          <p className={cn("text-[12px] font-semibold", TONE_TEXT[item.tone] || "text-ink-primary")}>{item.title}</p>
          {item.body && <Clamp text={item.body} lines={item.kind === "ended" ? 6 : 4} className="mt-0.5" />}
        </>
      );
  }
}

// A turn opens with a band across the feed, which stays at the top while
// you scroll through its rows. Clicking it shows that turn alone.
function TurnRow({ item, picked, clock, onTurn, narrowed }: RowProps) {
  const turn = item.turnInfo!;
  const status = turn.issue ? (
    <Badge variant="danger">issue</Badge>
  ) : turn.stopped ? (
    <Badge variant="warning">stopped by you</Badge>
  ) : turn.ended ? (
    <Badge>ended</Badge>
  ) : (
    <Badge variant="info">running</Badge>
  );
  return (
    <li
      data-row={item.key}
      aria-current={picked || undefined}
      className={cn(
        "sticky top-0 z-10 mt-2 grid grid-cols-[minmax(0,1fr)_104px] border-y border-divider bg-surface-1 first:mt-0 md:grid-cols-[minmax(0,1fr)_120px]",
        picked && PICKED,
      )}
    >
      <button
        type="button"
        disabled={narrowed}
        onClick={() => onTurn(item.turn)}
        className="flex h-9 min-w-0 cursor-pointer items-center gap-2.5 px-6 text-left text-[12px] disabled:cursor-default"
      >
        <Mono className="text-[11px] text-ink-faint">{item.turn}</Mono>
        <AgentName name={turn.agent} className="font-semibold text-ink-primary" />
        {turn.worker && turn.worker !== LOCAL && <span className="text-ink-muted">on {turn.worker}</span>}
        {status}
        {turn.tokens > 0 && <Mono className="text-[11px] text-ink-muted">{tokens(turn.tokens)} tokens</Mono>}
        {turn.issue && <span className="min-w-0 truncate text-danger">{turn.issue}</span>}
      </button>
      <Rail item={item} clock={clock} />
    </li>
  );
}

// The right-hand rail: one line down the whole feed, a mark per row, its
// time, and how long it took or has been going.
function Rail({ item, clock }: { item: ActivityItem; clock: number }) {
  const timed = item.kind === "action" || item.kind === "turn";
  return (
    <div className="relative pl-7">
      <span className="absolute top-0 bottom-0 left-[14px] w-px bg-divider" aria-hidden />
      <span
        className={cn(
          "absolute left-[10px] size-[9px] rounded-full border-2 border-surface-0",
          item.kind === "turn" ? "top-[13px]" : item.kind === "note" ? "top-[9px]" : "top-[12px]",
          NODE[item.tone],
          item.live && "animate-pulse bg-success",
        )}
        aria-hidden
      />
      <div
        className={cn(
          "flex flex-col font-mono text-[11px] leading-4",
          item.kind === "turn" ? "pt-2.5" : item.kind === "note" ? "pt-1" : "pt-2",
        )}
      >
        <span className="text-ink-muted">{clockTime(item.at)}</span>
        {timed && (
          <span className={item.live ? "text-success" : "text-ink-faint"}>
            {item.live ? `${duration(item.at, undefined, clock)}…` : item.until ? duration(item.at, item.until) : ""}
          </span>
        )}
      </div>
    </div>
  );
}

function NowRow({ now }: { now: Now }) {
  return (
    <li className="grid grid-cols-[minmax(0,1fr)_104px] md:grid-cols-[minmax(0,1fr)_120px]">
      <div className="flex items-center gap-3 px-6 py-3">
        <span className={cn("size-2 animate-pulse rounded-full", now.tone === "warning" ? "bg-warning" : "bg-success")} aria-hidden />
        <span className={cn("text-[12px] font-medium", now.tone === "warning" ? "text-warning" : "text-success")}>{now.label}</span>
      </div>
      <div className="relative pl-7">
        <span className="absolute top-[16px] bottom-0 left-[14px] w-px bg-divider" aria-hidden />
        <span
          className={cn(
            "absolute top-[16px] left-[10px] size-[9px] animate-pulse rounded-full border-2 border-surface-0",
            now.tone === "warning" ? "bg-warning" : "bg-success",
          )}
          aria-hidden
        />
        <span className="block pt-3 font-mono text-[11px] text-ink-muted">now</span>
      </div>
    </li>
  );
}

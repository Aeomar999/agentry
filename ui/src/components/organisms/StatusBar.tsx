import { GitBranch, SquareTerminal } from "lucide-react";
import { StatusDot, DotTone } from "@/components/atoms/StatusDot";
import { Kbd } from "@/components/atoms/Kbd";
import type { Connection } from "@/live/LiveProvider";
import { modKey } from "@/lib/shortcuts";
import { cn } from "@/lib/cn";

const CONNECTION: Record<Connection, { tone: DotTone; label: string }> = {
  connecting: { tone: "muted", label: "Connecting" },
  live: { tone: "success", label: "Live" },
  reconnecting: { tone: "warning", label: "Reconnecting" },
};

interface StatusBarProps {
  connection: Connection;
  position: number;
  defaultRepo: string;
  working: number;
  needsYou: number; // questions and approvals waiting for you
  onOpenQuestions: () => void;
  terminalOpen: boolean;
  onToggleTerminal: () => void;
}

export function StatusBar({
  connection,
  position,
  defaultRepo,
  working,
  needsYou,
  onOpenQuestions,
  terminalOpen,
  onToggleTerminal,
}: StatusBarProps) {
  const { tone, label } = CONNECTION[connection];
  return (
    <footer className="flex h-statusbar shrink-0 items-center gap-4 border-t border-divider bg-surface-1 px-3 text-[11px] text-ink-muted">
      <span className="flex items-center gap-1.5">
        <StatusDot tone={tone} />
        {label}
      </span>
      <span className="font-mono">log #{position}</span>
      {working > 0 && <span>{working === 1 ? "1 task working" : `${working} tasks working`}</span>}
      {defaultRepo && (
        <span className="flex min-w-0 items-center gap-1 max-sm:hidden">
          <GitBranch className="size-3 shrink-0" aria-hidden />
          <span className="truncate font-mono">{defaultRepo}</span>
        </span>
      )}
      {needsYou > 0 && (
        <button
          type="button"
          onClick={onOpenQuestions}
          className="ml-auto flex h-5 cursor-pointer items-center gap-1.5 rounded-sm bg-warning-soft px-1.5 whitespace-nowrap text-warning hover:text-ink-primary"
        >
          <StatusDot tone="warning" />
          {needsYou === 1 ? "1 waiting for you" : `${needsYou} waiting for you`}
        </button>
      )}
      <button
        type="button"
        onClick={onToggleTerminal}
        title={`Show or hide the terminal (${modKey} J)`}
        className={cn(
          "flex h-5 cursor-pointer items-center gap-1.5 rounded-sm px-1.5 whitespace-nowrap hover:bg-surface-3 hover:text-ink-primary",
          terminalOpen && "bg-surface-3 text-ink-primary",
          needsYou === 0 && "ml-auto",
        )}
      >
        <SquareTerminal className="size-3.5" aria-hidden />
        Terminal
      </button>
      <span className="flex items-center gap-1.5 whitespace-nowrap max-sm:hidden">
        Press <Kbd>?</Kbd> for shortcuts
      </span>
    </footer>
  );
}

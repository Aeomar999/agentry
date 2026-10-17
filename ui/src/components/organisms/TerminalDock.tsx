import { Inbox, Sparkles, X } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";
import { Kbd } from "@/components/atoms/Kbd";
import { ResizeHandle } from "@/components/molecules/ResizeHandle";
import { deskTerminalURL } from "@/api/client";
import { cn } from "@/lib/cn";
import type { Theme } from "@/lib/theme";
import { modKey } from "@/lib/shortcuts";
import { TerminalView } from "./TerminalView";
import { QuestionsPanel } from "./QuestionsPanel";

// The dock at the bottom of the content area, toggled with Ctrl+J and
// resized by dragging its top edge. Its tabs: Questions, everything waiting
// for you from every task; and Claude, your own session that turns what you
// say into tasks for the coordinator. An agent's terminal is on its task's
// page, in the Terminal tab.

export const QUESTIONS = "questions";
export const DESK = "claude";
export type DockTab = typeof QUESTIONS | typeof DESK;

interface TerminalDockProps {
  height: number;
  onResize: (height: number) => void; // the shell clamps it
  selected: DockTab;
  question: { id: string; asked: number }; // the question to show, when a page asked for one
  waiting: number; // how many questions wait for you
  onSelect: (tab: DockTab) => void;
  onOpenTaskTerminal: (taskId: string) => void; // on the task's page
  onClose: () => void;
  theme: Theme;
}

export function TerminalDock({
  height,
  onResize,
  selected: tab,
  question,
  waiting,
  onSelect,
  onOpenTaskTerminal,
  onClose,
  theme,
}: TerminalDockProps) {
  return (
    <section
      aria-label="Terminal"
      className="relative flex shrink-0 flex-col border-t border-divider bg-surface-1"
      style={{ height, maxHeight: "100%" }}
    >
      <ResizeHandle height={height} onResize={onResize} />
      <div className="flex h-9 shrink-0 scrollbar-thin items-center gap-0.5 overflow-x-auto border-b border-divider px-2">
        <DockTab
          active={tab === QUESTIONS}
          onClick={() => onSelect(QUESTIONS)}
          icon={<Inbox />}
          label={waiting > 0 ? `Questions ${waiting}` : "Questions"}
          attention={waiting > 0}
        />
        <DockTab active={tab === DESK} onClick={() => onSelect(DESK)} icon={<Sparkles />} label="Claude" />
        <span className="ml-auto flex shrink-0 items-center gap-1.5 pl-3 text-[11px] text-ink-faint">
          <Kbd>{modKey} J</Kbd>
          <IconButton icon={<X />} label="Close the dock" onClick={onClose} className="size-7" />
        </span>
      </div>
      <div className="min-h-0 flex-1">
        {tab === QUESTIONS ? (
          <QuestionsPanel onOpenTerminal={onOpenTaskTerminal} focus={question} />
        ) : (
          <TerminalView key={`desk-${theme}`} url={deskTerminalURL(theme)} title="claude" takeControlOnConnect />
        )}
      </div>
    </section>
  );
}

function DockTab({
  active,
  onClick,
  icon,
  label,
  attention = false,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  label: string;
  attention?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "inline-flex h-7 shrink-0 cursor-pointer items-center gap-1.5 rounded-sm px-2.5 font-mono text-[12px] whitespace-nowrap [&>svg]:size-3.5",
        active ? "bg-surface-3 text-ink-primary" : "text-ink-muted hover:bg-surface-2 hover:text-ink-primary",
        attention && "text-warning",
      )}
    >
      {icon}
      {label}
    </button>
  );
}

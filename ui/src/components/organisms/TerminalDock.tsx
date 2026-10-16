import { PointerEvent as ReactPointerEvent, useRef } from "react";
import { Sparkles, SquareTerminal, X } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";
import { Kbd } from "@/components/atoms/Kbd";
import { EmptyState } from "@/components/molecules/EmptyState";
import { terminalURL, deskTerminalURL } from "@/api/client";
import type { TaskSummary } from "@/api/types";
import { cn } from "@/lib/cn";
import type { Theme } from "@/lib/theme";
import { modKey } from "@/lib/shortcuts";
import { TerminalView } from "./TerminalView";

// The terminal dock at the bottom of the content area, toggled with Ctrl+J
// and resized by dragging its top edge. Its tabs: Claude, your own session
// that turns what you say into tasks for the coordinator, then the terminal
// of every agent working right now.

export const DESK = "claude";

interface TerminalDockProps {
  height: number;
  onResize: (height: number) => void; // the shell clamps it
  working: TaskSummary[];
  selected: string; // DESK or a task id
  onSelect: (tab: string) => void;
  onClose: () => void;
  theme: Theme;
}

export function TerminalDock({ height, onResize, working, selected, onSelect, onClose, theme }: TerminalDockProps) {
  const task = working.find((t) => t.id === selected);
  const tab = task ? task.id : DESK;
  return (
    <section
      aria-label="Terminal"
      className="relative flex shrink-0 flex-col border-t border-divider bg-surface-1"
      style={{ height, maxHeight: "100%" }}
    >
      <ResizeHandle height={height} onResize={onResize} />
      <div className="flex h-9 shrink-0 scrollbar-thin items-center gap-0.5 overflow-x-auto border-b border-divider px-2">
        <DockTab active={tab === DESK} onClick={() => onSelect(DESK)} icon={<Sparkles />} label="Claude" />
        {working.map((t) => (
          <DockTab
            key={t.id}
            active={tab === t.id}
            onClick={() => onSelect(t.id)}
            icon={<SquareTerminal />}
            label={`${t.id} · ${t.currentAgent}`}
          />
        ))}
        <span className="ml-auto flex shrink-0 items-center gap-1.5 pl-3 text-[11px] text-ink-faint">
          <Kbd>{modKey} J</Kbd>
          <IconButton icon={<X />} label="Close the terminal" onClick={onClose} className="size-7" />
        </span>
      </div>
      <div className="min-h-0 flex-1">
        {tab === DESK ? (
          <TerminalView key={`desk-${theme}`} url={deskTerminalURL(theme)} title="claude" takeControlOnConnect />
        ) : task ? (
          // A new key each turn: every turn is a new session.
          <TerminalView key={`${task.id}-${task.turns}`} url={terminalURL(task.id)} title={task.currentAgent} />
        ) : (
          <EmptyState icon={<SquareTerminal />} title="No agent is working right now" />
        )}
      </div>
    </section>
  );
}

function DockTab({ active, onClick, icon, label }: { active: boolean; onClick: () => void; icon: React.ReactNode; label: string }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "inline-flex h-7 shrink-0 cursor-pointer items-center gap-1.5 rounded-sm px-2.5 font-mono text-[12px] whitespace-nowrap [&>svg]:size-3.5",
        active ? "bg-surface-3 text-ink-primary" : "text-ink-muted hover:bg-surface-2 hover:text-ink-primary",
      )}
    >
      {icon}
      {label}
    </button>
  );
}

// Dragging the top edge sets the height: up is taller.
function ResizeHandle({ height, onResize }: { height: number; onResize: (height: number) => void }) {
  const start = useRef<{ y: number; height: number } | null>(null);

  function onPointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    start.current = { y: event.clientY, height };
    event.currentTarget.setPointerCapture(event.pointerId);
  }
  function onPointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    if (start.current) onResize(start.current.height + start.current.y - event.clientY);
  }
  function onPointerUp() {
    start.current = null;
  }

  return (
    <div
      role="separator"
      aria-orientation="horizontal"
      aria-label="Drag to resize the terminal"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      className="absolute inset-x-0 -top-[3px] z-10 h-[6px] cursor-row-resize hover:bg-divider-2 active:bg-ink-faint"
    />
  );
}

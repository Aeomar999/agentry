import { ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { CornerDownLeft, Search } from "lucide-react";
import { Kbd } from "@/components/atoms/Kbd";
import { cn } from "@/lib/cn";

// The palette: type to find a page, a task, an agent or a command, and Enter
// goes there. Results are grouped by kind, and it is driven by the keyboard.

export interface PaletteItem {
  id: string;
  group: string; // "Go to", "Tasks", "Agents", "Commands"
  label: string;
  hint?: string; // muted text on the right: an id, a status
  terms?: string; // more words that find it, not shown
  icon?: ReactNode;
  run: () => void;
}

interface CommandPaletteProps {
  open: boolean;
  items: PaletteItem[];
  onClose: () => void;
}

export function CommandPalette({ open, items, onClose }: CommandPaletteProps) {
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    setQuery("");
    setCursor(0);
    input.current?.focus();
  }, [open]);

  const matches = useMemo(() => {
    const words = query.toLowerCase().split(/\s+/).filter(Boolean);
    return items.filter((item) => {
      const text = `${item.group} ${item.label} ${item.hint ?? ""} ${item.terms ?? ""}`.toLowerCase();
      return words.every((word) => text.includes(word));
    });
  }, [items, query]);

  useEffect(() => setCursor(0), [query]);
  useEffect(() => {
    list.current?.querySelector(`[data-index="${cursor}"]`)?.scrollIntoView({ block: "nearest" });
  }, [cursor]);

  if (!open) return null;

  function run(item: PaletteItem | undefined) {
    if (!item) return;
    onClose();
    item.run();
  }

  function onKeyDown(event: React.KeyboardEvent) {
    if (event.key === "ArrowDown") setCursor((at) => Math.min(matches.length - 1, at + 1));
    else if (event.key === "ArrowUp") setCursor((at) => Math.max(0, at - 1));
    else if (event.key === "Enter") run(matches[cursor]);
    else if (event.key === "Escape") onClose();
    else return;
    event.preventDefault();
  }

  const groups = [...new Set(matches.map((item) => item.group))];
  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center bg-black/50 px-4 pt-[12vh]" onMouseDown={onClose}>
      <div
        role="dialog"
        aria-label="Command palette"
        className="flex max-h-[60vh] w-full max-w-xl flex-col overflow-hidden rounded-lg border border-divider-2 bg-surface-1"
        onMouseDown={(event) => event.stopPropagation()}
        onKeyDown={onKeyDown}
      >
        <div className="flex h-11 items-center gap-2 border-b border-divider px-3">
          <Search className="size-4 shrink-0 text-ink-muted" aria-hidden />
          <input
            ref={input}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search tasks, agents and commands"
            className="h-full min-w-0 flex-1 bg-transparent text-[14px] text-ink-primary outline-none placeholder:text-ink-faint"
          />
          <Kbd>Esc</Kbd>
        </div>
        <div ref={list} className="scrollbar-thin overflow-y-auto p-1.5">
          {matches.length === 0 && <p className="px-3 py-6 text-center text-[13px] text-ink-muted">Nothing matches "{query}".</p>}
          {groups.map((group) => (
            <div key={group} className="pb-1">
              <p className="px-2.5 pt-2 pb-1 text-[10.5px] font-semibold tracking-wider text-ink-faint uppercase">{group}</p>
              {matches.map((item, index) =>
                item.group !== group ? null : (
                  <button
                    key={item.id}
                    type="button"
                    data-index={index}
                    onMouseMove={() => setCursor(index)}
                    onClick={() => run(item)}
                    className={cn(
                      "flex h-8 w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] [&>svg]:size-4 [&>svg]:shrink-0",
                      index === cursor ? "bg-surface-3 text-ink-primary" : "text-ink-secondary",
                    )}
                  >
                    {item.icon}
                    <span className="min-w-0 flex-1 truncate">{item.label}</span>
                    {item.hint && <span className="shrink-0 font-mono text-[11px] text-ink-muted">{item.hint}</span>}
                    {index === cursor && <CornerDownLeft className="size-3.5 text-ink-muted" aria-hidden />}
                  </button>
                ),
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

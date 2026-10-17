import { useEffect, useRef } from "react";
import { cn } from "@/lib/cn";

// A right-click menu, as an editor's: where you clicked, kept inside the
// window, closed by a pick, a click elsewhere, Escape or a scroll.

export type MenuItem = { label: string; onClick: () => void; hint?: string; disabled?: boolean } | "separator";

export function ContextMenu({ x, y, items, onClose }: { x: number; y: number; items: MenuItem[]; onClose: () => void }) {
  const menu = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const close = (event: Event) => {
      if (event instanceof KeyboardEvent && event.key !== "Escape") return;
      if (event.type === "mousedown" && menu.current?.contains(event.target as Node)) return;
      onClose();
    };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", close);
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", close);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
    };
  }, [onClose]);
  const width = 240;
  const height = items.length * 28 + 8;
  const left = Math.min(x, window.innerWidth - width - 8);
  const top = Math.min(y, window.innerHeight - height - 8);
  return (
    <div
      ref={menu}
      role="menu"
      className="fixed z-50 flex flex-col rounded-md border border-divider-2 bg-surface-2 py-1"
      style={{ left, top, width }}
      onContextMenu={(event) => event.preventDefault()}
    >
      {items.map((item, i) =>
        item === "separator" ? (
          <span key={i} className="my-1 h-px bg-divider-2" />
        ) : (
          <button
            key={i}
            type="button"
            role="menuitem"
            disabled={item.disabled}
            onClick={() => {
              onClose();
              item.onClick();
            }}
            className={cn(
              "flex h-7 cursor-pointer items-center justify-between px-3 text-left text-[12.5px] text-ink-primary hover:bg-surface-3 disabled:cursor-default disabled:text-ink-faint disabled:hover:bg-transparent",
            )}
          >
            {item.label}
            {item.hint && <span className="text-[11px] text-ink-faint">{item.hint}</span>}
          </button>
        ),
      )}
    </div>
  );
}

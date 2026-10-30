import { useEffect } from "react";
import { Kbd } from "@/components/atoms/Kbd";
import { SHORTCUTS } from "@/lib/shortcuts";

export function ShortcutOverlay({ open, onClose }: { open: boolean; onClose: () => void }) {
  useEffect(() => {
    if (!open) return;
    const close = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [open, onClose]);

  if (!open) return null;
  const groups = [...new Set(SHORTCUTS.map((shortcut) => shortcut.group))];
  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/50 px-4" onMouseDown={onClose}>
      <div
        role="dialog"
        aria-label="Keyboard shortcuts"
        className="w-full max-w-md rounded-lg border border-divider-2 bg-surface-1 p-5"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-[14px] font-semibold text-ink-primary">Keyboard shortcuts</h2>
          <Kbd>Esc</Kbd>
        </div>
        <div className="flex flex-col gap-4">
          {groups.map((group) => (
            <div key={group}>
              <p className="mb-1.5 text-[10.5px] font-semibold tracking-wider text-ink-faint uppercase">{group}</p>
              {SHORTCUTS.filter((shortcut) => shortcut.group === group).map((shortcut) => (
                <div key={shortcut.label} className="flex h-7 items-center justify-between text-[13px] text-ink-secondary">
                  {shortcut.label}
                  <span className="flex items-center gap-1">
                    {shortcut.keys.map((key, i) => (
                      <span key={i} className="flex items-center gap-1">
                        {i > 0 && <span className="text-[11px] text-ink-faint">then</span>}
                        <Kbd>{key}</Kbd>
                      </span>
                    ))}
                  </span>
                </div>
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

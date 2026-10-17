import { ReactNode, useEffect } from "react";
import { cn } from "@/lib/cn";

// A small modal: a dimmed page behind, Esc or a click outside closes it.
// Wide fits a line of shell.
export function Dialog({ title, onClose, wide, children }: { title: string; onClose: () => void; wide?: boolean; children: ReactNode }) {
  useEffect(() => {
    const close = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center bg-black/50 px-4 pt-[15vh]" onMouseDown={onClose}>
      <div
        role="dialog"
        aria-label={title}
        className={cn("w-full rounded-lg border border-divider-2 bg-surface-1", wide ? "max-w-2xl" : "max-w-md")}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <h2 className="border-b border-divider px-4 py-3 text-[14px] font-semibold text-ink-primary">{title}</h2>
        <div className="p-4">{children}</div>
      </div>
    </div>
  );
}

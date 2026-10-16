import { ReactNode, useEffect } from "react";

// A small modal: a dimmed page behind, Esc or a click outside closes it.
export function Dialog({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
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
        className="w-full max-w-md rounded-lg border border-divider-2 bg-surface-1"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <h2 className="border-b border-divider px-4 py-3 text-[14px] font-semibold text-ink-primary">{title}</h2>
        <div className="p-4">{children}</div>
      </div>
    </div>
  );
}

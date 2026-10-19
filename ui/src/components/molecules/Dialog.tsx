import { ReactNode, useEffect } from "react";
import { cn } from "@/lib/cn";

// A modal: a dimmed page behind, Esc or a click outside closes it. Wide fits
// a line of shell; large fits a form. A body taller than the window scrolls,
// the dialog ending above the window's bottom edge.
export function Dialog({
  title,
  onClose,
  wide,
  large,
  children,
}: {
  title: string;
  onClose: () => void;
  wide?: boolean;
  large?: boolean;
  children: ReactNode;
}) {
  useEffect(() => {
    const close = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [onClose]);

  return (
    <div
      className={cn("fixed inset-0 z-40 flex items-start justify-center bg-black/50 px-4", large ? "pt-[6vh]" : "pt-[15vh]")}
      onMouseDown={onClose}
    >
      <div
        role="dialog"
        aria-label={title}
        className={cn(
          "flex w-full flex-col rounded-lg border border-divider-2 bg-surface-1",
          large ? "max-h-[88vh] max-w-3xl" : wide ? "max-h-[80vh] max-w-2xl" : "max-h-[80vh] max-w-md",
        )}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <h2 className="shrink-0 border-b border-divider px-4 py-3 text-[14px] font-semibold text-ink-primary">{title}</h2>
        <div className={cn("min-h-0 scrollbar-thin overflow-y-auto", large ? "p-5" : "p-4")}>{children}</div>
      </div>
    </div>
  );
}

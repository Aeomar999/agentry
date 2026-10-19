import { ReactNode, RefObject, useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";

// A modal side sheet over the right of the page: 680px wide, the whole
// screen under 640px, over a flat scrim. Everything else on the page is
// inert while it is open, so focus and the screen reader stay in it; focus
// goes back where it was when it closes. The body scrolls between the
// title and the footer. canClose false turns Close and Esc off, as while
// it saves.
interface SideSheetProps {
  title: string;
  onClose: () => void;
  canClose?: boolean;
  footer: ReactNode;
  children: ReactNode;
}

export function SideSheet({ title, onClose, canClose = true, footer, children }: SideSheetProps) {
  const titleId = useId();
  const sheet = useRef<HTMLDivElement>(null);
  useInertBehind(sheet);

  useEffect(() => {
    const close = (event: KeyboardEvent) => event.key === "Escape" && canClose && onClose();
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [onClose, canClose]);

  return createPortal(
    <div ref={sheet} className="fixed inset-0 z-40 flex justify-end bg-black/50">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="flex h-full w-full max-w-[680px] flex-col border-l border-divider-2 bg-surface-1 max-sm:max-w-none max-sm:border-l-0"
      >
        <div className="flex h-12 shrink-0 items-center justify-between gap-3 border-b border-divider pr-2 pl-5">
          <h2 id={titleId} className="truncate text-[15px] font-semibold text-ink-primary">
            {title}
          </h2>
          <IconButton icon={<X />} label="Close" onClick={onClose} disabled={!canClose} />
        </div>
        <div className="min-h-0 flex-1 scrollbar-thin overflow-y-auto px-5 py-5">{children}</div>
        <div className="shrink-0 border-t border-divider px-5 py-3">{footer}</div>
      </div>
    </div>,
    document.body,
  );
}

// useInertBehind makes every other part of the page inert while the sheet
// is open, and puts focus back where it was once it closes.
function useInertBehind(sheet: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const opener = document.activeElement as HTMLElement | null;
    const behind = [...document.body.children].filter(
      (element): element is HTMLElement => element instanceof HTMLElement && element !== sheet.current && !element.inert,
    );
    for (const element of behind) element.inert = true;
    return () => {
      for (const element of behind) element.inert = false;
      opener?.focus();
    };
  }, [sheet]);
}

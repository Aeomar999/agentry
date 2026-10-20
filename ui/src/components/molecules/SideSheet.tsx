import { ReactNode, RefObject, useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";

// A modal side sheet over the right of the page: 680px wide, the whole
// screen under 640px, over a flat scrim. Everything else on the page is
// inert while it is open, so focus and the screen reader stay in it, and
// the page doesn't scroll under it. Focus goes to returnTo when it closes,
// or back where it was when it opened. The body scrolls between the title
// and the footer. canClose false turns Close and Esc off, as while it saves.
interface SideSheetProps {
  title: string;
  returnTo?: RefObject<HTMLElement | null>; // for an opener that is gone by then, as one in a closed dialog
  onClose: () => void;
  canClose?: boolean;
  footer: ReactNode;
  children: ReactNode;
}

export function SideSheet({ title, returnTo, onClose, canClose = true, footer, children }: SideSheetProps) {
  const titleId = useId();
  const sheet = useRef<HTMLDivElement>(null);
  // Remembered as it first renders: by its effects, autoFocus inside it has run.
  const [opener] = useState(() => document.activeElement);
  useInertBehind(sheet); // first, so the page is no longer inert when focus returns to it
  useFocusReturn(sheet, returnTo, opener);
  useScrollLock();

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

// useInertBehind makes every other part of the page inert while the sheet is open.
function useInertBehind(sheet: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const behind = [...document.body.children].filter(
      (element): element is HTMLElement => element instanceof HTMLElement && element !== sheet.current && !element.inert,
    );
    for (const element of behind) element.inert = true;
    return () => {
      for (const element of behind) element.inert = false;
    };
  }, [sheet]);
}

// useFocusReturn focuses returnTo, or else the opener, once the sheet
// closes, unless focus has gone on somewhere else by then: into the sheet
// that replaced this one, say.
function useFocusReturn(sheet: RefObject<HTMLElement | null>, returnTo: RefObject<HTMLElement | null> | undefined, opener: Element | null) {
  useEffect(() => {
    const root = sheet.current;
    return () => {
      const at = document.activeElement;
      if (at && at !== document.body && !root?.contains(at)) return;
      const target = [returnTo?.current, opener].find((element) => element instanceof HTMLElement && element.isConnected);
      if (target instanceof HTMLElement) target.focus();
    };
  }, [sheet, returnTo, opener]);
}

// useScrollLock stops the page scrolling while the sheet is open: inert
// doesn't stop the wheel over the scrim.
function useScrollLock() {
  useEffect(() => {
    const page = document.scrollingElement ?? document.documentElement;
    if (!(page instanceof HTMLElement)) return;
    const was = page.style.overflow;
    page.style.overflow = "hidden";
    return () => {
      page.style.overflow = was;
    };
  }, []);
}

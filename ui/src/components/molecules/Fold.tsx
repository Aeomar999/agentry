import { ReactNode, useCallback, useId, useLayoutEffect, useRef, useState } from "react";
import { cn } from "@/lib/cn";

// Long text, cut to a few lines until you ask for all of it. The cut is a
// clean edge at the bottom of the last whole line above the limit, and what
// is below it stays in the page for screen readers but out of the Tab order:
// a link no one can see can't be tabbed to. Never `inert`, which would take
// the visible links too. Folds nest (a code box in a report), so a link is
// out of the Tab order while any fold around it cuts it off. A cut through
// a framed fold inside (a code box) gets a hairline, so the box still ends.

export interface FoldProps {
  children: ReactNode;
  lines: number; // how many lines show while folded
  lineHeight: number; // of the text, in px
  kind: string; // what the text is, for the button's name: brief, report, command, code…
  className?: string; // the frame around the text (a code box), above the button
  aside?: ReactNode; // in the frame beside the text, never cut (a code box's Copy)
}

interface Size {
  full: number; // the text's whole height
  cut: number; // where it is cut while folded
  edge: boolean; // the cut passes through a frame inside, so it draws one
}

const FOCUSABLE = "a[href], button, [tabindex]";

export function Fold({ children, lines, lineHeight, kind, className, aside }: FoldProps) {
  const id = useId();
  const box = useRef<HTMLDivElement>(null);
  const text = useRef<HTMLDivElement>(null);
  const closing = useRef(false);
  const [open, setOpen] = useState(false);
  const [size, setSize] = useState<Size | null>(null);
  const limit = lines * lineHeight;

  const measure = useCallback(() => {
    const next = sizeOf(text.current!, limit);
    setSize((size) => (size && size.full === next.full && size.cut === next.cut && size.edge === next.edge ? size : next));
  }, [limit]);
  // Measuring forces a layout, so only once, then when the width changes and
  // when the text does, never on a render that changed neither.
  useLayoutEffect(() => {
    measure();
    const edits = new MutationObserver(() => measure());
    edits.observe(text.current!, { childList: true, subtree: true, characterData: true });
    const resizes = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(() => measure());
    resizes?.observe(text.current!);
    return () => {
      edits.disconnect();
      resizes?.disconnect();
    };
  }, [measure]);

  const long = size === null || size.full > limit; // folded until measured, so nothing jumps
  const folded = long && !open;
  useLayoutEffect(() => takeHiddenOutOfTabOrder(box.current!));
  useLayoutEffect(() => {
    if (!closing.current) return;
    closing.current = false;
    scrollBack(box.current!);
  }, [open]);

  function toggle() {
    closing.current = open;
    setOpen(!open);
  }

  const n = size ? Math.round(size.full / lineHeight) : lines;
  const count = `${n} ${n === 1 ? "line" : "lines"}`;
  return (
    <div>
      <div data-fold-frame={className ? "" : undefined} className={cn(aside !== undefined && "relative", className)}>
        {aside}
        <div
          ref={box}
          id={id}
          data-fold-cut={folded ? (size?.cut ?? limit) : undefined}
          style={folded ? { maxHeight: (size?.cut ?? limit) + (size?.edge ? 1 : 0) } : undefined}
          // clip can't be scrolled, so focusing a link can't shift the lines; hidden where clip isn't known.
          className={cn("overflow-hidden supports-[overflow:clip]:overflow-clip", folded && size?.edge && "border-b border-divider")}
          onScroll={(event) => {
            event.currentTarget.scrollTop = 0;
          }}
        >
          <div ref={text}>{children}</div>
        </div>
      </div>
      {size && long && (
        <button
          type="button"
          aria-expanded={open}
          aria-controls={id}
          aria-label={open ? `Show less of this ${kind}` : `Show all ${count} of this ${kind}`}
          onClick={toggle}
          // A 44px box (8px above the text, 24px below) that overlaps the row's padding, so the row doesn't grow.
          className="-mb-6 block w-fit cursor-pointer pt-2 pb-6 text-[12px] leading-3 font-semibold text-ink-muted hover:underline"
        >
          {open ? "Show less" : `Show all · ${count}`}
        </button>
      )}
    </div>
  );
}

// sizeOf measures the text: its whole height, the cut, at the bottom of the
// last line box that ends above the limit, so no line is cut in half, and
// whether the cut passes through a frame inside.
function sizeOf(text: HTMLElement, limit: number): Size {
  const top = text.getBoundingClientRect().top;
  let cut = 0;
  const range = document.createRange();
  const walker = document.createTreeWalker(text, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    range.selectNodeContents(node);
    for (const line of range.getClientRects?.() ?? []) {
      const bottom = line.bottom - top;
      if (bottom <= limit && bottom > cut) cut = bottom;
    }
  }
  cut = Math.ceil(cut) || limit;
  const edge = [...text.querySelectorAll("[data-fold-frame]")].some((frame) => {
    const box = frame.getBoundingClientRect();
    return box.top - top < cut && box.bottom - top > cut + 1;
  });
  return { full: text.getBoundingClientRect().height, cut, edge };
}

// takeHiddenOutOfTabOrder gives every focusable element in a fold whose first
// line is cut off tabindex -1, keeping its own in data-fold-tabindex, and
// gives the rest theirs back.
function takeHiddenOutOfTabOrder(box: HTMLElement) {
  for (const element of box.querySelectorAll<HTMLElement>(FOCUSABLE)) {
    const saved = element.dataset.foldTabindex;
    if (cutOff(element)) {
      if (saved !== undefined) continue;
      element.dataset.foldTabindex = element.getAttribute("tabindex") ?? "";
      element.tabIndex = -1;
    } else if (saved !== undefined) {
      if (saved) element.setAttribute("tabindex", saved);
      else element.removeAttribute("tabindex");
      delete element.dataset.foldTabindex;
    }
  }
}

// cutOff says whether an element's first line box isn't wholly above the cut
// of a folded fold around it. A link that starts above the cut and wraps
// below it is still there to focus.
function cutOff(element: HTMLElement): boolean {
  const line = element.getClientRects()[0];
  if (!line) return false;
  for (
    let fold = element.parentElement?.closest<HTMLElement>("[data-fold-cut]");
    fold;
    fold = fold.parentElement?.closest("[data-fold-cut]")
  )
    if (line.bottom > fold.getBoundingClientRect().top + Number(fold.dataset.foldCut) + 0.5) return true;
  return false;
}

// scrollBack brings a folded entry's first line back under the stuck band
// when folding it left its top above the band. The scroller's
// scroll-padding-top is the band's height, so scrollIntoView lands under it.
function scrollBack(box: HTMLElement) {
  const scroller = scrollerOf(box); // null: the page itself, whose top is the window's
  const band = parseFloat(getComputedStyle(scroller ?? document.documentElement).scrollPaddingTop) || 0;
  const top = scroller ? scroller.getBoundingClientRect().top : 0;
  if (box.getBoundingClientRect().top < top + band) box.scrollIntoView({ block: "start" });
}

function scrollerOf(element: HTMLElement): HTMLElement | null {
  for (let parent = element.parentElement; parent && parent !== document.documentElement; parent = parent.parentElement)
    if (/(auto|scroll)/.test(getComputedStyle(parent).overflowY)) return parent;
  return null;
}

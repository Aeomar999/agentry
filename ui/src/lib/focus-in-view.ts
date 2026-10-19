import type { FocusEvent } from "react";

// keepFocusInView is a scroller's onFocus: it scrolls what Tab just focused
// wholly into view, below the stuck band or card header and above the
// bottom padding (Newest, on a phone), which the scroller's scroll-padding
// gives. Chromium does that from the padding alone; Firefox can leave part
// of a link or a button past the bottom edge. It scrolls the scroller, never
// a fold, whose visible lines stay put; it does nothing when the element is
// already in view, nor for a click, which must not jump the feed.
export function keepFocusInView(event: FocusEvent<HTMLElement>) {
  const scroller = event.currentTarget;
  const target = event.target;
  if (target === scroller || !target.matches(":focus-visible")) return;
  const style = getComputedStyle(scroller);
  const view = scroller.getBoundingClientRect();
  const box = target.getBoundingClientRect();
  const under = view.top + (parseFloat(style.scrollPaddingTop) || 0) - box.top;
  const below = box.bottom - view.bottom + (parseFloat(style.scrollPaddingBottom) || 0);
  // Taller than the room left, its top goes under the band and the rest runs on below.
  if (under > 0) scroller.scrollTop -= under;
  else if (below > 0) scroller.scrollTop += Math.min(below, -under);
}

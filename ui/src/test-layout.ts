import { afterEach, beforeEach, vi } from "vitest";

// jsdom lays nothing out, so tests that need geometry say where things are:
// an element with data-lines="0-20 20-40" has those line boxes (top-bottom,
// in px), as does the text in it; any other element's box is the span of
// the line boxes inside it. `layout.shift` moves everything up, as a scroll
// would. ResizeObserver and IntersectionObserver are stand-ins that never
// fire on their own: intersect() makes a watched element come near.

export const layout = { shift: 0 };

function rect(top: number, bottom: number): DOMRect {
  return new DOMRect(0, top - layout.shift, 100, bottom - top);
}

function linesOf(element: Element): DOMRect[] {
  const own = element.getAttribute("data-lines");
  if (own)
    return own.split(" ").map((line) => {
      const [top, bottom] = line.split("-").map(Number);
      return rect(top, bottom);
    });
  const inside = [...element.querySelectorAll("[data-lines]")].flatMap(linesOf);
  if (!inside.length) return [];
  return [rect(Math.min(...inside.map((r) => r.top + layout.shift)), Math.max(...inside.map((r) => r.bottom + layout.shift)))];
}

const watched: { callback: IntersectionObserverCallback; elements: Set<Element> }[] = [];

// intersect tells every observer watching an element that it has come near.
export function intersect(element: Element) {
  for (const { callback, elements } of watched)
    if (elements.has(element))
      callback([{ target: element, isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver);
}

class FakeResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

class FakeIntersectionObserver {
  private entry: (typeof watched)[number];
  constructor(callback: IntersectionObserverCallback) {
    this.entry = { callback, elements: new Set() };
    watched.push(this.entry);
  }
  observe(element: Element) {
    this.entry.elements.add(element);
  }
  unobserve(element: Element) {
    this.entry.elements.delete(element);
  }
  disconnect() {
    this.entry.elements.clear();
  }
}

// layOut installs the stand-ins for each test of the file that calls it.
// The observers stay: a module may keep one for good.
export function layOut() {
  globalThis.ResizeObserver = FakeResizeObserver as unknown as typeof ResizeObserver;
  globalThis.IntersectionObserver = FakeIntersectionObserver as unknown as typeof IntersectionObserver;
  const saved = {
    rects: Element.prototype.getClientRects,
    box: Element.prototype.getBoundingClientRect,
    range: Range.prototype.getClientRects,
    scroll: Element.prototype.scrollIntoView,
  };
  beforeEach(() => {
    Element.prototype.getClientRects = function (this: Element) {
      return linesOf(this) as unknown as DOMRectList;
    };
    Element.prototype.getBoundingClientRect = function (this: Element) {
      const lines = linesOf(this);
      return lines.length ? rect(lines[0].top + layout.shift, lines[lines.length - 1].bottom + layout.shift) : rect(0, 0);
    };
    Range.prototype.getClientRects = function (this: Range) {
      const parent = this.startContainer.parentElement?.closest("[data-lines]");
      return (parent ? linesOf(parent) : []) as unknown as DOMRectList;
    };
    Element.prototype.scrollIntoView = vi.fn();
  });
  afterEach(() => {
    Element.prototype.getClientRects = saved.rects;
    Element.prototype.getBoundingClientRect = saved.box;
    Range.prototype.getClientRects = saved.range;
    Element.prototype.scrollIntoView = saved.scroll;
    layout.shift = 0;
  });
}

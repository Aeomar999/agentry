import { Component, lazy, ReactNode, Suspense, useEffect, useRef, useState } from "react";

// Code as plain text at once, coloured by Monaco once it comes within 200px
// of the screen, so a page of commands loads no Monaco until one is near.
// The colour lands on the same text in the same font, so nothing moves, and
// it stays once it has come. If Monaco can't load, the code stays plain,
// with no message: it reads the same.
//
// The language goes to MonacoColorize as written: only it, in Monaco's own
// bundle, knows Monaco's names for languages.

let warned = false;
const MonacoColorize = lazy(() =>
  import("./MonacoColorize").catch((error) => {
    if (!warned) console.warn("Code stays plain: Monaco didn't load.", error);
    warned = true;
    return { default: ({ code }: { code: string; language: string }) => <>{code}</> };
  }),
);

export function LazyColorize({ code, language }: { code: string; language: string }) {
  const ref = useRef<HTMLSpanElement>(null);
  const [near, setNear] = useState(false);
  useEffect(() => {
    if (!language || near) return;
    return whenNear(ref.current!, () => setNear(true));
  }, [language, near]);
  return (
    <span ref={ref}>
      {near ? (
        <Plain code={code}>
          <Suspense fallback={code}>
            <MonacoColorize code={code} language={language} />
          </Suspense>
        </Plain>
      ) : (
        code
      )}
    </span>
  );
}

// One observer for every code box on the page.
let observer: IntersectionObserver | null = null;
const waiting = new Map<Element, () => void>();

// whenNear calls come once an element is within 200px of the screen, and
// returns the way to stop waiting.
function whenNear(element: Element, come: () => void): () => void {
  if (typeof IntersectionObserver === "undefined") {
    come();
    return () => {};
  }
  observer ??= new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        waiting.get(entry.target)?.();
        stop(entry.target);
      }
    },
    { rootMargin: "200px" },
  );
  waiting.set(element, come);
  observer.observe(element);
  return () => stop(element);
}

function stop(element: Element) {
  waiting.delete(element);
  observer?.unobserve(element);
}

// Plain keeps the plain text if colouring it fails.
class Plain extends Component<{ code: string; children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? this.props.code : this.props.children;
  }
}

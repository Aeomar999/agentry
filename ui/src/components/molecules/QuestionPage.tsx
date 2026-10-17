import { useEffect, useRef, useState } from "react";
import { Maximize2, X } from "lucide-react";
import { Button } from "@/components/atoms/Button";

// A page the coordinator wrote to ask you something, SVG diagrams and all,
// shown in a sandbox: its own origin (no allow-same-origin), so it can't
// read Agentry's token or pages, and a content policy that lets nothing load
// from the network. Its one way out is answer("text"), which hands the text
// to onAnswer for you to check and send: a page can't answer on its own.
//
// The page gets Agentry's colours as CSS variables and sensible styles for
// plain HTML, so it looks like the rest of the app in either theme. It is as
// tall as its content, up to most of the screen, and Expand shows it over
// the whole window.

const POLICY = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; script-src 'unsafe-inline'";

// What the page can use, by the names the coordinator is told (DecisionFormat).
const VARIABLES: Record<string, string> = {
  "--bg": "--color-surface-0",
  "--panel": "--color-surface-1",
  "--raised": "--color-surface-2",
  "--border": "--color-divider",
  "--text": "--color-ink-primary",
  "--text-2": "--color-ink-secondary",
  "--muted": "--color-ink-muted",
  "--accent": "--color-info",
  "--good": "--color-success",
  "--warn": "--color-warning",
  "--bad": "--color-danger",
  "--brand": "--color-brand",
  "--mono": "--font-mono",
  "--sans": "--font-sans",
};

function documentFor(page: string): string {
  const root = getComputedStyle(document.documentElement);
  const variables = Object.entries(VARIABLES)
    .map(([name, token]) => `${name}: ${root.getPropertyValue(token).trim()};`)
    .join(" ");
  return `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="${POLICY}">
<style>
  :root { ${variables} color-scheme: ${document.documentElement.classList.contains("light") ? "light" : "dark"}; }
  * { box-sizing: border-box; }
  body { margin: 0; padding: 24px; font: 14px/1.55 var(--sans); color: var(--text); background: var(--bg); }
  h1, h2, h3 { margin: 0 0 8px; line-height: 1.25; } h1 { font-size: 20px; } h2 { font-size: 16px; } h3 { font-size: 14px; }
  p { margin: 0 0 12px; color: var(--text-2); }
  code, pre { font-family: var(--mono); font-size: 12px; } pre { background: var(--panel); border: 1px solid var(--border); padding: 12px; overflow: auto; }
  table { border-collapse: collapse; width: 100%; } th, td { text-align: left; padding: 8px 10px; border-bottom: 1px solid var(--border); }
  th { color: var(--muted); font-weight: 600; font-size: 12px; }
  button { font: inherit; font-weight: 500; color: var(--text); background: var(--raised); border: 1px solid var(--border); border-radius: 3px; padding: 6px 12px; cursor: pointer; }
  button:hover { border-color: var(--muted); }
  svg { max-width: 100%; height: auto; }
</style>
<script>
  function answer(text) { parent.postMessage({ agentryAnswer: String(text) }, "*"); }
  new ResizeObserver(() => parent.postMessage({ agentryHeight: document.documentElement.scrollHeight }, "*")).observe(document.documentElement);
</script>
</head><body>${page}</body></html>`;
}

export function QuestionPage({ page, onAnswer }: { page: string; onAnswer: (text: string) => void }) {
  const [expanded, setExpanded] = useState(false);
  const answered = (text: string) => {
    setExpanded(false);
    onAnswer(text);
  };

  useEffect(() => {
    if (!expanded) return;
    const onKey = (event: KeyboardEvent) => event.key === "Escape" && setExpanded(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [expanded]);

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between text-[12px] text-ink-muted">
        <span>The coordinator made a page for this question.</span>
        <Button size="sm" variant="ghost" icon={<Maximize2 />} onClick={() => setExpanded(true)}>
          Expand
        </Button>
      </div>
      <Frame page={page} onAnswer={answered} fitContent />
      {expanded && (
        <div role="dialog" aria-label="The coordinator's page" className="fixed inset-0 z-50 flex flex-col bg-surface-0">
          <div className="flex h-11 shrink-0 items-center justify-between border-b border-divider px-4">
            <span className="text-[13px] font-semibold text-ink-primary">The coordinator's page</span>
            <Button size="sm" variant="ghost" icon={<X />} onClick={() => setExpanded(false)}>
              Close
            </Button>
          </div>
          <Frame page={page} onAnswer={answered} />
        </div>
      )}
    </div>
  );
}

// One sandboxed frame. With fitContent it takes the page's own height, from
// 240px to most of the screen; otherwise it fills the space it is given.
function Frame({ page, onAnswer, fitContent = false }: { page: string; onAnswer: (text: string) => void; fitContent?: boolean }) {
  const frame = useRef<HTMLIFrameElement>(null);
  const [height, setHeight] = useState(320);

  useEffect(() => {
    function onMessage(event: MessageEvent) {
      if (event.source !== frame.current?.contentWindow) return; // only this page's own frame
      const data = event.data as { agentryAnswer?: unknown; agentryHeight?: unknown };
      if (typeof data?.agentryAnswer === "string" && data.agentryAnswer.trim()) onAnswer(data.agentryAnswer.slice(0, 4000));
      if (typeof data?.agentryHeight === "number") setHeight(Math.max(240, Math.min(data.agentryHeight, window.innerHeight * 0.75)));
    }
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [onAnswer]);

  return (
    <iframe
      ref={frame}
      title="The coordinator's question"
      sandbox="allow-scripts"
      srcDoc={documentFor(page)}
      style={fitContent ? { height } : undefined}
      className={fitContent ? "w-full rounded-md border border-divider bg-surface-0" : "min-h-0 w-full flex-1 bg-surface-0"}
    />
  );
}

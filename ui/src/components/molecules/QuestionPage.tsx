import { useEffect, useRef, useState } from "react";
import { Maximize2, X } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { pageDocument } from "@/lib/sandbox";

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

// Its one script: answer(text), and telling the frame how tall it is.
const SCRIPT = `
  function answer(text) { parent.postMessage({ agentryAnswer: String(text) }, "*"); }
  new ResizeObserver(() => parent.postMessage({ agentryHeight: document.documentElement.scrollHeight }, "*")).observe(document.documentElement);
`;

function documentFor(page: string): string {
  return pageDocument(page, SCRIPT);
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

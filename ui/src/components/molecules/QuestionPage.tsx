import { useEffect, useRef } from "react";

// A page the coordinator wrote to ask you something, SVG diagrams and all,
// shown in a sandbox: its own origin (no allow-same-origin), so it can't
// read Agentry's token or pages, and a content policy that lets nothing load
// from the network. Its one way out is answer("text"), which hands the text
// to onAnswer for you to check and send: a page can't answer on its own.

const POLICY = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; script-src 'unsafe-inline'";

function token(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

function documentFor(page: string): string {
  return `<!doctype html><html><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="${POLICY}">
<style>
  html { color-scheme: ${document.documentElement.classList.contains("light") ? "light" : "dark"}; }
  body { margin: 16px; font: 14px/1.5 system-ui, sans-serif; color: ${token("--color-ink-primary")}; background: ${token("--color-surface-0")}; }
  svg { max-width: 100%; height: auto; }
</style>
<script>function answer(text) { parent.postMessage({ agentryAnswer: String(text) }, "*"); }</script>
</head><body>${page}</body></html>`;
}

export function QuestionPage({ page, onAnswer }: { page: string; onAnswer: (text: string) => void }) {
  const frame = useRef<HTMLIFrameElement>(null);

  useEffect(() => {
    function onMessage(event: MessageEvent) {
      if (event.source !== frame.current?.contentWindow) return; // only this page's own frame
      const text = (event.data as { agentryAnswer?: unknown })?.agentryAnswer;
      if (typeof text === "string" && text.trim()) onAnswer(text.slice(0, 4000));
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
      className="h-80 w-full rounded-md border border-divider bg-surface-0"
    />
  );
}

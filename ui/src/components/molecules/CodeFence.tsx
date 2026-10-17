import { lazy, Suspense, useState } from "react";
import { Check, Copy } from "lucide-react";

// A code block in a document: its language, a copy button, and the code
// highlighted as the editor would, once Monaco has loaded.
const MonacoColorize = lazy(() => import("./MonacoColorize"));

export function CodeFence({ code, language }: { code: string; language: string }) {
  const [copied, setCopied] = useState(false);
  function copy() {
    navigator.clipboard?.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }
  return (
    <div className="group relative rounded-md border border-divider bg-surface-1">
      <div className="flex h-8 items-center justify-between border-b border-divider px-3">
        <span className="font-mono text-[11px] text-ink-faint">{language || "text"}</span>
        <button
          type="button"
          onClick={copy}
          className="inline-flex cursor-pointer items-center gap-1 text-[11px] text-ink-muted hover:text-ink-primary [&>svg]:size-3.5"
        >
          {copied ? <Check className="text-success" /> : <Copy />}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className="scrollbar-thin overflow-x-auto px-4 py-3 font-mono text-[12.5px] leading-5 text-ink-primary">
        <Suspense fallback={code}>{language ? <MonacoColorize code={code} language={language} /> : code}</Suspense>
      </pre>
    </div>
  );
}

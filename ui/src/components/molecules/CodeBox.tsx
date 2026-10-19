import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";
import { Fold } from "./Fold";
import { LazyColorize } from "./LazyColorize";

// Code in an entry of a task's Activity or Handoffs: CodeFence's compact
// sibling. A command an agent ran wraps and folds at 3 lines; a fenced block
// keeps its layout, scrolls sideways, folds at 12 lines, and has its language
// and a Copy button at the top right. Both are coloured once on screen.
//
// It must not import lib/monaco, or Monaco would join the task page's
// bundle: the language goes on as written, and only MonacoColorize reads it.

const LINE = 20; // px, the box's line height (leading-5)

export function CodeBox({ code, language, command = false }: { code: string; language: string; command?: boolean }) {
  return (
    <Fold
      lines={command ? 3 : 12}
      lineHeight={LINE}
      kind={command ? "command" : "code"}
      className="rounded-md border border-divider bg-surface-1 px-2.5 py-2"
      aside={command ? undefined : <Corner code={code} language={language} />}
    >
      <pre
        className={
          command
            ? "font-mono text-[12px] leading-5 wrap-anywhere whitespace-pre-wrap text-ink-primary"
            : "scrollbar-thin overflow-x-auto pr-28 font-mono text-[12px] leading-5 text-ink-primary"
        }
      >
        <LazyColorize code={code} language={language} />
      </pre>
    </Fold>
  );
}

// Corner is a fence's language, as written, and its Copy button. Positioned
// with no z-index, so it is drawn over the code but under a stuck header.
function Corner({ code, language }: { code: string; language: string }) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false); // no clipboard here (plain http): the text can still be selected
    }
  }
  return (
    <div className="absolute top-1 right-1 flex items-center gap-2 bg-surface-1 pl-2">
      {language && <span className="max-w-[10ch] truncate font-mono text-[11px] text-ink-muted">{language.toLowerCase()}</span>}
      <IconButton
        icon={copied ? <Check className="text-success" /> : <Copy />}
        label={copied ? "Copied" : "Copy code"}
        onClick={copy}
        // A 28px button with a 44px hit area.
        className="relative size-7 after:absolute after:-inset-2 after:content-['']"
      />
      <span className="sr-only" aria-live="polite">
        {copied ? "Copied" : ""}
      </span>
    </div>
  );
}

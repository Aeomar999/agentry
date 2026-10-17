import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";

// One shell command to copy and run somewhere else.
export function CommandLine({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false); // no clipboard here (plain http): the text can still be selected
    }
  }
  return (
    <div className="flex items-start gap-1 rounded-md border border-divider bg-surface-0 py-1 pr-1 pl-3">
      <code className="min-w-0 flex-1 py-1.5 font-mono text-[12px] leading-5 break-all text-ink-primary select-all">{command}</code>
      <IconButton icon={copied ? <Check /> : <Copy />} label={copied ? "Copied" : "Copy"} onClick={copy} className="size-7" />
    </div>
  );
}

import { cn } from "@/lib/cn";

// Text an agent or a file wrote, shown as text, never as HTML.
export function CodeBlock({ text, className }: { text: string; className?: string }) {
  return (
    <pre
      className={cn(
        "scrollbar-thin overflow-auto rounded-md border border-divider bg-surface-1 px-3 py-2.5",
        "font-mono text-[12px] leading-5 break-words whitespace-pre-wrap text-ink-secondary",
        className,
      )}
    >
      {text}
    </pre>
  );
}

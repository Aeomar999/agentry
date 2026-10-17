import { useState } from "react";
import { cn } from "@/lib/cn";

// Long text, cut to a few lines until you ask for all of it.
export function Clamp({ text, lines, mono = false, className }: { text: string; lines: number; mono?: boolean; className?: string }) {
  const [all, setAll] = useState(false);
  const long = text.length > lines * 90 || text.split("\n").length > lines;
  return (
    <div className={className}>
      <p
        className={cn(
          "break-words whitespace-pre-wrap",
          mono ? "font-mono text-[12px] break-all text-ink-primary" : "text-[13px] leading-5 text-ink-secondary",
        )}
        style={all ? undefined : { display: "-webkit-box", WebkitLineClamp: lines, WebkitBoxOrient: "vertical", overflow: "hidden" }}
      >
        {text}
      </p>
      {long && (
        <button
          type="button"
          onClick={() => setAll((a) => !a)}
          className="mt-0.5 cursor-pointer text-[11px] text-ink-muted hover:text-ink-primary"
        >
          {all ? "Show less" : "Show all"}
        </button>
      )}
    </div>
  );
}

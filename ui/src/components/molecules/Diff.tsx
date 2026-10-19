import { cn } from "@/lib/cn";

// A unified diff as the server writes it, a line each: added lines green,
// removed ones red, the file names and hunk headers quiet.
export function Diff({ text, className }: { text: string; className?: string }) {
  const lines = text.replace(/\n$/, "").split("\n");
  return (
    <pre className={cn("overflow-x-auto bg-surface-1 py-2 font-mono text-[12px] leading-5", className)}>
      {lines.map((line, i) => (
        <span key={i} className={cn("block min-w-max px-4 whitespace-pre", toneOf(line, i < 2))}>
          {line || " "}
        </span>
      ))}
    </pre>
  );
}

// The file names are the "--- " and "+++ " lines at the top; further down,
// a removed "--x" or added "++x" line is just a change.
function toneOf(line: string, atTop: boolean): string {
  if (atTop && (line.startsWith("--- ") || line.startsWith("+++ "))) return "text-ink-muted";
  if (line.startsWith("@@")) return "text-info";
  if (line.startsWith("+")) return "bg-success-soft text-success";
  if (line.startsWith("-")) return "bg-danger-soft text-danger";
  return "text-ink-secondary";
}

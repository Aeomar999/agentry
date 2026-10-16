import { useAgentColor } from "@/live/agent-colors";
import { cn } from "@/lib/cn";

// An agent's name with its colour: a dot, and the name in mono.
export function AgentName({ name, className, dotOnly = false }: { name: string; className?: string; dotOnly?: boolean }) {
  const colorOf = useAgentColor();
  return (
    <span className={cn("inline-flex min-w-0 items-center gap-1.5", className)}>
      <span className="size-2 shrink-0 rounded-full" style={{ background: colorOf(name) }} aria-hidden />
      {!dotOnly && <span className="truncate font-mono">{name}</span>}
    </span>
  );
}

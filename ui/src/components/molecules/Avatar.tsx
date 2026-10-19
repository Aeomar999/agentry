import { useAgentColor } from "@/live/agent-colors";
import { cn } from "@/lib/cn";

// "you" and Agentry's own loop are people and plumbing, not agents: their
// chips are neutral, never an agent's colour.
const NEUTRAL = new Set(["you", "Agentry"]);

// An author's avatar: the first letter of its name on a tint of its colour.
// The name is always beside it in text, so it is hidden from screen readers.
export function Avatar({ name, size = 28 }: { name: string; size?: 20 | 28 }) {
  const colorOf = useAgentColor();
  const neutral = NEUTRAL.has(name);
  const color = colorOf(name);
  return (
    <span
      aria-hidden
      className={cn(
        "flex shrink-0 items-center justify-center rounded-md font-mono font-bold",
        size === 28 ? "size-7 text-[12px]" : "size-5 text-[11px]",
        neutral && "bg-surface-3 text-ink-secondary",
      )}
      style={
        neutral
          ? undefined
          : {
              color: `color-mix(in srgb, ${color} var(--chip-letter), var(--color-ink-primary))`,
              background: `color-mix(in srgb, ${color} var(--chip-tint), transparent)`,
            }
      }
    >
      {name.slice(0, 1).toUpperCase()}
    </span>
  );
}

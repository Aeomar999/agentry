import { AGENT_COLORS } from "@/live/agent-colors";
import { cn } from "@/lib/cn";

// The agent colours as a radio group of swatches, each named by its colour.
// labelledBy names the element that labels the group, such as a Field's label.
interface ColorSwatchesProps {
  value: string;
  onChange: (color: string) => void;
  labelledBy?: string;
  disabled?: boolean; // aria-disabled, so the choice stays readable and focusable
}

export function ColorSwatches({ value, onChange, labelledBy, disabled }: ColorSwatchesProps) {
  return (
    <div role="radiogroup" aria-label={labelledBy ? undefined : "Colour"} aria-labelledby={labelledBy} className="flex flex-wrap gap-1.5">
      {AGENT_COLORS.map((color) => (
        <button
          key={color}
          type="button"
          role="radio"
          aria-checked={value === color}
          aria-label={color}
          aria-disabled={disabled || undefined}
          title={color}
          onClick={() => !disabled && onChange(color)}
          className={cn(
            "size-6 cursor-pointer rounded-sm border",
            value === color ? "border-ink-primary" : "border-transparent hover:border-divider-2",
            disabled && "cursor-not-allowed",
          )}
        >
          <span className="mx-auto block size-3.5 rounded-full" style={{ background: `var(--color-agent-${color})` }} />
        </button>
      ))}
    </div>
  );
}

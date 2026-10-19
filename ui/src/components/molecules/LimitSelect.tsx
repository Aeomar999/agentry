import { useState } from "react";
import { Input } from "@/components/atoms/Input";
import { Select } from "@/components/atoms/Select";
import { cn } from "@/lib/cn";
import { CUSTOM, LimitChoice, LimitScale, UNLIMITED, limitOf } from "@/lib/limits";

interface LimitSelectProps {
  id?: string;
  scale: LimitScale;
  choice: LimitChoice;
  onChange: (choice: LimitChoice) => void;
  defaultLimit?: number; // the server's, marked in the list
  autoFocus?: boolean; // the list takes the focus, as when an editor opens on it
  className?: string;
}

// A limit's presets, Unlimited and Custom, which opens a field for your own
// number; a custom value that isn't one says so under it. The field takes
// the focus only when you pick Custom, not when a form opens on it.
export function LimitSelect({ id, scale, choice, onChange, defaultLimit, autoFocus, className }: LimitSelectProps) {
  const custom = choice.option === CUSTOM;
  const [picked, setPicked] = useState(false);
  const invalid = custom && choice.custom.trim() !== "" && Number.isNaN(limitOf(scale, choice));
  const label = (limit: number) => scale.show(limit) + (limit === defaultLimit ? " (default)" : "");
  return (
    <div className={cn("flex flex-col gap-1", className)}>
      <div className="flex flex-wrap items-center gap-2">
        <Select
          id={id}
          aria-label={scale.name}
          autoFocus={autoFocus}
          value={choice.option}
          onChange={(e) => {
            setPicked(e.target.value === CUSTOM);
            onChange({ ...choice, option: e.target.value });
          }}
          className="w-32"
        >
          {scale.presets.map((limit) => (
            <option key={limit} value={String(limit)}>
              {label(limit)}
            </option>
          ))}
          <option value={UNLIMITED}>Unlimited{defaultLimit === 0 ? " (default)" : ""}</option>
          <option value={CUSTOM}>Custom</option>
        </Select>
        {custom && (
          <Input
            mono
            autoFocus={picked}
            aria-label={`${scale.name}, custom`}
            aria-invalid={invalid}
            value={choice.custom}
            onChange={(e) => onChange({ ...choice, custom: e.target.value })}
            placeholder={scale.show(scale.presets[1])}
            className={cn("w-24", invalid && "border-danger")}
          />
        )}
      </div>
      {invalid && <p className="text-[11px] text-danger">{scale.example}</p>}
    </div>
  );
}

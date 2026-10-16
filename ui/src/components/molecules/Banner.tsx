import { ReactNode } from "react";
import { CircleAlert, Info, TriangleAlert } from "lucide-react";
import { cn } from "@/lib/cn";

type Tone = "danger" | "warning" | "info";

const TONES: Record<Tone, string> = {
  danger: "border-danger/40 bg-danger-soft text-danger",
  warning: "border-warning/40 bg-warning-soft text-warning",
  info: "border-info/40 bg-info-soft text-info",
};

const ICONS = { danger: CircleAlert, warning: TriangleAlert, info: Info };

export function Banner({ tone = "danger", title, children }: { tone?: Tone; title: string; children?: ReactNode }) {
  const Icon = ICONS[tone];
  return (
    <div role="alert" className={cn("flex gap-2.5 rounded-md border px-3 py-2.5", TONES[tone])}>
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden />
      <div className="min-w-0 text-[13px]">
        <p className="font-medium">{title}</p>
        {children && <div className="mt-1 text-ink-secondary">{children}</div>}
      </div>
    </div>
  );
}

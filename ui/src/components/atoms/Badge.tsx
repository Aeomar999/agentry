import { ReactNode } from "react";
import { cn } from "@/lib/cn";

export type BadgeVariant = "default" | "success" | "danger" | "warning" | "info" | "brand";

const VARIANTS: Record<BadgeVariant, string> = {
  default: "bg-surface-3 text-ink-secondary",
  success: "bg-success-soft text-success-ink",
  danger: "bg-danger-soft text-danger-ink",
  warning: "bg-warning-soft text-warning-ink",
  info: "bg-info-soft text-info-ink",
  brand: "bg-brand-soft text-brand-ink",
};

// A fixed 18px height, so rows don't jitter between values.
export function Badge({ variant = "default", className, children }: { variant?: BadgeVariant; className?: string; children: ReactNode }) {
  return (
    <span
      className={cn(
        "inline-flex h-[18px] shrink-0 items-center rounded-sm px-1.5 font-mono text-[10px] font-medium tracking-wider whitespace-nowrap uppercase",
        VARIANTS[variant],
        className,
      )}
    >
      {children}
    </span>
  );
}

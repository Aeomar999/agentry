import { cn } from "@/lib/cn";

export type DotTone = "success" | "warning" | "danger" | "info" | "muted";

const TONES: Record<DotTone, string> = {
  success: "bg-success",
  warning: "bg-warning",
  danger: "bg-danger",
  info: "bg-info",
  muted: "bg-ink-faint",
};

export function StatusDot({ tone, className }: { tone: DotTone; className?: string }) {
  return <span className={cn("inline-block size-2 shrink-0 rounded-full", TONES[tone], className)} aria-hidden />;
}

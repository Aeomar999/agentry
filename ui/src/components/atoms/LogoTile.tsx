import { cn } from "@/lib/cn";

// Agentry's mark: a hub with three spokes, the coordinator and its agents.
export function LogoTile({ className }: { className?: string }) {
  return (
    <span className={cn("inline-flex size-7 shrink-0 items-center justify-center rounded-md bg-brand-soft text-brand", className)}>
      <svg viewBox="0 0 24 24" className="size-4" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" aria-hidden>
        <circle cx="12" cy="12" r="3" fill="currentColor" stroke="none" />
        <path d="M12 9V3.5M9.4 13.5 4.6 16.3M14.6 13.5l4.8 2.8" />
        <circle cx="12" cy="3.5" r="1.6" fill="currentColor" stroke="none" />
        <circle cx="4.6" cy="16.3" r="1.6" fill="currentColor" stroke="none" />
        <circle cx="19.4" cy="16.3" r="1.6" fill="currentColor" stroke="none" />
      </svg>
    </span>
  );
}

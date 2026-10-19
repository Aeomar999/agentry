import { StatusDot } from "@/components/atoms/StatusDot";
import { cn } from "@/lib/cn";

export interface Tab {
  id: string;
  label: string;
  count?: number;
  live?: boolean; // something is running in it: a pulsing dot
}

export function Tabs({ tabs, active, onChange }: { tabs: Tab[]; active: string; onChange: (id: string) => void }) {
  return (
    <div role="tablist" className="flex scrollbar-thin gap-1 overflow-x-auto border-b border-divider px-6">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          role="tab"
          aria-selected={tab.id === active}
          onClick={() => onChange(tab.id)}
          className={cn(
            // The focus line goes inside: the strip scrolls, so it would clip one outside.
            "-mb-px inline-flex h-10 cursor-pointer items-center gap-2 border-b-2 px-2 text-[13px] whitespace-nowrap focus-visible:-outline-offset-2",
            "transition-colors duration-(--dur-fast)",
            tab.id === active ? "border-brand font-medium text-ink-primary" : "border-transparent text-ink-muted hover:text-ink-primary",
          )}
        >
          {tab.live && <StatusDot tone="success" />}
          {tab.label}
          {tab.count !== undefined && (
            <span className="rounded-sm bg-surface-3 px-1.5 font-mono text-[10px] leading-[16px] text-ink-secondary">{tab.count}</span>
          )}
        </button>
      ))}
    </div>
  );
}

import { ReactNode } from "react";

export function EmptyState({ icon, title, children }: { icon?: ReactNode; title: string; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-6 py-14 text-center">
      {icon && <div className="mb-1 text-ink-faint [&>svg]:size-6">{icon}</div>}
      <p className="text-[13px] font-medium text-ink-primary">{title}</p>
      {children && <div className="max-w-md text-[12px] text-ink-muted">{children}</div>}
    </div>
  );
}

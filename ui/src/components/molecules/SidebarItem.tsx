import { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { cn } from "@/lib/cn";

interface SidebarItemProps {
  to: string;
  label: string;
  icon: ReactNode;
  count?: number; // pending work, shown as a badge
  collapsed: boolean;
}

export function SidebarItem({ to, label, icon, count, collapsed }: SidebarItemProps) {
  return (
    <NavLink
      to={to}
      title={collapsed ? label : undefined}
      className={({ isActive }) =>
        cn(
          "flex h-row-nav items-center gap-2.5 rounded-md px-2.5 text-[13px] transition-colors duration-(--dur-fast) [&>svg]:size-4 [&>svg]:shrink-0",
          isActive ? "bg-surface-3 font-medium text-ink-primary" : "text-ink-secondary hover:bg-surface-2 hover:text-ink-primary",
          collapsed && "justify-center px-0",
        )
      }
    >
      {icon}
      {!collapsed && <span className="flex-1 truncate">{label}</span>}
      {!collapsed && count !== undefined && count > 0 && (
        <span className="rounded-sm bg-warning-soft px-1.5 font-mono text-[10px] leading-[16px] text-warning">{count}</span>
      )}
    </NavLink>
  );
}

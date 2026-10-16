import { Link } from "react-router-dom";
import { Bell, Moon, PanelLeft, Search, Sun } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";
import { Kbd } from "@/components/atoms/Kbd";
import { LogoTile } from "@/components/atoms/LogoTile";
import { Breadcrumbs, Crumb } from "@/components/molecules/Breadcrumbs";
import type { Theme } from "@/lib/theme";
import { cn } from "@/lib/cn";
import { modKey } from "@/lib/shortcuts";

interface TopBarProps {
  crumbs: Crumb[];
  needsYou: number;
  theme: Theme;
  onToggleTheme: () => void;
  onToggleSidebar: () => void;
  onOpenPalette: () => void;
}

export function TopBar({ crumbs, needsYou, theme, onToggleTheme, onToggleSidebar, onOpenPalette }: TopBarProps) {
  return (
    <header className="relative z-20 flex h-topbar shrink-0 items-center gap-3 border-b border-divider bg-surface-1 px-3">
      <IconButton icon={<PanelLeft />} label="Show or hide the sidebar" onClick={onToggleSidebar} />
      <Link to="/tasks" className="flex shrink-0 items-center gap-2">
        <LogoTile />
        <span className="font-display text-[16px] font-bold tracking-tight text-ink-primary [font-stretch:85%] max-sm:hidden">Agentry</span>
      </Link>
      <span className="h-5 w-px shrink-0 bg-divider max-sm:hidden" />
      <div className="min-w-0 flex-1 max-sm:hidden">
        <Breadcrumbs crumbs={crumbs} />
      </div>
      <div className="flex-1 sm:hidden" />

      <button
        type="button"
        onClick={onOpenPalette}
        className={cn(
          "flex h-8 w-60 cursor-pointer items-center gap-2 rounded-md border border-divider bg-surface-0 px-2.5 text-[12px] text-ink-faint",
          "transition-colors duration-(--dur-fast) hover:border-divider-2 max-md:w-8 max-md:justify-center max-md:px-0",
        )}
      >
        <Search className="size-3.5 shrink-0" aria-hidden />
        <span className="flex-1 text-left max-md:hidden">Search or jump to</span>
        <Kbd className="max-md:hidden">{modKey} K</Kbd>
      </button>

      <IconButton
        icon={theme === "dark" ? <Sun /> : <Moon />}
        label={theme === "dark" ? "Light theme" : "Dark theme"}
        onClick={onToggleTheme}
      />
      <Link
        to="/tasks?status=needs-you"
        title={needsYou ? `${needsYou} waiting for you` : "Nothing waiting for you"}
        className="relative inline-flex size-8 items-center justify-center rounded-md text-ink-muted hover:bg-surface-2 hover:text-ink-primary"
      >
        <Bell className="size-4" aria-hidden />
        {needsYou > 0 && (
          <span className="absolute top-1 right-1 flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-brand px-1 font-mono text-[9px] leading-none text-white">
            {needsYou}
          </span>
        )}
      </Link>
    </header>
  );
}

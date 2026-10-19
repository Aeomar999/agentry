import { Bot, ListTodo, Map as MapIcon, Plug, Server, ShieldCheck } from "lucide-react";
import { SidebarItem } from "@/components/molecules/SidebarItem";
import { cn } from "@/lib/cn";
import { Features, visibleFeatures } from "@/features";

interface SidebarProps {
  collapsed: boolean; // a 56px icon rail, on wide screens
  drawerOpen: boolean; // on narrow screens the sidebar is a drawer over the page
  needsYou: number;
  features: Features; // which features show, each adding its items to its section
  onCloseDrawer: () => void;
}

const SECTIONS = [
  {
    section: "work",
    title: "Work",
    items: [
      { to: "/tasks", label: "Tasks", icon: <ListTodo />, countsNeedsYou: true },
      { to: "/plan", label: "Plan", icon: <MapIcon /> },
    ],
  },
  {
    section: "setup",
    title: "Setup",
    items: [
      { to: "/agents", label: "Agents", icon: <Bot /> },
      { to: "/workers", label: "Workers", icon: <Server /> },
      { to: "/integrations", label: "Integrations", icon: <Plug /> },
      { to: "/policy", label: "Policy", icon: <ShieldCheck /> },
    ],
  },
];

export function Sidebar({ collapsed, drawerOpen, needsYou, features, onCloseDrawer }: SidebarProps) {
  const rail = collapsed && !drawerOpen;
  const shown = visibleFeatures(features);
  return (
    <>
      {drawerOpen && <div className="fixed inset-0 top-topbar z-20 bg-black/50 md:hidden" onClick={onCloseDrawer} />}
      <nav
        aria-label="Main"
        onClick={(event) => (event.target as HTMLElement).closest("a") && onCloseDrawer()}
        className={cn(
          "flex shrink-0 scrollbar-thin flex-col gap-4 overflow-y-auto border-r border-divider bg-surface-1 p-2 pt-3",
          "transition-[width] duration-(--dur-base) ease-(--ease-snap)",
          rail ? "w-sidebar-collapsed" : "w-sidebar",
          drawerOpen ? "max-md:fixed max-md:top-topbar max-md:bottom-0 max-md:left-0 max-md:z-30" : "max-md:hidden",
        )}
      >
        {SECTIONS.map((section) => (
          <div key={section.title} className="flex flex-col gap-0.5">
            {!rail && <p className="px-2.5 pb-1 text-[10.5px] font-semibold tracking-wider text-ink-faint uppercase">{section.title}</p>}
            {section.items.map((item) => (
              <SidebarItem
                key={item.to}
                to={item.to}
                label={item.label}
                icon={item.icon}
                collapsed={rail}
                count={"countsNeedsYou" in item ? needsYou : undefined}
              />
            ))}
            {shown.flatMap((feature) =>
              (feature.nav ?? [])
                .filter((item) => item.section === section.section)
                .map((item) => (
                  <SidebarItem
                    key={item.to}
                    to={item.to}
                    label={item.label}
                    icon={item.icon}
                    collapsed={rail}
                    count={features.badge(feature.name)}
                  />
                )),
            )}
          </div>
        ))}
      </nav>
    </>
  );
}

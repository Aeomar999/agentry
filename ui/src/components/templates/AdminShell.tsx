import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { Outlet, useMatch, useNavigate } from "react-router-dom";
import { Bot, KeyRound, ListTodo, Moon, Plus, ShieldCheck, Sparkles, SquareTerminal, Sun } from "lucide-react";
import { CommandPalette, PaletteItem } from "@/components/organisms/CommandPalette";
import { ShortcutOverlay } from "@/components/organisms/ShortcutOverlay";
import { Sidebar } from "@/components/organisms/Sidebar";
import { StatusBar } from "@/components/organisms/StatusBar";
import { TopBar } from "@/components/organisms/TopBar";
import { DESK, TerminalDock } from "@/components/organisms/TerminalDock";
import { EmptyState } from "@/components/molecules/EmptyState";
import type { Crumb } from "@/components/molecules/Breadcrumbs";
import type { Agent, ServerConfig, TaskSummary } from "@/api/types";
import { useLive } from "@/live/LiveProvider";
import { useResource } from "@/live/use-resource";
import { applyTheme, savedTheme, Theme } from "@/lib/theme";
import { useKeys } from "@/lib/shortcuts";
import { readStored, writeStored } from "@/lib/stored";

// The one template: top bar, sidebar, status bar, the terminal dock, the
// palette and shortcuts around every page. It stays mounted while pages
// change, and a page only supplies its breadcrumb (with useBreadcrumb) and
// its content. A page can open the dock on a task with useTerminal.

const CrumbContext = createContext<(crumbs: Crumb[]) => void>(() => {});

export function useBreadcrumb(...crumbs: Crumb[]): void {
  const setCrumbs = useContext(CrumbContext);
  const key = JSON.stringify(crumbs);
  useEffect(() => setCrumbs(JSON.parse(key)), [key, setCrumbs]);
}

const TerminalContext = createContext<(taskId?: string) => void>(() => {});

// useTerminal opens the dock, on the task's terminal when given one.
export function useTerminal(): (taskId?: string) => void {
  return useContext(TerminalContext);
}

const sidebarKey = "agentry-sidebar-collapsed";
const dockKey = "agentry-terminal-height";
const minDockHeight = 120;

export function AdminShell() {
  const navigate = useNavigate();
  const { connection, position } = useLive();
  const tasks = useResource<TaskSummary[]>("/api/tasks").data ?? [];
  const agents = useResource<Agent[]>("/api/agents").data ?? [];
  const config = useResource<ServerConfig>("/api/config").data;

  const [crumbs, setCrumbs] = useState<Crumb[]>([]);
  const [theme, setTheme] = useState<Theme>(savedTheme);
  const [collapsed, setCollapsed] = useState(() => readStored(sidebarKey) === "true");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [palette, setPalette] = useState(false);
  const [overlay, setOverlay] = useState(false);

  // The dock: open or not, its height, and which tab. On a task's page it
  // opens on that task's terminal.
  const column = useRef<HTMLDivElement>(null);
  const [dockOpen, setDockOpen] = useState(false);
  const [dockHeight, setDockHeight] = useState(() => Number(readStored(dockKey)) || 320);
  const [dockTab, setDockTab] = useState(DESK);
  const routeTask = useMatch("/tasks/:taskId")?.params.taskId;
  const resizeDock = useCallback((height: number) => {
    const most = column.current?.clientHeight ?? height; // up to the whole content area, never over the top bar
    setDockHeight(Math.round(Math.max(minDockHeight, Math.min(most, height))));
  }, []);
  const openTerminal = useCallback(
    (taskId?: string) => {
      setDockTab(taskId ?? routeTask ?? DESK);
      setDockOpen(true);
    },
    [routeTask],
  );

  useEffect(() => applyTheme(theme), [theme]);
  useEffect(() => writeStored(sidebarKey, String(collapsed)), [collapsed]);
  useEffect(() => writeStored(dockKey, String(dockHeight)), [dockHeight]);
  useEffect(() => {
    document.title = crumbs.length ? `${crumbs[crumbs.length - 1].label} · Agentry` : "Agentry";
  }, [crumbs]);

  const toggleTheme = useCallback(() => setTheme((t) => (t === "dark" ? "light" : "dark")), []);

  // Ctrl+K and Ctrl+J work from anywhere, even a field or a terminal, so
  // they are caught before the focused element sees them. Every other key
  // is in useKeys.
  const toggleDock = useRef(() => {});
  toggleDock.current = () => (dockOpen ? setDockOpen(false) : openTerminal());
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (!(event.metaKey || event.ctrlKey)) return;
      const key = event.key.toLowerCase();
      if (key !== "k" && key !== "j") return;
      event.preventDefault();
      event.stopPropagation();
      if (key === "k") setPalette((open) => !open);
      else toggleDock.current();
    }
    window.addEventListener("keydown", onKey, { capture: true });
    return () => window.removeEventListener("keydown", onKey, { capture: true });
  }, []);

  useKeys({
    "?": () => setOverlay(true),
    n: () => navigate("/tasks/new"),
    "/": () => document.querySelector<HTMLInputElement>("[data-page-search] input")?.focus(),
    "g t": () => navigate("/tasks"),
    "g a": () => navigate("/agents"),
    "g p": () => navigate("/policy"),
  });

  const paletteItems = useMemo<PaletteItem[]>(
    () => [
      { id: "go-tasks", group: "Go to", label: "Tasks", icon: <ListTodo />, run: () => navigate("/tasks") },
      { id: "go-agents", group: "Go to", label: "Agents", icon: <Bot />, run: () => navigate("/agents") },
      { id: "go-policy", group: "Go to", label: "Policy", icon: <ShieldCheck />, run: () => navigate("/policy") },
      { id: "new-task", group: "Commands", label: "New task", icon: <Plus />, run: () => navigate("/tasks/new") },
      { id: "desk", group: "Commands", label: "Ask Claude to start a task", icon: <Sparkles />, run: () => openTerminal(DESK) },
      ...tasks
        .filter((task) => task.status === "working" && task.currentAgent)
        .map((task) => ({
          id: `terminal-${task.id}`,
          group: "Commands",
          label: `Open the terminal of ${task.id}`,
          hint: task.currentAgent,
          icon: <SquareTerminal />,
          run: () => openTerminal(task.id),
        })),
      {
        id: "theme",
        group: "Commands",
        label: theme === "dark" ? "Switch to the light theme" : "Switch to the dark theme",
        icon: theme === "dark" ? <Sun /> : <Moon />,
        run: toggleTheme,
      },
      ...tasks.map((task) => ({
        id: `task-${task.id}`,
        group: "Tasks",
        label: task.title,
        hint: `${task.id} · ${task.status}`,
        icon: <ListTodo />,
        run: () => navigate(`/tasks/${task.id}`),
      })),
      ...agents.map((agent) => ({
        id: `agent-${agent.name}`,
        group: "Agents",
        label: agent.name,
        hint: agent.coordinates ? "coordinator" : agent.engine,
        icon: <Bot />,
        run: () => navigate(`/agents/${encodeURIComponent(agent.name)}`),
      })),
    ],
    [tasks, agents, theme, navigate, toggleTheme, openTerminal],
  );

  const needsYou = tasks.filter((task) => task.status === "needs you").length;
  const working = tasks.filter((task) => task.status !== "done" && task.currentAgent);
  return (
    <CrumbContext.Provider value={setCrumbs}>
      <TerminalContext.Provider value={openTerminal}>
        <div className="flex h-full flex-col">
          <TopBar
            crumbs={crumbs}
            needsYou={needsYou}
            theme={theme}
            onToggleTheme={toggleTheme}
            onToggleSidebar={() => (matchMedia("(min-width: 768px)").matches ? setCollapsed((c) => !c) : setDrawerOpen((open) => !open))}
            onOpenPalette={() => setPalette(true)}
          />
          <div className="flex min-h-0 flex-1">
            <Sidebar collapsed={collapsed} drawerOpen={drawerOpen} needsYou={needsYou} onCloseDrawer={() => setDrawerOpen(false)} />
            <div ref={column} className="flex min-w-0 flex-1 flex-col">
              <main className="min-h-0 flex-1 scrollbar-thin overflow-y-auto">
                {connection === "signed out" ? (
                  <EmptyState icon={<KeyRound />} title="Not signed in">
                    Open the link <span className="font-mono">agentry serve</span> printed. Its token signs this browser in.
                  </EmptyState>
                ) : (
                  <Outlet />
                )}
              </main>
              {dockOpen && (
                <TerminalDock
                  height={dockHeight}
                  onResize={resizeDock}
                  working={working}
                  selected={dockTab}
                  onSelect={setDockTab}
                  onClose={() => setDockOpen(false)}
                />
              )}
            </div>
          </div>
          <StatusBar
            connection={connection}
            position={position}
            defaultRepo={config?.defaultRepo ?? ""}
            working={working.length}
            terminalOpen={dockOpen}
            onToggleTerminal={() => toggleDock.current()}
          />
        </div>
        <CommandPalette open={palette} items={paletteItems} onClose={() => setPalette(false)} />
        <ShortcutOverlay open={overlay} onClose={() => setOverlay(false)} />
      </TerminalContext.Provider>
    </CrumbContext.Provider>
  );
}

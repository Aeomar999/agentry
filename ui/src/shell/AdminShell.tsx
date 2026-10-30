import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { NewTaskDialog } from "@/domains/tasks/NewTaskDialog";
import { useNewTask } from "@/lib/new-task";
import { Outlet, useNavigate } from "react-router-dom";
import { Bot, ListTodo, Map as MapIcon, Moon, Plug, Plus, Settings, ShieldCheck, Sparkles, SquareTerminal, Sun } from "lucide-react";
import { CommandPalette, PaletteItem } from "@/shell/CommandPalette";
import { ShortcutOverlay } from "@/shell/ShortcutOverlay";
import { Sidebar } from "@/shell/Sidebar";
import { StatusBar } from "@/shell/StatusBar";
import { TopBar } from "@/shell/TopBar";
import { DESK, DockTab, QUESTIONS, TerminalDock } from "@/components/organisms/TerminalDock";
import type { Crumb } from "@/components/molecules/Breadcrumbs";
import type { Agent, Question, ServerConfig, TaskSummary } from "@/api/types";
import { useLive } from "@/live/LiveProvider";
import { useResource } from "@/live/use-resource";
import { AgentColorsProvider } from "@/live/agent-colors";
import { TaskTitlesProvider } from "@/live/refs";
import { applyTheme, savedTheme, Theme } from "@/lib/theme";
import { useKeys } from "@/lib/shortcuts";
import { CrumbContext } from "@/lib/breadcrumb";
import { QuestionContext } from "@/shell/questions";
import { readStored, writeStored } from "@/lib/stored";
import { shownId } from "@/lib/shown-id";
import { useExtensions, visibleExtensions } from "@/extensions";

// The one template: top bar, sidebar, status bar, the terminal dock, the
// palette and shortcuts around every page. It stays mounted while pages
// change, and a page only supplies its breadcrumb (with useBreadcrumb) and
// its content. An agent's terminal is on its task's page, not in the dock.
// A page opens the dock on one question with useOpenQuestion.

const sidebarKey = "agentry-sidebar-collapsed";
const dockKey = "agentry-terminal-height";
const minDockHeight = 120;

export function AdminShell() {
  const navigate = useNavigate();
  const { connection, position } = useLive();
  const tasks = useResource<TaskSummary[]>("/api/tasks").data ?? [];
  const questions = useResource<Question[]>("/api/questions").data ?? [];
  const agents = useResource<Agent[]>("/api/agents").data ?? [];
  const config = useResource<ServerConfig>("/api/config").data;
  const extensions = useExtensions();

  const [crumbs, setCrumbs] = useState<Crumb[]>([]);
  const [theme, setTheme] = useState<Theme>(savedTheme);
  const [collapsed, setCollapsed] = useState(() => readStored(sidebarKey) === "true");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [palette, setPalette] = useState(false);
  const [overlay, setOverlay] = useState(false);

  // The dock: open or not, its height, and which tab: Questions or Claude.
  const column = useRef<HTMLDivElement>(null);
  const [dockOpen, setDockOpen] = useState(false);
  const [dockHeight, setDockHeight] = useState(() => Number(readStored(dockKey)) || 320);
  const [dockTab, setDockTab] = useState<DockTab>(DESK);
  const resizeDock = useCallback((height: number) => {
    const most = column.current?.clientHeight ?? height; // up to the whole content area, never over the top bar
    setDockHeight(Math.round(Math.max(minDockHeight, Math.min(most, height))));
  }, []);
  // Opened without a tab, the dock opens on Questions whenever something
  // is waiting for you, and otherwise where you left it.
  const waiting = questions.length;
  const openDock = useCallback(
    (tab?: DockTab) => {
      if (tab) setDockTab(tab);
      else if (waiting > 0) setDockTab(QUESTIONS);
      setDockOpen(true);
    },
    [waiting],
  );
  const openTaskTerminal = useCallback((taskId: string) => navigate(`/tasks/${taskId}?tab=terminal`), [navigate]);
  const [dockQuestion, setDockQuestion] = useState({ id: "", asked: 0 }); // asked counts, so the same one opens again
  const openQuestion = useCallback((id: string) => {
    setDockQuestion((last) => ({ id, asked: last.asked + 1 }));
    setDockTab(QUESTIONS);
    setDockHeight((height) => Math.max(height, Math.round((column.current?.clientHeight ?? 800) * 0.6)));
    setDockOpen(true);
  }, []);

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
  toggleDock.current = () => (dockOpen ? setDockOpen(false) : openDock());
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

  const newTask = useNewTask();
  useKeys({
    "?": () => setOverlay(true),
    n: () => newTask(),
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
      { id: "new-task", group: "Commands", label: "New task", icon: <Plus />, run: () => newTask() },
      { id: "new-epic", group: "Commands", label: "New epic", icon: <Plus />, run: () => newTask("epic") },
      { id: "go-plan", group: "Go to", label: "Plan", icon: <MapIcon />, run: () => navigate("/plan") },
      ...visibleExtensions(extensions).flatMap((extension) => extension.commands?.(navigate) ?? []),
      { id: "go-integrations", group: "Go to", label: "Integrations", icon: <Plug />, run: () => navigate("/integrations") },
      { id: "go-settings", group: "Go to", label: "Settings", icon: <Settings />, run: () => navigate("/settings") },
      { id: "desk", group: "Commands", label: "Ask Claude to start a task", icon: <Sparkles />, run: () => openDock(DESK) },
      ...tasks
        .filter((task) => task.status === "working" && task.currentAgent)
        .map((task) => ({
          id: `terminal-${task.id}`,
          group: "Commands",
          label: `Open the terminal of ${shownId(task.id, task.kind)}`,
          hint: task.currentAgent,
          icon: <SquareTerminal />,
          run: () => openTaskTerminal(task.id),
        })),
      {
        id: "theme",
        group: "Commands",
        label: theme === "dark" ? "Switch to the light theme" : "Switch to the dark theme",
        icon: theme === "dark" ? <Sun /> : <Moon />,
        run: toggleTheme,
      },
      ...tasks.map((task) => taskItem(task, () => navigate(`/tasks/${task.id}`))),
      ...agents.map((agent) => ({
        id: `agent-${agent.name}`,
        group: "Agents",
        label: agent.name,
        hint: agent.coordinates ? "coordinator" : agent.engine,
        icon: <Bot />,
        run: () => navigate(`/agents/${encodeURIComponent(agent.name)}`),
      })),
    ],
    [tasks, agents, extensions, theme, navigate, newTask, toggleTheme, openDock, openTaskTerminal],
  );

  const needsYou = tasks.filter((task) => task.status === "needs you").length;
  return (
    <CrumbContext.Provider value={setCrumbs}>
      <QuestionContext.Provider value={openQuestion}>
        <AgentColorsProvider agents={agents}>
          <TaskTitlesProvider tasks={tasks}>
            <div className="flex h-full flex-col">
              <TopBar
                crumbs={crumbs}
                needsYou={needsYou}
                theme={theme}
                onToggleTheme={toggleTheme}
                onToggleSidebar={() =>
                  matchMedia("(min-width: 768px)").matches ? setCollapsed((c) => !c) : setDrawerOpen((open) => !open)
                }
                onOpenPalette={() => setPalette(true)}
                trust={config?.trust}
              />
              <div className="flex min-h-0 flex-1">
                <Sidebar
                  collapsed={collapsed}
                  drawerOpen={drawerOpen}
                  needsYou={needsYou}
                  extensions={extensions}
                  onCloseDrawer={() => setDrawerOpen(false)}
                />
                <div ref={column} className="flex min-w-0 flex-1 flex-col">
                  <main className="min-h-0 flex-1 scrollbar-thin overflow-y-auto">
                    <Outlet />
                    <NewTaskDialog />
                  </main>
                  {dockOpen && (
                    <TerminalDock
                      height={dockHeight}
                      onResize={resizeDock}
                      selected={dockTab}
                      question={dockQuestion}
                      waiting={questions.length}
                      onSelect={setDockTab}
                      onOpenTaskTerminal={openTaskTerminal}
                      onClose={() => setDockOpen(false)}
                      theme={theme}
                    />
                  )}
                </div>
              </div>
              <StatusBar
                connection={connection}
                position={position}
                defaultRepo={config?.defaultRepo ?? ""}
                policyProblem={config?.policyProblem ?? false}
                missingProfiles={config?.missingProfiles ?? {}}
                working={tasks.filter((task) => task.status === "working").length}
                needsYou={questions.length}
                onOpenQuestions={() => openDock(QUESTIONS)}
                terminalOpen={dockOpen}
                onToggleTerminal={() => toggleDock.current()}
              />
            </div>
            <CommandPalette open={palette} items={paletteItems} onClose={() => setPalette(false)} />
            <ShortcutOverlay open={overlay} onClose={() => setOverlay(false)} />
          </TaskTitlesProvider>
        </AgentColorsProvider>
      </QuestionContext.Provider>
    </CrumbContext.Provider>
  );
}

// taskItem is a task in the palette, found by its id as shown or as stored.
export function taskItem(task: TaskSummary, run: () => void): PaletteItem {
  return {
    id: `task-${task.id}`,
    group: "Tasks",
    label: task.title,
    hint: `${shownId(task.id, task.kind)} · ${task.status}`,
    terms: task.id,
    icon: <ListTodo />,
    run,
  };
}

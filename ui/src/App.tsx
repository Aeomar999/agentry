import { createBrowserRouter, Navigate, RouterProvider, useSearchParams } from "react-router-dom";
import { AdminShell } from "@/components/templates/AdminShell";
import { SignedIn } from "@/components/templates/SignedIn";
import { ToastProvider } from "@/components/organisms/Toasts";
import { LiveProvider } from "@/live/LiveProvider";
import { TasksPage } from "@/pages/TasksPage";
import { PlanPage } from "@/pages/PlanPage";
import { IntegrationsPage } from "@/pages/IntegrationsPage";
import { TaskPage } from "@/pages/TaskPage";
import { AgentsPage } from "@/pages/AgentsPage";
import { AgentPage } from "@/pages/AgentPage";
import { AgentEditPage } from "@/pages/AgentEditPage";
import { PolicyPage } from "@/pages/PolicyPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { WorkersPage } from "@/pages/WorkersPage";
import { NotFoundPage } from "@/pages/NotFoundPage";
import { ErrorPage } from "@/pages/ErrorPage";
import { WorkspacePage } from "@/pages/WorkspacePage";
import { EXTENSIONS } from "@/extensions";

// Every page renders inside the one shell. The Go server answers any path
// that isn't the API or a file with index.html, so these addresses can be
// bookmarked and reloaded. An extension's routes are there whether or not it
// shows, so a bookmark to one still opens.
const router = createBrowserRouter([
  // A task's Workspace is its own page, in its own tab.
  { path: "/tasks/:taskId/workspace", element: <WorkspacePage />, errorElement: <ErrorPage /> },
  {
    element: <AdminShell />,
    children: [
      {
        errorElement: <ErrorPage />,
        children: [
          { path: "/", element: <Navigate to="/tasks" replace /> },
          { path: "/tasks", element: <TasksPage /> },
          { path: "/tasks/new", element: <NewTaskLink /> },
          { path: "/plan", element: <PlanPage /> },
          { path: "/tasks/:taskId", element: <TaskPage /> },
          { path: "/agents", element: <AgentsPage /> },
          { path: "/agents/new", element: <AgentEditPage /> },
          { path: "/agents/:name", element: <AgentPage /> },
          { path: "/agents/:name/edit", element: <AgentEditPage /> },
          { path: "/workers", element: <WorkersPage /> },
          { path: "/integrations", element: <IntegrationsPage /> },
          { path: "/policy", element: <PolicyPage /> },
          { path: "/settings", element: <SettingsPage /> },
          ...EXTENSIONS.flatMap((extension) => extension.routes ?? []),
          { path: "*", element: <NotFoundPage /> },
        ],
      },
    ],
  },
]);

// NewTaskLink keeps old links to the New task page working: it is a modal
// over the Tasks page now (lib/new-task).
function NewTaskLink() {
  const [params] = useSearchParams();
  const epic = params.get("epic");
  const kind = epic ? "story" : params.get("kind") === "epic" ? "epic" : "task";
  return <Navigate to={`/tasks?new=${kind}${epic ? `&newIn=${encodeURIComponent(epic)}` : ""}`} replace />;
}

export function App() {
  return (
    <SignedIn>
      <LiveProvider>
        <ToastProvider>
          <RouterProvider router={router} />
        </ToastProvider>
      </LiveProvider>
    </SignedIn>
  );
}

import { createBrowserRouter, Navigate, RouterProvider } from "react-router-dom";
import { AdminShell } from "@/components/templates/AdminShell";
import { ToastProvider } from "@/components/organisms/Toasts";
import { LiveProvider } from "@/live/LiveProvider";
import { TasksPage } from "@/pages/TasksPage";
import { NewTaskPage } from "@/pages/NewTaskPage";
import { TaskPage } from "@/pages/TaskPage";
import { AgentsPage } from "@/pages/AgentsPage";
import { AgentPage } from "@/pages/AgentPage";
import { AgentEditPage } from "@/pages/AgentEditPage";
import { PolicyPage } from "@/pages/PolicyPage";
import { NotFoundPage } from "@/pages/NotFoundPage";
import { ErrorPage } from "@/pages/ErrorPage";
import { WorkspacePage } from "@/pages/WorkspacePage";

// Every page renders inside the one shell. The Go server answers any path
// that isn't the API or a file with index.html, so these addresses can be
// bookmarked and reloaded.
const router = createBrowserRouter([
  // The peek into a task's workspace is its own page, in its own tab.
  { path: "/tasks/:taskId/workspace", element: <WorkspacePage />, errorElement: <ErrorPage /> },
  {
    element: <AdminShell />,
    children: [
      {
        errorElement: <ErrorPage />,
        children: [
          { path: "/", element: <Navigate to="/tasks" replace /> },
          { path: "/tasks", element: <TasksPage /> },
          { path: "/tasks/new", element: <NewTaskPage /> },
          { path: "/tasks/:taskId", element: <TaskPage /> },
          { path: "/agents", element: <AgentsPage /> },
          { path: "/agents/new", element: <AgentEditPage /> },
          { path: "/agents/:name", element: <AgentPage /> },
          { path: "/agents/:name/edit", element: <AgentEditPage /> },
          { path: "/policy", element: <PolicyPage /> },
          { path: "*", element: <NotFoundPage /> },
        ],
      },
    ],
  },
]);

export function App() {
  return (
    <LiveProvider>
      <ToastProvider>
        <RouterProvider router={router} />
      </ToastProvider>
    </LiveProvider>
  );
}

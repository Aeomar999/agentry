import { useBreadcrumb } from "@/components/templates/AdminShell";
import { PageHeader } from "@/components/molecules/PageHeader";
import { NewTaskForm } from "@/components/organisms/NewTaskForm";
import type { ServerConfig } from "@/api/types";
import { useResource } from "@/live/use-resource";

export function NewTaskPage() {
  useBreadcrumb({ label: "Tasks", to: "/tasks" }, { label: "New task" });
  const config = useResource<ServerConfig>("/api/config").data;
  return (
    <>
      <PageHeader title="New task" meta="The coordinator reads it first and decides which agent works on it." />
      <div className="px-6 py-6">
        <NewTaskForm defaultRepo={config?.defaultRepo ?? ""} />
      </div>
    </>
  );
}

import { useBreadcrumb } from "@/components/templates/AdminShell";
import { PageHeader } from "@/components/molecules/PageHeader";
import { NewTaskForm } from "@/components/organisms/NewTaskForm";

export function NewTaskPage() {
  useBreadcrumb({ label: "Tasks", to: "/tasks" }, { label: "New task" });
  return (
    <>
      <PageHeader title="New task" meta="The coordinator reads it first, names it, and decides which agent works on it." />
      <div className="px-6 py-6">
        <NewTaskForm />
      </div>
    </>
  );
}

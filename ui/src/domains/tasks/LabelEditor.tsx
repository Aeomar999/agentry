import { useState } from "react";
import { Tag } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { LabelList } from "@/components/molecules/LabelChip";
import { LabelInput } from "@/components/molecules/LabelInput";
import { api } from "@/api/client";
import type { TaskSummary } from "@/api/types";
import { knownLabels } from "@/domains/tasks/labels";
import { useResource } from "@/live/use-resource";
import { useToast } from "@/shell/Toasts";

// A task's labels, and editing them in place.
export function LabelEditor({ taskId, labels }: { taskId: string; labels: string[] }) {
  const notify = useToast();
  const tasks = useResource<TaskSummary[]>("/api/tasks").data ?? [];
  const [editing, setEditing] = useState<string[] | null>(null);

  async function save() {
    try {
      await api("PUT", `/api/tasks/${encodeURIComponent(taskId)}/labels`, { labels: editing });
      setEditing(null);
    } catch (failure) {
      notify("danger", "The labels weren't saved", (failure as Error).message);
    }
  }

  if (editing === null) {
    return (
      <span className="flex flex-wrap items-center gap-1.5">
        <LabelList labels={labels} />
        <Button size="sm" variant="ghost" icon={<Tag />} onClick={() => setEditing(labels)}>
          {labels.length ? "Edit labels" : "Add labels"}
        </Button>
      </span>
    );
  }
  return (
    <span className="flex w-full max-w-xl items-center gap-2">
      <LabelInput labels={editing} onChange={setEditing} known={knownLabels(tasks)} autoFocus />
      <Button size="sm" variant="primary" onClick={save}>
        Save
      </Button>
      <Button size="sm" variant="ghost" onClick={() => setEditing(null)}>
        Cancel
      </Button>
    </span>
  );
}

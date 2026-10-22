import type { TaskDetail, TaskSummary } from "@/api/types";
import { Select } from "@/components/atoms/Select";
import { useResource } from "@/live/use-resource";
import { fileUnder } from "@/lib/epic-api";
import { shownEpic } from "@/lib/shown-id";
import { useToast } from "./Toasts";

// EpicPicker files a task under one of the epics still going, moves it to
// another, or takes it out.
export function EpicPicker({ task, onChanged }: { task: TaskDetail; onChanged: () => void }) {
  const notify = useToast();
  const epics = (useResource<TaskSummary[]>("/api/tasks").data ?? []).filter(
    (other) => other.kind === "epic" && (other.status !== "done" || other.id === task.epic),
  );

  async function pick(epic: string) {
    try {
      await fileUnder(task.id, epic);
      notify("success", epic ? `${task.id} is a story of ${shownEpic(epic)}` : `${task.id} is out of its epic`);
      onChanged();
    } catch (failure) {
      notify("danger", "That didn't change", (failure as Error).message);
    }
  }

  return (
    <Select aria-label="Epic" value={task.epic} onChange={(event) => pick(event.target.value)} className="max-w-xs">
      <option value="">No epic</option>
      {epics.map((epic) => (
        <option key={epic.id} value={epic.id}>
          {shownEpic(epic.id)} · {epic.title}
        </option>
      ))}
    </Select>
  );
}

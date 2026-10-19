import { useState } from "react";
import { CircleCheck } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Clamp } from "@/components/molecules/Clamp";
import { Panel } from "@/components/molecules/Panel";
import { api } from "@/api/client";
import type { TaskDetail } from "@/api/types";
import { useToast } from "./Toasts";

// An epic you left open with every story done: it waits quietly until you
// close it, or add a story to it.

// CLOSE_EPIC is the answer that approves closing an epic (coordinator.CloseEpic).
const CLOSE_EPIC = "Close the epic";

export function OpenEpic({ task, onChanged }: { task: TaskDetail; onChanged: () => void }) {
  const notify = useToast();
  const [busy, setBusy] = useState(false);

  async function close() {
    setBusy(true);
    try {
      await api("POST", `/api/tasks/${task.id}/answer`, { answer: CLOSE_EPIC });
      notify("success", "Epic closed", task.title);
      onChanged();
    } catch (error) {
      notify("danger", "Couldn't close the epic", (error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Panel title={`All ${task.stories} stories done`}>
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-ink-secondary">It stays open until you close it, or add a story to it.</p>
        <Clamp text={task.closing} lines={2} />
        <div>
          <Button variant="primary" icon={<CircleCheck />} disabled={busy} onClick={close}>
            Close the epic
          </Button>
        </div>
      </div>
    </Panel>
  );
}

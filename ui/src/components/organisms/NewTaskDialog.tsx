import { useNavigate, useSearchParams } from "react-router-dom";
import { Dialog } from "@/components/molecules/Dialog";
import { NewTaskForm } from "./NewTaskForm";
import type { NewKind } from "@/lib/new-task";

// NewTaskDialog is New task as a modal, over any page, while the address
// asks for it (lib/new-task). A new story stays where you are, so you can
// plan the next; a new task or epic opens.
export function NewTaskDialog() {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const asked = params.get("new") as NewKind | null;
  if (!asked) return null;
  const kind: NewKind = asked === "epic" || asked === "story" ? asked : "task";
  const epic = kind === "story" ? (params.get("newIn") ?? "") : "";
  const close = () =>
    setParams((current) => {
      const next = new URLSearchParams(current);
      next.delete("new");
      next.delete("newIn");
      return next;
    });

  return (
    <Dialog title={epic ? `New story of ${epic}` : kind === "epic" ? "New epic" : "New task"} onClose={close} large>
      <NewTaskForm
        key={`${kind}-${epic}`}
        start={kind}
        epic={epic}
        onCancel={close}
        onCreated={(id) => (epic ? close() : navigate(`/tasks/${id}`))}
      />
    </Dialog>
  );
}

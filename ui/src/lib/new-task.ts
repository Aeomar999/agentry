import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";

// New task is a modal over whatever page you are on, opened by the address
// (?new=task, ?new=epic, or ?new=story&newIn=task-E for a story of an epic),
// so links, Back and a reload all keep it.

export type NewKind = "task" | "story" | "epic";

// useNewTask opens the modal on this page.
export function useNewTask(): (kind?: NewKind, epic?: string) => void {
  const [, setParams] = useSearchParams();
  return useCallback(
    (kind: NewKind = "task", epic = "") =>
      setParams((current) => {
        const next = new URLSearchParams(current);
        next.set("new", kind);
        if (epic) next.set("newIn", epic);
        else next.delete("newIn");
        return next;
      }),
    [setParams],
  );
}

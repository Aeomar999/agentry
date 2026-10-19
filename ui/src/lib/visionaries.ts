import { useState } from "react";
import type { Agent } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { VISIONARIES, addTeam, teamOf } from "./teams";
import { useNewTask } from "./new-task";

export interface AskVisionaries {
  ready: boolean; // the agents are read, so has says what you have
  has: boolean; // you have a Visionaries team
  adding: boolean;
  error: string; // why adding the team failed
  ask: () => void; // opens New task with the team picked, adding it first if you don't have it
}

// useAskVisionaries starts a Visionaries session: New task with the team
// picked, its description the theme or question. With no Visionaries team
// yet, it adds the team from its template first, then carries on.
export function useAskVisionaries(): AskVisionaries {
  const { data, reload } = useResource<Agent[]>("/api/agents");
  const newTask = useNewTask();
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState("");
  const has = (data ?? []).some((agent) => teamOf(agent) === VISIONARIES);

  async function ask() {
    if (!has) {
      setAdding(true);
      setError("");
      try {
        await addTeam(VISIONARIES);
        reload();
      } catch (failure) {
        setError((failure as Error).message);
        return;
      } finally {
        setAdding(false);
      }
    }
    newTask("task", "", VISIONARIES);
  }

  return { ready: data !== undefined, has, adding, error, ask };
}

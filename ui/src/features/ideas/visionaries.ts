import { useEffect, useState } from "react";
import { IDEAS, addTeam, missingProfilesOf, useTeamWith } from "@/lib/teams";
import { useNewTask } from "@/lib/new-task";

export interface AskVisionaries {
  ready: boolean; // the templates and the agents are read, so has says what you have
  has: boolean; // you have a team made from the Visionaries template
  team: string; // its name: yours, or the template's while you have none
  adding: boolean; // adding the team, or waiting for it to show before opening New task
  error: string; // why adding the team failed
  missingProfiles: string[]; // the profiles policy.toml lacks, when adding the team was refused for them
  ask: () => void; // opens New task with the team picked, adding it first if you don't have it
  askAddingProfiles: () => void; // ask, adding the missing profiles before the team
}

// useAskVisionaries starts a Visionaries session: New task with the team
// picked, its description the theme or question. The team is the one made
// from the template that turns Ideas on, whatever you called it. With none
// yet, it adds one from the template first, then opens New task once the
// agents show it, by the name the server gave it.
export function useAskVisionaries(): AskVisionaries {
  const { ready, template, team, reload } = useTeamWith(IDEAS);
  const newTask = useNewTask();
  const [adding, setAdding] = useState(false);
  const [opening, setOpening] = useState(false); // added: New task opens when the team shows
  const [error, setError] = useState("");
  const [missingProfiles, setMissingProfiles] = useState<string[]>([]);

  useEffect(() => {
    if (!opening || !team) return;
    setOpening(false);
    newTask("task", "", team);
  }, [opening, team, newTask]);

  async function ask(addingProfiles = false) {
    if (team) {
      newTask("task", "", team);
      return;
    }
    if (!template || adding || opening) return;
    setAdding(true);
    setError("");
    try {
      // "" is deliberate: the server uses the policy in force, and still refuses a hand edit that isn't reloaded.
      await addTeam(template.team, addingProfiles ? "" : undefined);
      setMissingProfiles([]);
      setOpening(true);
      reload();
    } catch (failure) {
      const profiles = missingProfilesOf(failure);
      if (profiles && !addingProfiles) setMissingProfiles(profiles);
      else {
        setMissingProfiles([]);
        setError((failure as Error).message);
      }
    } finally {
      setAdding(false);
    }
  }

  const has = team !== undefined;
  return {
    ready: ready && template !== undefined,
    has,
    team: team ?? template?.team ?? "Visionaries",
    adding: adding || opening,
    // the team may have been added elsewhere, so drop what adding it left
    error: has ? "" : error,
    missingProfiles: has ? [] : missingProfiles,
    ask: () => void ask(),
    askAddingProfiles: () => void ask(true),
  };
}

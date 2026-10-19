import { useEffect, useState } from "react";
import { IDEAS, addTeam, missingProfilesOf, useTeamWith } from "@/lib/teams";
import { useNewTask } from "@/lib/new-task";

export interface AskVisionaries {
  read: boolean; // the templates and the agents are read
  ready: boolean; // read, and a template turns Ideas on, so has says what you have
  loadError: string; // why the templates or the agents couldn't be read
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
// agents show it, by the name the server gave it, or says it didn't show
// if the agents read after adding it lack it (it was removed at once), or
// that they couldn't be read.
export function useAskVisionaries(): AskVisionaries {
  const { ready, error: loadError, agentsError, template, team, reload } = useTeamWith(IDEAS);
  const newTask = useNewTask();
  const [adding, setAdding] = useState(false);
  const [opening, setOpening] = useState(false); // added: New task opens when the team shows
  const [reloaded, setReloaded] = useState(false); // the agents were read after adding it
  const [error, setError] = useState("");
  const [missingProfiles, setMissingProfiles] = useState<string[]>([]);

  useEffect(() => {
    if (!opening) return;
    if (team) {
      setOpening(false);
      newTask("task", "", team);
    } else if (reloaded) {
      setOpening(false);
      setError(
        agentsError
          ? "The team was added, but the agents couldn't be read; try again."
          : "The team was added but didn't show up; try again.",
      );
    }
  }, [opening, reloaded, team, agentsError, newTask]);

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
      setReloaded(false);
      setOpening(true);
      await reload();
      setReloaded(true);
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
    read: ready,
    ready: ready && template !== undefined,
    loadError,
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

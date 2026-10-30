import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import type { Agent } from "@/api/types";
import { api } from "@/api/client";
import { Button } from "@/components/atoms/Button";
import { useToast } from "@/shell/Toasts";
import { onlyTeamReason, removeTeamDetail, removeTeamQuestion, tasksNeeding } from "@/lib/custom-team";
import { cn } from "@/lib/cn";

// Remove team in the Edit team panel (docs/design/custom-teams.md, "2. Edit
// a team", decisions 7 and 12): a danger text button that turns into its
// question in place, Keep first. It is the heading's Remove team, by the
// same route: Agentry refuses while an unfinished task needs the team, and
// the refusal names the tasks. On desktop it lives in the footer; on a
// phone, at the end of the body under This team. Both places are drawn and
// CSS shows one, so each says where it is (place) for its ids and focus.

export type Place = "footer" | "body";

// Why the server refused, and the roster it refused against: Remove stays
// off until the roster changes, as when a task it named finishes.
interface Refusal {
  words: string;
  tasks: string[];
  roster?: Agent[];
}

// useRemoveTeam is the question's state: where it was asked, if it is,
// the request while it runs, and the last refusal.
// Keep puts focus back on the Remove team it was asked from.
export function useRemoveTeam(
  team: string,
  roster: Agent[] | undefined,
  focus: (id: string) => void,
  onRemoved: (team: string) => Promise<void>,
) {
  const notify = useToast();
  const [asking, setAsking] = useState<Place | null>(null);
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<Refusal | null>(null);

  async function remove() {
    setBusy(true);
    try {
      await api("DELETE", `/api/teams/${encodeURIComponent(team)}`);
    } catch (failure) {
      const words = (failure as Error).message;
      setRefused({ words, tasks: tasksNeeding(words), roster });
      setBusy(false);
      return;
    }
    notify("success", `Removed the ${team} team`);
    await onRemoved(team);
  }

  const keep = () => {
    if (asking) focus(idOf(asking, "button"));
    setAsking(null);
    setRefused(null);
  };

  return { asking, busy, refused, off: refused !== null && refused.roster === roster, ask: setAsking, keep, remove };
}

export type TeamRemoval = ReturnType<typeof useRemoveTeam>;

const idOf = (place: Place, part: string) => `team-remove-${place}-${part}`;

interface RemoveTeamButtonProps {
  place: Place;
  removal: TeamRemoval;
  only: boolean; // your only team, which stays
  off: boolean; // while the team saves
  className?: string;
}

// RemoveTeamButton asks the question, or says beside it why it can't.
export function RemoveTeamButton({ place, removal, only, off, className }: RemoveTeamButtonProps) {
  return (
    <span className={cn("flex items-center gap-x-3 gap-y-1", className)}>
      <Button
        id={idOf(place, "button")}
        variant="ghost"
        disabled={only || off}
        aria-describedby={only ? idOf(place, "why") : undefined}
        onClick={() => removal.ask(place)}
        className="text-danger-ink hover:bg-danger-soft hover:text-danger-ink max-sm:h-11 max-sm:w-full"
      >
        Remove team
      </Button>
      {only && (
        <span id={idOf(place, "why")} className="max-w-56 text-[12px] text-ink-muted max-sm:max-w-none">
          {onlyTeamReason}
        </span>
      )}
    </span>
  );
}

interface RemoveTeamQuestionProps {
  place: Place;
  removal: TeamRemoval;
  team: string; // as saved
  agents: number;
  changed: boolean; // your unsaved changes go with the team
  saving: boolean; // Remove is off while the team saves
}

// RemoveTeamQuestion is the question in place: a group named by it, focus
// on Keep where it was asked. A refusal says the server's words, quoted,
// and links the tasks it names; it takes focus as it shows.
export function RemoveTeamQuestion({ place, removal, team, agents, changed, saving }: RemoveTeamQuestionProps) {
  const keep = useRef<HTMLButtonElement>(null);
  const refusal = useRef<HTMLDivElement>(null);
  const here = removal.asking === place;
  useEffect(() => {
    if (here) keep.current?.focus();
  }, [here]);
  useEffect(() => {
    if (here && removal.refused) refusal.current?.focus();
  }, [here, removal.refused]);
  return (
    <div
      role="group"
      aria-labelledby={idOf(place, "question")}
      aria-describedby={idOf(place, "detail")}
      className="flex flex-wrap items-center justify-end gap-x-3 gap-y-2"
    >
      <div className="mr-auto min-w-0 max-sm:basis-full">
        <p id={idOf(place, "question")} className="text-[13px] font-medium text-ink-primary">
          {removeTeamQuestion(team, agents)}
        </p>
        <p id={idOf(place, "detail")} className="text-[12px] text-ink-secondary">
          {removeTeamDetail(changed)}
        </p>
        {removal.refused && (
          <div ref={refusal} tabIndex={-1} className="mt-1.5 text-[12px] text-danger-ink outline-none focus-visible:outline-focus">
            <p>It wasn&apos;t removed: &ldquo;{removal.refused.words}&rdquo;</p>
            {removal.refused.tasks.length > 0 && <TaskLinks tasks={removal.refused.tasks} />}
          </div>
        )}
      </div>
      <Button ref={keep} disabled={removal.busy} onClick={removal.keep} className="max-sm:h-11 max-sm:flex-1">
        Keep
      </Button>
      <Button
        variant="danger"
        disabled={removal.busy || removal.off || saving}
        onClick={removal.remove}
        className="max-sm:h-11 max-sm:flex-1"
      >
        {removal.busy ? "Removing…" : "Remove"}
      </Button>
    </div>
  );
}

// TaskLinks is "Tasks: " and each task, a link to it.
function TaskLinks({ tasks }: { tasks: string[] }) {
  return (
    <p>
      Tasks:{" "}
      {tasks.map((task, i) => (
        <span key={task}>
          {i > 0 && ", "}
          <Link to={`/tasks/${task}`} className="font-mono text-ink-primary underline hover:no-underline">
            {task}
          </Link>
        </span>
      ))}
    </p>
  );
}

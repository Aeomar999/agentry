import { FormEvent, useState } from "react";
import { Plus, Users } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Input, TextArea } from "@/components/atoms/Input";
import { Banner } from "@/components/molecules/Banner";
import { Field } from "@/components/molecules/Field";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { api } from "@/api/client";
import type { Agent, CreatedEvent, ServerConfig, TaskSummary } from "@/api/types";
import { Select } from "@/components/atoms/Select";
import { FilterChip } from "@/components/molecules/FilterChip";
import { VISIONARIES, addTeam, teamsOf } from "@/lib/teams";
import { LimitSelect } from "@/components/molecules/LimitSelect";
import { LimitChoice, LimitKind, SCALES, choiceOf, limitOf, sent } from "@/lib/limits";
import { LabelInput } from "@/components/molecules/LabelInput";
import { knownLabels } from "@/lib/labels";
import { useResource } from "@/live/use-resource";
import { modKey } from "@/lib/shortcuts";
import { useToast } from "./Toasts";
import { share, sizeOf } from "@/lib/shared";

// A task is what you describe. The title is optional: the coordinator reads
// the task and names it, and renames a title that doesn't say what the task
// is. Its work goes in the default repo, another repo, or a new repo that
// Agentry makes for it: its worktree is always of that repo, so the
// Workspace shows the work and it goes with the task between machines.
interface NewTaskFormProps {
  start?: "task" | "story" | "epic"; // the kind it starts as
  epic?: string; // for a story: its epic
  team?: string; // the team it starts with picked
  onCreated: (id: string) => void;
  onCancel: () => void;
}

export function NewTaskForm({ start = "task", epic = "", team: preset = "", onCreated, onCancel }: NewTaskFormProps) {
  const notify = useToast();
  const [description, setDescription] = useState("");
  const [title, setTitle] = useState("");
  const [labels, setLabels] = useState<string[]>([]);
  const [attached, setAttached] = useState<File[]>([]); // shared with the task once it is made
  const [limits, setLimits] = useState<Partial<Record<LimitKind, LimitChoice>>>({}); // only those you changed
  const [where, setWhere] = useState<"default" | "existing" | "new">("default");
  const agents = useResource<Agent[]>("/api/agents");
  const teams = teamsOf(agents.data ?? []);
  const [team, setTeam] = useState(preset);
  const [kind, setKind] = useState<"" | "story" | "epic">(epic ? "story" : start === "epic" ? "epic" : "");
  const chosen = teams.find((t) => t.team === team); // none: the coordinators decide
  const lead = chosen?.agents.find((a) => a.coordinates);
  const asksVisionaries = chosen?.team === VISIONARIES;
  // Asked for the Visionaries team, and you don't have it yet.
  const visionariesMissing = preset === VISIONARIES && agents.data !== undefined && !teams.some((t) => t.team === VISIONARIES);
  const [adding, setAdding] = useState(false);
  const [repo, setRepo] = useState("");
  const config = useResource<ServerConfig>("/api/config").data;
  const tasks = useResource<TaskSummary[]>("/api/tasks").data ?? [];
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event?: FormEvent) {
    event?.preventDefault();
    if (!description.trim() || busy) return;
    const asked = askedLimits(limits);
    if (Number.isNaN(asked.budget) || Number.isNaN(asked.maxTurns)) {
      setError("Give the budget and max turns as numbers, or pick a preset.");
      return;
    }
    if (where !== "default" && !repo.trim().startsWith("/")) {
      setError("Give the repo's full path, starting with /.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const created = await api<CreatedEvent>("POST", "/api/tasks", {
        description,
        title,
        labels,
        ...asked,
        repo: where === "default" ? "" : repo.trim(),
        team: chosen?.team ?? "",
        kind,
        epic,
        newRepo: where === "new",
      });
      notify(
        "success",
        `Created ${created.stream}`,
        epic ? `A story of ${epic}, in your Backlog until you start it.` : "The coordinator reads it next.",
      );
      const unshared: string[] = [];
      for (const file of attached) {
        try {
          await share(created.stream, file);
        } catch (failure) {
          unshared.push(`${file.name}: ${(failure as Error).message}`);
        }
      }
      if (unshared.length) notify("danger", "Some files weren't shared", unshared.join("\n"));
      onCreated(created.stream);
    } catch (failure) {
      setError((failure as Error).message);
      setBusy(false);
    }
  }

  async function addVisionaries() {
    setAdding(true);
    try {
      await addTeam(VISIONARIES);
      notify("success", "Added the Visionaries team", "It is picked: give it a theme or a question.");
      agents.reload();
    } catch (failure) {
      setError((failure as Error).message);
    }
    setAdding(false);
  }

  // The box you describe the task in, saying what to write in it.
  function describe(placeholder: string) {
    return (
      <TextArea
        id="task-description"
        aria-label={asksVisionaries ? "Theme or question" : "The task"}
        autoFocus
        rows={8}
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && (e.metaKey || e.ctrlKey) && submit()}
        placeholder={placeholder}
        className="text-[14px]"
      />
    );
  }

  return (
    <form onSubmit={submit} className="flex max-w-3xl flex-col gap-5">
      {error && <Banner title="The task wasn't created">{error}</Banner>}
      {visionariesMissing && (
        <Banner tone="info" title="You have no Visionaries team yet">
          <div className="flex flex-col items-start gap-2">
            <span>Add it from its template: a lead and four viewpoints that argue out what to build next, and change no code.</span>
            <Button size="sm" icon={<Users />} disabled={adding} onClick={addVisionaries}>
              {adding ? "Adding…" : "Add the Visionaries team"}
            </Button>
          </div>
        </Banner>
      )}
      {asksVisionaries ? (
        <Field
          label="Theme or question"
          htmlFor="task-description"
          hint="What the visionaries think about together. The ideas they agree are worth it come to the Ideas page, for you to accept or dismiss."
        >
          {describe("A theme, a question or a part of the product to think about, such as: what would make reviewing work faster?")}
        </Field>
      ) : (
        describe("Describe what should be done, in your own words. What to change, why, and how you'll know it works.")
      )}
      <Field label="Title" htmlFor="task-title" hint="Optional. The coordinator names the task once it has read it.">
        <Input id="task-title" value={title} onChange={(e) => setTitle(e.target.value)} className="max-w-md" />
      </Field>
      {teams.length > 1 && (
        <Field
          label="Team"
          htmlFor="task-team"
          hint={
            !chosen
              ? `${teams[0].agents.find((a) => a.coordinates)?.name ?? "The first team's coordinator"} reads it first and passes it to the team whose work it is.`
              : lead
                ? `${lead.name} coordinates it, with ${chosen!.agents
                    .filter((a) => !a.coordinates)
                    .map((a) => a.name)
                    .join(", ")}.`
                : "This team has no coordinator yet."
          }
        >
          <div id="task-team" role="group" aria-label="Team" className="flex flex-wrap gap-1.5">
            <FilterChip label="Let the coordinators decide" active={!chosen} onClick={() => setTeam("")} />
            {teams.map((t) => (
              <FilterChip key={t.team} label={t.team} active={chosen?.team === t.team} onClick={() => setTeam(t.team)} />
            ))}
          </div>
        </Field>
      )}
      {epic ? (
        <Field label="Epic" htmlFor="task-kind" hint="It is a story of this epic, and waits in your Backlog until you start it.">
          <span id="task-kind" className="font-mono text-[13px] text-ink-primary">
            {epic}
          </span>
        </Field>
      ) : (
        <Field
          label="Kind"
          htmlFor="task-kind"
          hint={
            kind === "epic"
              ? "A goal too big for one change. The coordinator splits it into stories, each a task with its own branch and checks, and finishes it once they are all done."
              : kind === "story"
                ? "One change, on one branch, checked before it is done. It is never split."
                : "The coordinator splits it into an epic of stories if it is too big for one change."
          }
        >
          <Select id="task-kind" value={kind} onChange={(e) => setKind(e.target.value as typeof kind)} className="max-w-xs">
            <option value="">Let the coordinator decide</option>
            <option value="story">Story</option>
            <option value="epic">Epic</option>
          </Select>
        </Field>
      )}
      <Field
        label="Where the work goes"
        htmlFor="task-repo"
        hint={
          where === "default"
            ? epic
              ? "The epic's repo."
              : `The default repo${config ? `, ${config.defaultRepo}` : ""}.`
            : where === "existing"
              ? "A git repository on this machine by its full path, or an address to copy it from: any git address (git@host:team/app.git, https://…) or a link to a .zip or .tar.gz. Agentry clones or downloads it."
              : "Agentry makes a new git repository at this path, with a first commit, and the task works in it."
        }
      >
        <div className="flex max-w-xl flex-col gap-2">
          <SegmentedControl
            active={where}
            onChange={(id) => setWhere(id as typeof where)}
            segments={[
              { id: "default", label: epic ? "The epic's repo" : "The default repo" },
              { id: "existing", label: "Another repo" },
              { id: "new", label: "A new repo" },
            ]}
          />
          {where !== "default" && (
            <Input
              id="task-repo"
              mono
              value={repo}
              onChange={(e) => setRepo(e.target.value)}
              placeholder={where === "new" ? "/home/you/projects/new-project" : "/home/you/projects/app or git@github.com:team/app.git"}
            />
          )}
        </div>
      </Field>
      <Field
        label="Files"
        htmlFor="task-files"
        hint="Optional. Shared with the task: its agents find them in .agentry/files, on whichever machine works on it."
      >
        <div className="flex flex-col gap-2">
          <input
            id="task-files"
            type="file"
            multiple
            onChange={(e) => e.target.files && setAttached([...attached, ...e.target.files])}
            className="max-w-md text-[12px] text-ink-secondary file:mr-3 file:cursor-pointer file:rounded-sm file:border file:border-divider file:bg-surface-1 file:px-2 file:py-1 file:text-ink-primary"
          />
          {attached.length > 0 && (
            <ul className="flex max-w-md flex-col gap-1">
              {attached.map((file, n) => (
                <li key={`${file.name}-${n}`} className="flex items-center gap-2 text-[12px] text-ink-secondary">
                  <span className="truncate font-mono">{file.name}</span>
                  <span className="text-ink-faint">{sizeOf(file.size)}</span>
                  <button
                    type="button"
                    onClick={() => setAttached(attached.filter((_, i) => i !== n))}
                    className="ml-auto cursor-pointer text-ink-faint hover:text-danger"
                  >
                    Remove
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Field>
      <Field label="Labels" htmlFor="task-labels" hint="Optional. Enter or a comma after each.">
        <div id="task-labels" className="max-w-md">
          <LabelInput labels={labels} onChange={setLabels} known={knownLabels(tasks)} />
        </div>
      </Field>
      {config && (
        <div className="flex flex-wrap gap-6">
          <Field
            label="Token budget"
            htmlFor="task-budget"
            hint="When the task has used this many tokens it stops and asks you to raise it or wrap up."
          >
            <LimitSelect
              id="task-budget"
              scale={SCALES.budget}
              choice={limits.budget ?? choiceOf(SCALES.budget, config.budget)}
              onChange={(choice) => setLimits({ ...limits, budget: choice })}
              defaultLimit={config.budget}
            />
          </Field>
          <Field label="Max turns" htmlFor="task-turns" hint="Turns without your answer before it stops and asks you.">
            <LimitSelect
              id="task-turns"
              scale={SCALES.turns}
              choice={limits.turns ?? choiceOf(SCALES.turns, config.maxTurns)}
              onChange={(choice) => setLimits({ ...limits, turns: choice })}
              defaultLimit={config.maxTurns}
            />
          </Field>
        </div>
      )}
      <div className="flex items-center gap-3">
        <Button type="submit" variant="primary" icon={<Plus />} disabled={busy || !description.trim()}>
          {epic ? "Add story" : kind === "epic" ? "Add epic" : "Add task"}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <span className="text-[11px] text-ink-faint">{modKey} Enter to add</span>
      </div>
    </form>
  );
}

// The limits a new task asks for: left out takes the server's default, -1
// is no limit, NaN a custom value that isn't a number.
function askedLimits(limits: Partial<Record<LimitKind, LimitChoice>>): { budget?: number; maxTurns?: number } {
  const asked = (kind: LimitKind) => {
    const choice = limits[kind];
    return choice && sent(limitOf(SCALES[kind], choice));
  };
  return { budget: asked("budget"), maxTurns: asked("turns") };
}

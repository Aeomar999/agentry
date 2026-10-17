import { FormEvent, useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Input, TextArea } from "@/components/atoms/Input";
import { Banner } from "@/components/molecules/Banner";
import { Field } from "@/components/molecules/Field";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { api } from "@/api/client";
import type { Agent, CreatedEvent, ServerConfig, TaskSummary } from "@/api/types";
import { Select } from "@/components/atoms/Select";
import { teamsOf } from "@/lib/teams";
import { parseTokens, tokens } from "@/lib/tokens";
import { LabelInput } from "@/components/molecules/LabelInput";
import { knownLabels } from "@/lib/labels";
import { useResource } from "@/live/use-resource";
import { modKey } from "@/lib/shortcuts";
import { useToast } from "./Toasts";

// A task is what you describe. The title is optional: the coordinator reads
// the task and names it, and renames a title that doesn't say what the task
// is. Its work goes in the default repo, another repo, or a new repo that
// Agentry makes for it: its worktree is always of that repo, so the
// Workspace shows the work and it goes with the task between machines.
interface NewTaskFormProps {
  start?: "task" | "story" | "epic"; // the kind it starts as
  epic?: string; // for a story: its epic
  onCreated: (id: string) => void;
  onCancel: () => void;
}

export function NewTaskForm({ start = "task", epic = "", onCreated, onCancel }: NewTaskFormProps) {
  const notify = useToast();
  const [description, setDescription] = useState("");
  const [title, setTitle] = useState("");
  const [labels, setLabels] = useState<string[]>([]);
  const [budget, setBudget] = useState("");
  const [where, setWhere] = useState<"default" | "existing" | "new">("default");
  const teams = teamsOf(useResource<Agent[]>("/api/agents").data ?? []);
  const [team, setTeam] = useState("");
  const [kind, setKind] = useState<"" | "story" | "epic">(epic ? "story" : start === "epic" ? "epic" : "");
  const chosen = teams.find((t) => t.team === team); // none: the coordinators decide
  const lead = chosen?.agents.find((a) => a.coordinates);
  const [repo, setRepo] = useState("");
  const config = useResource<ServerConfig>("/api/config").data;
  const tasks = useResource<TaskSummary[]>("/api/tasks").data ?? [];
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event?: FormEvent) {
    event?.preventDefault();
    if (!description.trim() || busy) return;
    const asked = budget.trim() ? parseTokens(budget) : 0;
    if (Number.isNaN(asked) || asked < 0) {
      setError("A budget is a number of tokens, such as 2M or 500k.");
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
        budget: asked,
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
      onCreated(created.stream);
    } catch (failure) {
      setError((failure as Error).message);
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex max-w-3xl flex-col gap-5">
      {error && <Banner title="The task wasn't created">{error}</Banner>}
      <TextArea
        aria-label="The task"
        autoFocus
        rows={8}
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && (e.metaKey || e.ctrlKey) && submit()}
        placeholder="Describe what should be done, in your own words. What to change, why, and how you'll know it works."
        className="text-[14px]"
      />
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
          <Select id="task-team" value={chosen?.team ?? ""} onChange={(e) => setTeam(e.target.value)} className="max-w-xs">
            <option value="">Let the coordinators decide</option>
            {teams.map((t) => (
              <option key={t.team} value={t.team}>
                {t.team}
              </option>
            ))}
          </Select>
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
              ? "A git repository on this machine, by its full path."
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
              placeholder={where === "new" ? "/home/you/projects/new-project" : "/home/you/projects/existing-repo"}
            />
          )}
        </div>
      </Field>
      <Field label="Labels" htmlFor="task-labels" hint="Optional. Enter or a comma after each.">
        <div id="task-labels" className="max-w-md">
          <LabelInput labels={labels} onChange={setLabels} known={knownLabels(tasks)} />
        </div>
      </Field>
      <Field
        label="Token budget"
        htmlFor="task-budget"
        hint="Optional. When the task has used this many tokens it stops and asks you to raise it or wrap up."
      >
        <Input
          id="task-budget"
          mono
          value={budget}
          onChange={(e) => setBudget(e.target.value)}
          placeholder={config ? (config.budget ? `${tokens(config.budget)}, the default` : "no limit") : ""}
          className="max-w-48"
        />
      </Field>
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

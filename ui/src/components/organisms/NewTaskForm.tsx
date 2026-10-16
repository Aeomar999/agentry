import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Plus } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Input, TextArea } from "@/components/atoms/Input";
import { Banner } from "@/components/molecules/Banner";
import { Field } from "@/components/molecules/Field";
import { api } from "@/api/client";
import type { CreatedEvent, ServerConfig, TaskSummary } from "@/api/types";
import { parseTokens, tokens } from "@/lib/tokens";
import { LabelInput } from "@/components/molecules/LabelInput";
import { knownLabels } from "@/lib/labels";
import { useResource } from "@/live/use-resource";
import { modKey } from "@/lib/shortcuts";
import { useToast } from "./Toasts";

// A task is what you describe. The title is optional: the coordinator reads
// the task and names it, and renames a title that doesn't say what the task
// is. It works in the default repo; the desk can name another.
export function NewTaskForm() {
  const navigate = useNavigate();
  const notify = useToast();
  const [description, setDescription] = useState("");
  const [title, setTitle] = useState("");
  const [labels, setLabels] = useState<string[]>([]);
  const [budget, setBudget] = useState("");
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
    setBusy(true);
    setError("");
    try {
      const created = await api<CreatedEvent>("POST", "/api/tasks", { description, title, labels, budget: asked });
      notify("success", `Created ${created.stream}`, "The coordinator reads it next.");
      navigate(`/tasks/${created.stream}`);
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
        rows={12}
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && (e.metaKey || e.ctrlKey) && submit()}
        placeholder="Describe what should be done, in your own words. What to change, why, and how you'll know it works."
        className="text-[14px]"
      />
      <Field label="Title" htmlFor="task-title" hint="Optional. The coordinator names the task once it has read it.">
        <Input id="task-title" value={title} onChange={(e) => setTitle(e.target.value)} className="max-w-md" />
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
          Add task
        </Button>
        <Button variant="ghost" onClick={() => navigate("/tasks")}>
          Cancel
        </Button>
        <span className="text-[11px] text-ink-faint">{modKey} Enter to add</span>
      </div>
    </form>
  );
}

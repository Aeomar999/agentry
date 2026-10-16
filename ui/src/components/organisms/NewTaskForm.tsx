import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Plus } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Input, TextArea } from "@/components/atoms/Input";
import { Banner } from "@/components/molecules/Banner";
import { Field } from "@/components/molecules/Field";
import { api } from "@/api/client";
import type { CreatedEvent } from "@/api/types";
import { useToast } from "./Toasts";

export function NewTaskForm({ defaultRepo }: { defaultRepo: string }) {
  const navigate = useNavigate();
  const notify = useToast();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [repo, setRepo] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const created = await api<CreatedEvent>("POST", "/api/tasks", { title, description, repo });
      notify("success", `Created ${created.stream}`, "The coordinator picks it up next.");
      navigate(`/tasks/${created.stream}`);
    } catch (failure) {
      setError((failure as Error).message);
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex max-w-2xl flex-col gap-5">
      {error && <Banner title="The task wasn't created">{error}</Banner>}
      <Field label="What should be done" htmlFor="task-title">
        <Input
          id="task-title"
          required
          autoFocus
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Remove the TODO comments in src/"
        />
      </Field>
      <Field label="Details" htmlFor="task-description" hint="Optional. Anything the coordinator should know.">
        <TextArea id="task-description" value={description} onChange={(e) => setDescription(e.target.value)} rows={6} />
      </Field>
      <Field
        label="Repository"
        htmlFor="task-repo"
        hint="The task works in its own git worktree of this repo. Leave it empty for the default."
      >
        <Input id="task-repo" mono value={repo} onChange={(e) => setRepo(e.target.value)} placeholder={defaultRepo} />
      </Field>
      <div className="flex gap-2">
        <Button type="submit" variant="primary" icon={<Plus />} disabled={busy || !title.trim()}>
          Add task
        </Button>
        <Button variant="ghost" onClick={() => navigate("/tasks")}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

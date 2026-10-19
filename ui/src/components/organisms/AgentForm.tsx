import { FormEvent, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { Save, Trash2 } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Checkbox } from "@/components/atoms/Checkbox";
import { Input } from "@/components/atoms/Input";
import { Select } from "@/components/atoms/Select";
import { Banner } from "@/components/molecules/Banner";
import { CodeViewer } from "@/components/molecules/CodeViewer";
import { Field } from "@/components/molecules/Field";
import { DEFAULT_TEAM, teamsOf } from "@/lib/teams";
import { Panel } from "@/components/molecules/Panel";
import { api } from "@/api/client";
import type { Agent, Policy, ServerConfig } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { useToast } from "./Toasts";
import { effectiveProfile } from "@/lib/profiles";
import { AGENT_COLORS } from "@/live/agent-colors";
import { cn } from "@/lib/cn";
import { LOCAL, machineName } from "@/lib/machines";

// Add an agent, or change one. A new name renames it: it keeps its prompt,
// settings and knowledge, and the tasks that name it go on with the new
// name. Workers are the machines it may run on, in order; none means the
// machine Agentry runs on.

// Model names each engine understands besides a full model id.
const MODELS: Record<string, string[]> = { claude: ["opus", "sonnet", "haiku"], codex: [] };

const BLANK: Agent = {
  name: "",
  description: "",
  prompt: "",
  engine: "claude",
  model: "",
  profile: "",
  coordinates: false,
  team: "",
  workers: [],
  color: "",
  column: "",
  knowledge: "",
};

export function AgentForm({ agent }: { agent?: Agent }) {
  const navigate = useNavigate();
  const notify = useToast();
  const config = useResource<ServerConfig>("/api/config").data;
  const policy = useResource<Policy>("/api/policy").data;
  const [params] = useSearchParams();
  const [draft, setDraft] = useState<Agent>(agent ?? { ...BLANK, team: params.get("team") ?? "" });
  const teams = teamsOf(useResource<Agent[]>("/api/agents").data ?? []).map((t) => t.team);
  const [error, setError] = useState({ title: "", reason: "" });
  const [busy, setBusy] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const isNew = agent === undefined;
  const set = (change: Partial<Agent>) => setDraft((current) => ({ ...current, ...change }));
  // For the profile being picked, not the one saved.
  const inForce = policy && effectiveProfile(draft.profile, policy);

  async function save(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError({ title: "", reason: "" });
    try {
      if (isNew) await api("POST", "/api/agents", draft);
      else {
        // A new name first: the agent keeps everything, and its tasks follow it.
        if (draft.name !== agent.name) await api("POST", `/api/agents/${encodeURIComponent(agent.name)}/rename`, { to: draft.name });
        await api("PUT", `/api/agents/${encodeURIComponent(draft.name)}`, draft);
      }
      notify("success", isNew ? `Added ${draft.name}` : `Saved ${draft.name}`, "The coordinator sees it from its next turn.");
      navigate(`/agents/${encodeURIComponent(draft.name)}`);
    } catch (failure) {
      setError({ title: isNew ? "The agent wasn't added" : "The change wasn't saved", reason: (failure as Error).message });
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    try {
      await api("DELETE", `/api/agents/${encodeURIComponent(draft.name)}`);
      notify("success", `Removed ${draft.name}`);
      navigate("/agents");
    } catch (failure) {
      setError({ title: `${draft.name} wasn't removed`, reason: (failure as Error).message });
      setBusy(false);
      setConfirmRemove(false);
    }
  }

  const workers = config?.workers ?? [LOCAL];
  const toggleWorker = (worker: string, on: boolean) =>
    set({ workers: on ? [...draft.workers, worker] : draft.workers.filter((w) => w !== worker) });

  return (
    <form onSubmit={save} className="flex flex-col gap-6 xl:flex-row xl:items-start">
      <div className="flex min-w-0 flex-1 flex-col gap-4">
        {error.reason && <Banner title={error.title}>{error.reason}</Banner>}
        <Panel flush title="Prompt" aside="Its standing instructions, at the start of every turn">
          <CodeViewer text={draft.prompt} language="markdown" minHeight={360} maxHeight={2000} onChange={(prompt) => set({ prompt })} />
        </Panel>
      </div>

      <aside className="flex w-full shrink-0 flex-col gap-4 xl:w-inspector">
        <Panel title="Agent">
          <div className="flex flex-col gap-4">
            <Field
              label="Name"
              htmlFor="agent-name"
              hint={
                isNew
                  ? "Lowercase letters, digits and dashes."
                  : draft.name !== agent.name
                    ? `Renames ${agent.name}: it keeps its prompt, settings and what it knows, and its tasks go on with the new name.`
                    : "Lowercase letters, digits and dashes."
              }
            >
              <Input
                id="agent-name"
                mono
                required
                value={draft.name}
                onChange={(e) => set({ name: e.target.value.toLowerCase() })}
                placeholder="designer"
              />
            </Field>
            <Field
              label="Team"
              htmlFor="agent-team"
              hint="The team it works in. Its coordinator hands it work; a new name starts a new team, which needs a coordinator first."
            >
              <Input
                id="agent-team"
                list="agent-teams"
                value={draft.team || DEFAULT_TEAM}
                onChange={(e) => set({ team: e.target.value })}
              />
              <datalist id="agent-teams">
                {teams.map((team) => (
                  <option key={team} value={team} />
                ))}
              </datalist>
            </Field>
            <Field label="Description" htmlFor="agent-description" hint="One line. The coordinator reads it to choose who works next.">
              <Input id="agent-description" value={draft.description} onChange={(e) => set({ description: e.target.value })} />
            </Field>
            <div className="flex flex-col gap-1.5">
              <span className="text-[12px] font-medium text-ink-secondary">Colour</span>
              <div role="radiogroup" aria-label="Colour" className="flex flex-wrap gap-1.5">
                {AGENT_COLORS.map((color) => (
                  <button
                    key={color}
                    type="button"
                    role="radio"
                    aria-checked={draft.color === color}
                    aria-label={color}
                    title={color}
                    onClick={() => set({ color })}
                    className={cn(
                      "size-6 cursor-pointer rounded-sm border",
                      draft.color === color ? "border-ink-primary" : "border-transparent hover:border-divider-2",
                    )}
                  >
                    <span className="mx-auto block size-3.5 rounded-full" style={{ background: `var(--color-agent-${color})` }} />
                  </button>
                ))}
              </div>
            </div>
            <Field
              label="Its work counts as"
              htmlFor="agent-column"
              hint={
                draft.column === "review"
                  ? "Checking. Its report ends with a verdict, and its coordinator can't finish a task until it approves the last change. Its tasks show in Review on the board."
                  : draft.column === "ship"
                    ? "Shipping checked work, such as deploying it: it doesn't change the work, so the checks still hold. Its tasks show in Shipping on the board."
                    : "Building. A change it makes is checked again by the team's reviewers before the task is done. Its tasks show in In progress on the board."
              }
            >
              <Select id="agent-column" value={draft.column} onChange={(e) => set({ column: e.target.value as Agent["column"] })}>
                <option value="">Building</option>
                <option value="review">Review</option>
                <option value="ship">Shipping</option>
              </Select>
            </Field>
            <Checkbox
              checked={draft.coordinates}
              onChange={(coordinates) => set({ coordinates })}
              label="Coordinates"
              hint="Decides who works next in its team instead of working. The team's tasks go to it."
            />
          </div>
        </Panel>

        <Panel title="Engine">
          <div className="flex flex-col gap-4">
            <Field label="Engine" htmlFor="agent-engine">
              <Select id="agent-engine" value={draft.engine} onChange={(e) => set({ engine: e.target.value })}>
                {(config?.engines ?? ["claude", "codex"]).map((engine) => (
                  <option key={engine} value={engine}>
                    {engine}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Model" htmlFor="agent-model" hint="Empty for the engine's default.">
              <Input id="agent-model" mono list="agent-models" value={draft.model} onChange={(e) => set({ model: e.target.value })} />
              <datalist id="agent-models">
                {(MODELS[draft.engine] ?? []).map((model) => (
                  <option key={model} value={model} />
                ))}
              </datalist>
            </Field>
            <Field
              label="Policy profile"
              htmlFor="agent-profile"
              hint={
                inForce?.overridden
                  ? `In force: ${inForce.text}. This picks its own profile, for when use_for_everyone is off.`
                  : policy?.useForEveryone
                    ? `The policy file sets every agent to ${policy.useForEveryone} for now.`
                    : "What it may do without asking you, from policy.toml."
              }
            >
              <Select id="agent-profile" value={draft.profile} onChange={(e) => set({ profile: e.target.value })}>
                <option value="">default ({policy?.defaultProfile ?? "developer"})</option>
                {(policy?.profiles ?? []).map((profile) => (
                  <option key={profile.name} value={profile.name}>
                    {profile.name}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
        </Panel>

        <Panel title="Runs on">
          {workers.length <= 1 ? (
            <p className="text-[12px] text-ink-muted">
              This machine, where Agentry runs.{" "}
              <Link to="/workers" className="text-ink-secondary underline hover:text-ink-primary">
                Add a worker
              </Link>{" "}
              to run it on another machine.
            </p>
          ) : (
            <div className="flex flex-col gap-3">
              {workers.map((worker) => (
                <Checkbox
                  key={worker}
                  checked={draft.workers.includes(worker)}
                  onChange={(on) => toggleWorker(worker, on)}
                  label={
                    <span className="flex items-center gap-2">
                      <span className={worker === LOCAL ? "" : "font-mono"}>{machineName(worker)}</span>
                      {draft.workers.includes(worker) && draft.workers.length > 1 && (
                        <span className="font-mono text-[11px] text-ink-faint">{draft.workers.indexOf(worker) + 1}</span>
                      )}
                    </span>
                  }
                />
              ))}
              <p className="text-[11px] text-ink-muted">The first one online, in the order you pick them. None picked: this machine.</p>
            </div>
          )}
        </Panel>

        <div className="flex flex-wrap items-center gap-2">
          <Button type="submit" variant="primary" icon={<Save />} disabled={busy || !draft.name || !draft.prompt.trim()}>
            {isNew ? "Add agent" : "Save"}
          </Button>
          <Button variant="ghost" onClick={() => navigate(isNew ? "/agents" : `/agents/${encodeURIComponent(draft.name)}`)}>
            Cancel
          </Button>
          {!isNew &&
            (confirmRemove ? (
              <span className="ml-auto flex items-center gap-2 text-[12px] text-ink-secondary">
                Remove {draft.name}?
                <Button size="sm" variant="danger" disabled={busy} onClick={remove}>
                  Remove
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setConfirmRemove(false)}>
                  Keep
                </Button>
              </span>
            ) : (
              <Button variant="danger" icon={<Trash2 />} className="ml-auto" onClick={() => setConfirmRemove(true)}>
                Remove
              </Button>
            ))}
        </div>
      </aside>
    </form>
  );
}

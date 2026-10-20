import { RefObject, useEffect, useRef, useState } from "react";
import { Plus, TriangleAlert } from "lucide-react";
import type { Agent, Policy, ServerConfig, TeamTemplate } from "@/api/types";
import { Button } from "@/components/atoms/Button";
import { Input } from "@/components/atoms/Input";
import { Select } from "@/components/atoms/Select";
import { errorId, Field, FieldError } from "@/components/molecules/Field";
import { MissingProfilesOffer } from "@/components/molecules/MissingProfilesOffer";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { SideSheet } from "@/components/molecules/SideSheet";
import { useToast } from "@/components/organisms/Toasts";
import {
  addCustomTeam,
  agentCount,
  blankTeam,
  changedAgents,
  checkTeam,
  copyTemplate,
  type DraftAgent,
  footerLine,
  missingLine,
  missingProfiles,
  newAgent,
  type Problem,
  replaceDetail,
  replaceQuestion,
  shownName,
  summaryTitle,
  type TeamDraft,
  whyNotCreated,
} from "@/lib/custom-team";
import { missingProfilesOf } from "@/lib/teams";
import { useResource } from "@/live/use-resource";
import { AgentRow, fieldId, toggleId } from "./AgentRow";

// The New team panel (docs/design/custom-teams.md): a side sheet at
// /agents?team=new where you name a team, start it blank or from a copy of
// a template, shape its agents and create it in one request. The rules and
// their words are lib/custom-team's; this is the form around them.

interface TeamPanelProps {
  template?: string; // the id of the template to start from a copy of; none starts blank
  returnTo?: RefObject<HTMLElement | null>; // where focus goes when the panel closes
  onClose: () => void;
  onCreated: (team: string) => Promise<void>; // shows the new team on the page
}

const TITLE = "New team";

export function TeamPanel({ template, returnTo, onClose, onCreated }: TeamPanelProps) {
  const roster = useResource<Agent[]>("/api/agents").data;
  const templates = useResource<TeamTemplate[]>("/api/teams/templates").data;
  const policy = useResource<Policy>("/api/policy").data;
  const engines = useResource<ServerConfig>("/api/config").data?.engines;
  if (!roster || !templates || !policy || !engines) {
    return (
      <SideSheet title={TITLE} returnTo={returnTo} onClose={onClose} footer={<Footer text="Loading…" onClose={onClose} />}>
        <p aria-live="polite" className="text-[13px] text-ink-muted">
          Loading engines and profiles…
        </p>
      </SideSheet>
    );
  }
  return (
    <TeamForm
      roster={roster}
      templates={templates}
      policy={policy}
      engines={engines}
      copied={templates.find((t) => t.id === template)}
      returnTo={returnTo}
      onClose={onClose}
      onCreated={onCreated}
    />
  );
}

interface TeamFormProps {
  roster: Agent[];
  templates: TeamTemplate[];
  policy: Policy;
  engines: string[];
  copied?: TeamTemplate; // the template the form starts from a copy of, or Blank
  returnTo?: RefObject<HTMLElement | null>;
  onClose: () => void;
  onCreated: (team: string) => Promise<void>;
}

// What Start from asks before it replaces agents you changed: the template's id, or "" for Blank.
type Replacing = { to: string; changed: string[] };

function TeamForm({ roster, templates, policy, engines, copied, returnTo, onClose, onCreated }: TeamFormProps) {
  const notify = useToast();
  const [first] = useState(() => startOf(copied, roster));
  const [start, setStart] = useState(first); // how the agents started, to tell what you changed
  const [copy, setCopy] = useState(copied?.id ?? "");
  const [team, setTeam] = useState(first.team);
  const [rows, setRows] = useState(first.agents);
  const [removed, setRemoved] = useState<string[]>([]);
  const [open, setOpen] = useState<string[]>(first.agents.slice(0, 1).map((agent) => agent.key));
  const [replacing, setReplacing] = useState<Replacing | null>(null);
  const [tried, setTried] = useState(false); // Create team was pressed, so problems show
  const [failure, setFailure] = useState(""); // why the server refused, in words
  const [offer, setOffer] = useState<string[] | null>(null); // the profiles a 409 asked for
  const [saving, setSaving] = useState(false);
  const [focusOn, setFocusOn] = useState<{ id: string } | null>(null);

  const draft: TeamDraft = { team, agents: rows.filter((agent) => !removed.includes(agent.key)) };
  const problems = tried ? checkTeam(draft, roster, { templates, policy, engines }) : [];
  const missing = missingProfiles(draft, policy);
  const rowProblems = (key: string) => problems.filter((p) => typeof p.field === "object" && p.field.row === key);
  const problemAt = (field: "team" | "agents") => problems.find((p) => p.field === field)?.message;

  // Focus moves after the render that shows its target: a row opened by a
  // summary link, a new row, Undo.
  useEffect(() => {
    if (!focusOn) return;
    const target = document.getElementById(focusOn.id);
    (target?.querySelector<HTMLElement>("textarea") ?? target)?.focus();
  }, [focusOn]);

  const focus = (id: string) => setFocusOn({ id });
  const change = (key: string, change: Partial<DraftAgent>) =>
    setRows((rows) =>
      rows.map((agent) => (agent.key === key ? { ...agent, ...change } : change.coordinates ? { ...agent, coordinates: false } : agent)),
    );

  // toggle opens a row and closes the others that have no problem, or closes it.
  function toggle(key: string) {
    setOpen((open) => (open.includes(key) ? open.filter((k) => k !== key) : [...open.filter((k) => rowProblems(k).length > 0), key]));
  }

  function startFrom(to: string) {
    const changed = changedAgents(start, draft);
    if (changed.length > 0) setReplacing({ to, changed });
    else replaceWith(to);
  }

  function replaceWith(to: string) {
    const next = startOf(
      templates.find((t) => t.id === to),
      roster,
    );
    // A name you typed stays; the one the last start suggested follows the new one.
    if (team.trim() === "" || team === start.team) setTeam(next.team);
    setStart(next);
    setCopy(to);
    setRows(next.agents);
    setRemoved([]);
    setOpen(next.agents.slice(0, 1).map((agent) => agent.key));
    setReplacing(null);
  }

  function addAgent() {
    const agent = newAgent(draft, engines);
    setRows((rows) => [...rows, agent]);
    setOpen((open) => [...open.filter((k) => rowProblems(k).length > 0), agent.key]);
    focus(fieldId(agent.key, "name"));
  }

  function remove(key: string) {
    setRemoved((removed) => [...removed, key]);
    focus(`${key}-undo`);
  }

  // undo puts a removed row back, no longer the coordinator if another row has become one.
  function undo(key: string) {
    setRemoved((removed) => removed.filter((k) => k !== key));
    if (draft.agents.some((agent) => agent.coordinates)) change(key, { coordinates: false });
    focus(toggleId(key));
  }

  // follow opens the row a summary link names and focuses the field.
  function follow(problem: Problem) {
    if (problem.field === "team") return focus("team-name");
    if (problem.field === "agents") return focus("team-agents");
    const { row, field } = problem.field;
    setOpen((open) => (open.includes(row) ? open : [...open, row]));
    focus(fieldId(row, field));
  }

  async function create(policyHash?: string) {
    setTried(true);
    setFailure("");
    const found = checkTeam(draft, roster, { templates, policy, engines });
    if (found.length > 0) {
      setOffer(null);
      // A row with a problem opens, so its errors show under its fields.
      setOpen((open) => [...new Set([...open, ...found.flatMap((p) => (typeof p.field === "object" ? [p.field.row] : []))])]);
      return focus("team-summary");
    }
    setSaving(true);
    try {
      await addCustomTeam(draft, policyHash);
      notify("success", `Added the ${draft.team.trim()} team`, "Pick it on New task; its coordinator takes the task.");
      await onCreated(draft.team.trim());
    } catch (refusal) {
      const profiles = missingProfilesOf(refusal);
      if (profiles && policyHash === undefined) {
        setOffer(profiles);
        focus("team-offer");
      } else {
        setOffer(null);
        setFailure(whyNotCreated(refusal));
        focus("team-summary");
      }
      setSaving(false);
    }
  }

  const count = draft.agents.length;

  return (
    <SideSheet
      title={TITLE}
      returnTo={returnTo}
      onClose={onClose}
      canClose={!saving}
      footer={<Footer text={footerLine(draft, problems.length, saving)} saving={saving} onClose={onClose} onCreate={() => create()} />}
    >
      <div className="flex flex-col gap-5">
        {(problems.length > 0 || failure) && <ErrorSummary problems={problems} failure={failure} onFollow={follow} />}
        {offer && (
          <div id="team-offer" tabIndex={-1} className="outline-none">
            <MissingProfilesOffer team={draft.team.trim()} profiles={offer} creates adding={saving} onAdd={() => create(policy.hash)} />
          </div>
        )}

        <Field label="Team name" htmlFor="team-name" hint="Shown on the Agents page and on New task." error={problemAt("team")}>
          <Input
            id="team-name"
            autoFocus
            value={team}
            readOnly={saving}
            onChange={(e) => setTeam(e.target.value)}
            {...(problemAt("team") && { "aria-invalid": true, "aria-describedby": errorId("team-name"), className: "border-danger" })}
          />
        </Field>

        <StartFrom
          copy={copy}
          templates={templates}
          saving={saving}
          onChoose={startFrom}
          replacing={replacing}
          onKeep={() => setReplacing(null)}
          onReplace={() => replacing && replaceWith(replacing.to)}
        />

        <fieldset aria-describedby={problemAt("agents") ? errorId("team-agents") : undefined} className="flex min-w-0 flex-col gap-2">
          <legend
            id="team-agents"
            tabIndex={-1}
            className="mb-2 flex w-full items-baseline justify-between text-[12px] font-medium text-ink-secondary outline-none"
          >
            Agents · one coordinates
            <span className="font-normal text-ink-muted">{agentCount(count)}</span>
          </legend>
          {problemAt("agents") && <FieldError id={errorId("team-agents")}>{problemAt("agents")}</FieldError>}
          {missing.length > 0 && (
            <p className="flex items-start gap-1.5 text-[12px] text-warning-ink">
              <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
              <span>{missingLine(draft, missing)}</span>
            </p>
          )}
          <ul className="rounded-md border border-divider bg-surface-1">
            {rows.map((agent) =>
              removed.includes(agent.key) ? (
                <RemovedStub key={agent.key} agent={agent} saving={saving} onUndo={() => undo(agent.key)} />
              ) : (
                <AgentRow
                  key={agent.key}
                  agent={agent}
                  open={open.includes(agent.key)}
                  onToggle={() => toggle(agent.key)}
                  problems={rowProblems(agent.key)}
                  profileMissing={missing.includes(agent.profile.trim())}
                  engines={engines}
                  policy={policy}
                  saving={saving}
                  onChange={(c) => change(agent.key, c)}
                  onRemove={() => remove(agent.key)}
                />
              ),
            )}
          </ul>
          {count === 0 && <p className="text-[12px] text-ink-muted">No agents yet. Add one: it coordinates the team.</p>}
          <span className="flex items-center gap-3">
            <Button
              size="sm"
              variant="ghost"
              icon={<Plus />}
              aria-label="Add agent"
              aria-disabled={saving || undefined}
              onClick={() => !saving && addAgent()}
            >
              Add agent
            </Button>
            {count === 1 && draft.agents[0].coordinates && (
              <span className="text-[12px] text-ink-muted">Add the agents it hands work to.</span>
            )}
          </span>
        </fieldset>
      </div>
    </SideSheet>
  );
}

// startOf is the team a form starts with: a copy of the template, or Blank.
const startOf = (template: TeamTemplate | undefined, roster: Agent[]) => (template ? copyTemplate(template, roster) : blankTeam(roster));

interface ErrorSummaryProps {
  problems: Problem[];
  failure: string;
  onFollow: (problem: Problem) => void;
}

// ErrorSummary is why Create team didn't create the team: a link to each
// problem, in screen order, or the server's words. It takes focus, which
// reads it out, so it is no alert.
function ErrorSummary({ problems, failure, onFollow }: ErrorSummaryProps) {
  return (
    <div
      id="team-summary"
      tabIndex={-1}
      aria-labelledby="team-summary-title"
      className="flex flex-col gap-1.5 rounded-md border border-danger/40 bg-danger-soft px-3 py-2.5 outline-none focus-visible:border-focus"
    >
      <h3 id="team-summary-title" className="text-[13px] font-medium text-danger-ink">
        {summaryTitle(problems.length)}
      </h3>
      {failure && <p className="text-[13px] text-ink-secondary">{failure}</p>}
      {problems.length > 0 && (
        <ul className="flex flex-col gap-1">
          {problems.map((problem) => (
            <li key={problem.summary}>
              <button
                type="button"
                onClick={() => onFollow(problem)}
                className="cursor-pointer text-left text-[13px] text-ink-primary underline hover:no-underline"
              >
                {problem.summary}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

interface StartFromProps {
  copy: string; // the template copied, or "" for Blank
  templates: TeamTemplate[];
  saving: boolean;
  onChoose: (to: string) => void;
  replacing: Replacing | null;
  onKeep: () => void;
  onReplace: () => void;
}

// StartFrom is Blank or Copy a template, and, when that would replace
// agents you changed, the question first, in place.
function StartFrom({ copy, templates, saving, onChoose, replacing, onKeep, onReplace }: StartFromProps) {
  // While it asks, the choice shows what you asked for.
  const chosen = replacing ? replacing.to : copy;
  const copying = chosen !== "";
  const keep = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (replacing) keep.current?.focus();
  }, [replacing]);
  const choose = (segment: string) => {
    if (saving) return;
    if (segment === "blank" && copying) onChoose("");
    if (segment === "copy" && !copying && templates.length > 0) onChoose(templates[0].id);
  };
  const to = replacing ? templates.find((t) => t.id === replacing.to) : undefined;
  const from = templates.find((t) => t.id === copy);
  return (
    <div className="flex flex-col gap-2">
      <Field
        label="Start from"
        labelId="team-start"
        hint={
          copying
            ? "Fills every agent from the template; change any of them below."
            : "A coordinator to start; add the agents it hands work to."
        }
      >
        <div className="flex flex-wrap items-center gap-2">
          <SegmentedControl
            labelledBy="team-start"
            active={copying ? "copy" : "blank"}
            onChange={choose}
            segments={[
              { id: "blank", label: "Blank" },
              { id: "copy", label: "Copy a template" },
            ]}
          />
          {copying && (
            <Select
              aria-label="Template"
              value={chosen}
              aria-disabled={saving || undefined}
              onChange={(e) => !saving && onChoose(e.target.value)}
              className="w-56 max-sm:w-full"
            >
              <optgroup label="Built in">
                {templates.map((template) => (
                  <option key={template.id} value={template.id}>
                    {template.team}
                  </option>
                ))}
              </optgroup>
            </Select>
          )}
        </div>
      </Field>
      {replacing && (
        <div
          role="group"
          aria-labelledby="team-replace"
          className="flex flex-col gap-2 rounded-md border border-divider bg-surface-0 px-3 py-2.5"
        >
          <p id="team-replace" className="text-[13px] font-medium text-ink-primary">
            {replaceQuestion(replacing.changed, to)}
          </p>
          <p className="text-[12px] text-ink-secondary">{replaceDetail(replacing.changed, to, from)}</p>
          <span className="flex gap-2">
            <Button ref={keep} size="sm" onClick={onKeep}>
              Keep mine
            </Button>
            <Button size="sm" variant="ghost" onClick={onReplace}>
              Replace
            </Button>
          </span>
        </div>
      )}
    </div>
  );
}

// RemovedStub stands where a removed agent was, with Undo, while the panel is open.
function RemovedStub({ agent, saving, onUndo }: { agent: DraftAgent; saving: boolean; onUndo: () => void }) {
  const name = shownName(agent);
  return (
    <li className="flex min-h-11 items-center gap-3 border-b border-divider px-4 text-[12px] text-ink-muted last:border-b-0">
      <span className="font-mono">{name} removed</span>
      <Button
        id={`${agent.key}-undo`}
        size="sm"
        variant="ghost"
        aria-label={`Undo, keep ${name}`}
        aria-disabled={saving || undefined}
        onClick={() => !saving && onUndo()}
      >
        Undo
      </Button>
    </li>
  );
}

interface FooterProps {
  text: string;
  saving?: boolean;
  onClose: () => void;
  onCreate?: () => void; // none while what a new team needs loads
}

function Footer({ text, saving = false, onClose, onCreate }: FooterProps) {
  return (
    <div className="flex flex-wrap items-center justify-end gap-x-3 gap-y-2">
      <p aria-live="polite" id="team-footer-text" className="mr-auto text-[12px] text-ink-muted">
        {text}
      </p>
      <Button variant="ghost" disabled={saving} onClick={onClose}>
        Cancel
      </Button>
      <Button
        variant="primary"
        disabled={saving || !onCreate}
        aria-describedby={onCreate ? undefined : "team-footer-text"}
        onClick={onCreate}
      >
        {saving ? "Creating…" : "Create team"}
      </Button>
    </div>
  );
}

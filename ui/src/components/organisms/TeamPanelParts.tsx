import { ReactNode, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Plus, TriangleAlert } from "lucide-react";
import type { Agent, Policy, ServerConfig, TeamTemplate } from "@/api/types";
import { Button } from "@/components/atoms/Button";
import { Input } from "@/components/atoms/Input";
import { Banner } from "@/components/molecules/Banner";
import { errorId, Field, FieldError } from "@/components/molecules/Field";
import {
  agentCount,
  backNotice,
  builtInDescription,
  changedAgents,
  type DraftAgent,
  missingLine,
  noCoordinator,
  noTeamDescription,
  type NeededBy,
  onlyCoordinator,
  type Problem,
  removedNotice,
  removingNotice,
  routingLine,
  rowChange,
  shownName,
  type TeamDraft,
  whyNotLoaded,
} from "@/lib/custom-team";
import { useResource } from "@/live/use-resource";
import { cn } from "@/lib/cn";
import { AgentRow, fieldId, toggleId } from "./AgentRow";

// What the New team and Edit team panels share (docs/design/custom-teams.md):
// the agents list as you shape it, the summary of why saving didn't save,
// the footer and the questions it asks, and a failed load.

// useTeamReads reads what a panel needs: the roster, the templates, the
// policy and the engines, each undefined until it has loaded, and why the
// first that failed with nothing from before to show didn't; Try again
// reads those again.
export function useTeamReads() {
  const roster = useResource<Agent[]>("/api/agents");
  const templates = useResource<TeamTemplate[]>("/api/teams/templates");
  const policy = useResource<Policy>("/api/policy");
  const config = useResource<ServerConfig>("/api/config");
  const failed = [roster, templates, policy, config].filter((read) => read.data === undefined && read.error !== "");
  return {
    roster: roster.data,
    templates: templates.data,
    policy: policy.data,
    engines: config.data?.engines,
    loadError: failed[0] ? whyNotLoaded(failed[0].error) : "",
    onRetry: () => failed.forEach((read) => read.reload()),
    reloadRoster: roster.reload,
  };
}

export type TeamReads = ReturnType<typeof useTeamReads>;

// useTeamRows is a panel's agents list: its rows, those removed (each a
// stub with Undo while the panel is open), those open, where focus goes
// after the next render, and what the live region says.
export function useTeamRows() {
  const [rows, setRows] = useState<DraftAgent[]>([]);
  const [removed, setRemoved] = useState<string[]>([]);
  const [open, setOpen] = useState<string[]>([]);
  const [focusOn, setFocusOn] = useState<{ id: string } | null>(null);
  const [notice, setNotice] = useState(""); // what the live region last said
  const agents = rows.filter((agent) => !removed.includes(agent.key));

  // Focus moves after the render that shows its target: a row opened by a
  // summary link, a new row, Undo.
  useEffect(() => {
    if (!focusOn) return;
    const target = document.getElementById(focusOn.id);
    (target?.querySelector<HTMLElement>("textarea") ?? target)?.focus();
  }, [focusOn]);

  const focus = (id: string) => setFocusOn({ id });
  // say empties the live region first and speaks on the next frame, so the
  // same words twice in a row are read again. Saying again drops what the
  // last frame was going to say.
  const speaking = useRef(0);
  const say = (text: string) => {
    cancelAnimationFrame(speaking.current);
    setNotice("");
    speaking.current = requestAnimationFrame(() => setNotice(text));
  };
  useEffect(() => () => cancelAnimationFrame(speaking.current), []);

  // start puts these agents in the list, none removed, the opened ones open.
  function start(agents: DraftAgent[], opened: string[] = []) {
    setRows(agents);
    setRemoved([]);
    setOpen(opened);
  }

  const change = (key: string, change: Partial<DraftAgent>) =>
    setRows((rows) =>
      rows.map((agent) => (agent.key === key ? { ...agent, ...change } : change.coordinates ? { ...agent, coordinates: false } : agent)),
    );

  // toggle opens a row and closes the others that have no problem, or closes it.
  function toggle(key: string, hasProblems: (key: string) => boolean) {
    setOpen((open) => (open.includes(key) ? open.filter((k) => k !== key) : [...open.filter(hasProblems), key]));
  }

  // add puts a new row at the end, open, with focus in its Name.
  function add(agent: DraftAgent, hasProblems: (key: string) => boolean) {
    setRows((rows) => [...rows, agent]);
    setOpen((open) => [...open.filter(hasProblems), agent.key]);
    focus(fieldId(agent.key, "name"));
  }

  function remove(agent: DraftAgent, notice: string) {
    setRemoved((removed) => [...removed, agent.key]);
    say(notice);
    focus(undoId(agent.key));
  }

  // undo puts a removed row back, no longer the coordinator if another row has become one.
  function undo(agent: DraftAgent) {
    setRemoved((removed) => removed.filter((k) => k !== agent.key));
    if (agents.some((other) => other.coordinates)) change(agent.key, { coordinates: false });
    say(backNotice(agent));
    focus(toggleId(agent.key));
  }

  // openRows opens the rows with these problems, so their errors show under their fields.
  const openRows = (problems: Problem[]) =>
    setOpen((open) => [...new Set([...open, ...problems.flatMap((p) => (typeof p.field === "object" ? [p.field.row] : []))])]);

  // follow opens the row a summary link names and focuses the field.
  function follow(problem: Problem) {
    if (problem.field === "team") return focus("team-name");
    if (problem.field === "agents") return focus("team-agents");
    const { row, field } = problem.field;
    setOpen((open) => (open.includes(row) ? open : [...open, row]));
    focus(fieldId(row, field));
  }

  return { rows, removed, open, notice, agents, focus, say, start, change, toggle, add, remove, undo, openRows, follow };
}

export type TeamRows = ReturnType<typeof useTeamRows>;

export const undoId = (row: string) => `${row}-undo`;

interface TeamDescriptionProps {
  list: TeamRows;
  team: string; // as typed
  saving: boolean;
  builtIn?: Pick<TeamTemplate, "team" | "description">; // the built-in template the team was made from, whose description the server uses instead
}

// TeamDescription is What this team does: the coordinator's description,
// one value that its row's Description field changes too, and the line
// other coordinators read about the team. Empty, it warns without stopping
// a save.
export function TeamDescription({ list, team, saving, builtIn }: TeamDescriptionProps) {
  const coordinators = list.agents.filter((agent) => agent.coordinates);
  const coordinator = coordinators.length === 1 ? coordinators[0] : undefined;
  const description = coordinator?.description ?? "";
  const read = builtIn?.description || description;
  const warns = coordinator !== undefined && read.trim() === "";
  return (
    <div className="flex flex-col gap-2">
      <Field
        label="What this team does"
        htmlFor="team-description"
        hint={
          !coordinator
            ? noCoordinator
            : builtIn
              ? builtInDescription(builtIn.team)
              : "One line. Other coordinators read it to choose what to send this team."
        }
      >
        <Input
          id="team-description"
          value={description}
          disabled={!coordinator}
          readOnly={saving}
          onChange={(e) => coordinator && list.change(coordinator.key, { description: e.target.value })}
          aria-describedby={warns ? "team-description-warning" : undefined}
        />
      </Field>
      {warns && (
        <p id="team-description-warning" className="flex items-start gap-1.5 text-[12px] text-warning-ink">
          <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
          <span>{noTeamDescription}</span>
        </p>
      )}
      <div className="flex flex-col gap-0.5 text-[12px]">
        <span className="text-ink-muted">Other coordinators read:</span>
        <span id="team-routing-line" className="font-mono break-words text-ink-secondary">
          {routingLine(team, coordinator?.name ?? "", read)}
        </span>
      </div>
    </div>
  );
}

interface TeamAgentsProps {
  list: TeamRows;
  loaded?: TeamDraft; // the team as saved, when editing one
  problems: Problem[];
  missing: string[]; // the profiles policy.toml lacks that saving offers to add
  engines: string[];
  policy: Policy;
  saving: boolean;
  builtIn?: string; // the built-in template the team was made from
  onAdd: () => void;
}

// TeamAgents is the panel's agents: one radio group across the rows, each
// row open in place or as a stub with Undo, then Add agent. Editing a team,
// each row says how it differs from the team as saved.
export function TeamAgents({ list, loaded, problems, missing, engines, policy, saving, builtIn, onAdd }: TeamAgentsProps) {
  const draft = { team: "", agents: list.agents };
  const count = list.agents.length;
  const changed = loaded ? changedAgents(loaded, draft).length : 0;
  const rowProblems = (key: string) => problems.filter((p) => typeof p.field === "object" && p.field.row === key);
  // A removed agent tasks still need says so on its stub, not here.
  const legendProblem = problems.find((p) => p.field === "agents" && !p.removed)?.message;
  const savedName = (agent: DraftAgent) => loaded?.agents.find((row) => row.key === agent.key)?.name;
  const coordinators = list.agents.filter((agent) => agent.coordinates).length;
  return (
    <fieldset aria-describedby={legendProblem ? errorId("team-agents") : undefined} className="flex min-w-0 flex-col gap-2">
      <legend
        id="team-agents"
        tabIndex={-1}
        className="mb-2 flex w-full items-baseline justify-between text-[12px] font-medium text-ink-secondary outline-none"
      >
        Agents · one coordinates
        <span className="font-normal text-ink-muted">
          {agentCount(count)}
          {changed > 0 && ` · ${changed} changed`}
        </span>
      </legend>
      {legendProblem && <FieldError id={errorId("team-agents")}>{legendProblem}</FieldError>}
      {missing.length > 0 && (
        <p className="flex items-start gap-1.5 text-[12px] text-warning-ink">
          <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
          <span>{missingLine(draft, missing, loaded ? "Save changes" : "Create team")}</span>
        </p>
      )}
      <ul className="rounded-md border border-divider bg-surface-1">
        {list.rows.map((agent) => {
          if (list.removed.includes(agent.key)) {
            const name = savedName(agent) ?? shownName(agent);
            return (
              <RemovedStub
                key={agent.key}
                row={agent.key}
                name={name}
                editing={Boolean(loaded)}
                needed={problems.find((p) => p.removed?.agent === name)?.removed}
                saving={saving}
                onUndo={() => list.undo(agent)}
              />
            );
          }
          return (
            <AgentRow
              key={agent.key}
              agent={agent}
              open={list.open.includes(agent.key)}
              onToggle={() => list.toggle(agent.key, (key) => rowProblems(key).length > 0)}
              problems={rowProblems(agent.key)}
              profileMissing={missing.includes(agent.profile.trim())}
              engines={engines}
              policy={policy}
              saving={saving}
              onChange={(c) => list.change(agent.key, c)}
              onRemove={() => list.remove(agent, loaded ? removingNotice(savedName(agent) ?? shownName(agent)) : removedNotice(agent))}
              edit={loaded && { ...rowChange(loaded, agent), saved: savedName(agent) }}
              unpicked={coordinators > 1}
              builtIn={builtIn}
            />
          );
        })}
      </ul>
      {count === 0 && <p className="text-[12px] text-ink-muted">No agents yet. Add one: it coordinates the team.</p>}
      <span className="flex items-center gap-3">
        <Button
          size="sm"
          variant="ghost"
          icon={<Plus />}
          aria-label="Add agent"
          aria-disabled={saving || undefined}
          onClick={() => !saving && onAdd()}
        >
          Add agent
        </Button>
        {count === 1 && list.agents[0].coordinates && <span className="text-[12px] text-ink-muted">{onlyCoordinator}</span>}
      </span>
    </fieldset>
  );
}

interface RemovedStubProps {
  row: string;
  name: string;
  editing: boolean; // the agent goes when the team is saved
  needed?: NeededBy; // the unfinished tasks that refused its removal
  saving: boolean;
  onUndo: () => void;
}

// RemovedStub stands where a removed agent was, with Undo, while the panel
// is open, and says which tasks still need it when saving was refused.
function RemovedStub({ row, name, editing, needed, saving, onUndo }: RemovedStubProps) {
  return (
    <li className="flex min-h-11 flex-wrap items-center gap-x-3 border-b border-divider px-4 py-1 text-[12px] text-ink-muted last:border-b-0">
      <span className="font-mono">{editing ? `${name} will be removed when you save` : `${name} removed`}</span>
      <Button
        id={undoId(row)}
        size="sm"
        variant="ghost"
        aria-label={`Undo, keep ${name}`}
        aria-disabled={saving || undefined}
        onClick={() => !saving && onUndo()}
      >
        Undo
      </Button>
      {needed && <StillNeeded agent={name} needed={needed} />}
    </li>
  );
}

// StillNeeded is a refused removal on its stub, each task a link to it.
function StillNeeded({ agent, needed: { tasks, turns } }: { agent: string; needed: NeededBy }) {
  const links = tasks.map((task) => (
    <Link key={task} to={`/tasks/${task}`} className="font-mono text-ink-primary underline hover:no-underline">
      {task}
    </Link>
  ));
  const joined = links.flatMap((link, i) => (i === 0 ? [link] : [i === links.length - 1 ? " and " : ", ", link]));
  const it = tasks.length === 1 ? "it" : "them";
  return (
    <p className="basis-full pb-1.5 text-danger-ink">
      {turns ? (
        <>
          {tasks.length === 1 ? "A turn" : "Turns"} running on {joined}. Wait for {it} to end, or undo to keep {agent}.
        </>
      ) : (
        <>
          Still needed by {joined}. Finish {it} first, or undo to keep {agent}.
        </>
      )}
    </p>
  );
}

interface ErrorSummaryProps {
  title: string;
  problems: Problem[];
  failure: string;
  onFollow: (problem: Problem) => void;
}

// ErrorSummary is why saving didn't save the team: a link to each problem,
// in screen order, or the server's words. It takes focus, which reads it
// out, so it is no alert.
export function ErrorSummary({ title, problems, failure, onFollow }: ErrorSummaryProps) {
  return (
    <div
      id="team-summary"
      tabIndex={-1}
      aria-labelledby="team-summary-title"
      className="flex flex-col gap-1.5 rounded-md border border-danger/40 bg-danger-soft px-3 py-2.5 outline-none focus-visible:border-focus"
    >
      <h3 id="team-summary-title" className="text-[13px] font-medium text-danger-ink">
        {title}
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

interface FooterProps {
  text?: string; // beside the buttons: what the primary does, or why it can't yet
  saving?: boolean;
  off?: boolean; // the primary is off for the reason in text: loading, or nothing to save
  primary?: { label: string; busy: string; onClick: () => void }; // none when the load failed: there is nothing to save
  onCancel: () => void;
  locked?: boolean; // both buttons off while something else runs, as removing the team
  start?: ReactNode; // the team's own actions, on the far side; not on a phone
}

// Footer is Cancel and the panel's primary button. On a phone it is the two
// buttons alone, side by side; the line beside them is still read out.
export function Footer({ text = "", saving = false, off = false, primary, onCancel, locked = false, start }: FooterProps) {
  return (
    <div className="flex flex-wrap items-center justify-end gap-x-3 gap-y-2 max-sm:flex-nowrap">
      {start && <div className="mr-auto flex items-center gap-x-3 max-sm:hidden">{start}</div>}
      <p aria-live="polite" id="team-footer-text" className={cn("text-[12px] text-ink-muted max-sm:sr-only", !start && "mr-auto")}>
        {text}
      </p>
      <Button id="team-cancel" variant="ghost" disabled={saving || locked} onClick={onCancel} className={PHONE_BUTTON}>
        Cancel
      </Button>
      {primary && (
        <Button
          variant="primary"
          disabled={saving || off || locked}
          aria-describedby={off && !saving ? "team-footer-text" : undefined}
          onClick={primary.onClick}
          className={PHONE_BUTTON}
        >
          {saving ? primary.busy : primary.label}
        </Button>
      )}
    </div>
  );
}

// On a phone the footer's two buttons share its one 44px row.
const PHONE_BUTTON = "max-sm:h-11 max-sm:flex-1";

interface DiscardQuestionProps {
  question: string;
  detail: string;
  discard?: string; // the danger answer, "Discard and open" for a link
  onKeep: () => void;
  onDiscard: () => void;
}

// DiscardQuestion takes the footer's place when closing, or following a
// link, would lose what you did: a group named by the question, starting
// on the safe answer.
export function DiscardQuestion({ question, detail, discard = "Discard", onKeep, onDiscard }: DiscardQuestionProps) {
  const keep = useRef<HTMLButtonElement>(null);
  useEffect(() => keep.current?.focus(), []);
  return (
    <div
      role="group"
      aria-labelledby="team-discard"
      aria-describedby="team-discard-detail"
      className="flex flex-wrap items-center justify-end gap-x-3 gap-y-2"
    >
      <div className="mr-auto min-w-0 max-sm:basis-full">
        <p id="team-discard" className="text-[13px] font-medium text-ink-primary">
          {question}
        </p>
        <p id="team-discard-detail" className="text-[12px] text-ink-secondary">
          {detail}
        </p>
      </div>
      <Button ref={keep} onClick={onKeep} className={PHONE_BUTTON}>
        Keep editing
      </Button>
      <Button variant="danger" onClick={onDiscard} className={PHONE_BUTTON}>
        {discard}
      </Button>
    </div>
  );
}

// LoadFailure says what didn't load, in the server's words or ours when it
// couldn't be reached, and tries again. Try again takes focus.
export function LoadFailure({ title, error, onRetry }: { title: string; error: string; onRetry: () => void }) {
  const retry = useRef<HTMLButtonElement>(null);
  useEffect(() => retry.current?.focus(), []);
  return (
    <Banner title={title}>
      <p>{error}</p>
      <Button ref={retry} size="sm" onClick={onRetry} className="mt-2">
        Try again
      </Button>
    </Banner>
  );
}

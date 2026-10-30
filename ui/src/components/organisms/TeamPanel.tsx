import { RefObject, useEffect, useRef, useState } from "react";
import type { Agent, TeamTemplate } from "@/api/types";
import { Button } from "@/components/atoms/Button";
import { Input } from "@/components/atoms/Input";
import { Select } from "@/components/atoms/Select";
import { errorId, Field } from "@/components/molecules/Field";
import { MissingProfilesOffer } from "@/components/molecules/MissingProfilesOffer";
import { SegmentedControl } from "@/components/molecules/SegmentedControl";
import { SideSheet } from "@/components/molecules/SideSheet";
import { useToast } from "@/shell/Toasts";
import {
  addCustomTeam,
  blankNotice,
  blankTeam,
  changedAgents,
  checkTeam,
  copyTemplate,
  discardDetail,
  discardQuestion,
  FIRST_TEMPLATE,
  footerLine,
  loadFailed,
  loadingAgents,
  loadingTemplates,
  missingProfiles,
  newAgent,
  replacedNotice,
  replaceDetail,
  replaceQuestion,
  summaryTitle,
  type TeamDraft,
  templateToCopy,
  unsaved,
  whyNotCreated,
} from "@/lib/custom-team";
import { useCloseGuard } from "@/lib/close-guard";
import { missingProfilesOf } from "@/lib/teams";
import { templateGroups } from "@/lib/templates";
import {
  DiscardQuestion,
  ErrorSummary,
  Footer,
  LoadFailure,
  TeamAgents,
  TeamDescription,
  type TeamReads,
  useTeamReads,
  useTeamRows,
} from "./TeamPanelParts";

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

// TeamPanel reads what a new team needs; the form works while it loads.
export function TeamPanel(props: TeamPanelProps) {
  return <TeamForm {...props} {...useTeamReads()} />;
}

type TeamFormProps = TeamPanelProps & TeamReads;

// What Start from asks before it replaces agents you changed: the template's id, or "" for Blank.
type Replacing = { to: string; changed: string[] };

// How a team starts before its agents can: no name and no agents.
const NOTHING_YET: TeamDraft = { team: "", agents: [] };

function TeamForm(props: TeamFormProps) {
  const { roster, templates, policy, engines, template, loadError, onRetry, returnTo, onClose, onCreated } = props;
  const notify = useToast();
  const ready = roster && templates && policy && engines ? { roster, templates, policy, engines } : undefined;
  const [start, setStart] = useState<TeamDraft | null>(null); // how the agents started, to tell what you changed
  const [copy, setCopy] = useState(template ?? "");
  const [team, setTeam] = useState("");
  const list = useTeamRows();
  const [replacing, setReplacing] = useState<Replacing | null>(null);
  const [tried, setTried] = useState(false); // Create team was pressed, so problems show
  const [failure, setFailure] = useState(""); // why the server refused, in words
  const [offer, setOffer] = useState<string[] | null>(null); // the profiles a 409 asked for
  const [saving, setSaving] = useState(false);

  // The agents start once everything they need is in. A name typed meanwhile stays.
  if (ready && !start) {
    const copied = templateToCopy(ready.templates, copy);
    const first = startOf(copied, ready.roster);
    setStart(first);
    setCopy(copied?.id ?? "");
    list.start(
      first.agents,
      first.agents.slice(0, 1).map((agent) => agent.key),
    );
    if (team.trim() === "") setTeam(first.team);
  }

  const draft: TeamDraft = { team, agents: list.agents, copiedFrom: start?.copiedFrom };
  const problems = tried && ready ? checkTeam(draft, ready.roster, ready) : [];
  const missing = policy ? missingProfiles(draft, policy) : [];
  const guard = useCloseGuard(unsaved(start ?? NOTHING_YET, draft), saving, onClose);
  const hasProblems = (key: string) => problems.some((p) => typeof p.field === "object" && p.field.row === key);
  const teamProblem = problems.find((p) => p.field === "team")?.message;

  function startFrom(to: string) {
    if (!start) return setCopy(to); // the agents start from it once they can
    const changed = changedAgents(start, draft);
    if (changed.length > 0) setReplacing({ to, changed });
    else replaceWith(to);
  }

  function replaceWith(to: string) {
    if (!ready) return;
    const template = templateToCopy(ready.templates, to);
    const next = startOf(template, ready.roster);
    // A name you typed stays; the one the last start suggested follows the new one.
    if (team.trim() === "" || team === start?.team) setTeam(next.team);
    setStart(next);
    setCopy(to);
    list.start(
      next.agents,
      next.agents.slice(0, 1).map((agent) => agent.key),
    );
    setReplacing(null);
    list.say(template ? replacedNotice(template) : blankNotice);
  }

  async function create(policyHash?: string) {
    if (!ready) return;
    setTried(true);
    setFailure("");
    const found = checkTeam(draft, ready.roster, ready);
    if (found.length > 0) {
      setOffer(null);
      list.openRows(found);
      return list.focus("team-summary");
    }
    setSaving(true);
    try {
      await addCustomTeam(draft, policyHash);
      notify("success", `Added the ${draft.team.trim()} team`, "Pick it on New task; its coordinator takes the task.");
      await onCreated(draft.team.trim());
      guard.leave();
    } catch (refusal) {
      const profiles = missingProfilesOf(refusal);
      if (profiles && policyHash === undefined) {
        setOffer(profiles);
        list.focus("team-offer");
      } else {
        setOffer(null);
        setFailure(whyNotCreated(refusal));
        list.focus("team-summary");
      }
      setSaving(false);
    }
  }

  const footer = guard.asking ? (
    <DiscardQuestion question={discardQuestion} detail={discardDetail} onKeep={guard.keepEditing} onDiscard={guard.leave} />
  ) : loadError ? (
    <Footer onCancel={guard.ask} />
  ) : (
    <Footer
      text={ready ? footerLine(draft, problems.length, saving) : "Loading…"}
      saving={saving}
      off={!ready}
      primary={{ label: "Create team", busy: "Creating…", onClick: () => create() }}
      onCancel={guard.ask}
    />
  );

  return (
    <SideSheet title={TITLE} returnTo={returnTo} onClose={guard.ask} canClose={!saving} footer={footer}>
      {/* On a phone every input and select in the panel is 44px tall, for thumbs. */}
      <div className="flex flex-col gap-5 max-sm:[&_[data-field]]:h-11">
        <p aria-live="polite" className="sr-only">
          {list.notice}
        </p>
        {loadError && <LoadFailure title={loadFailed} error={loadError} onRetry={onRetry} />}
        {(problems.length > 0 || failure) && (
          <ErrorSummary title={summaryTitle(problems.length)} problems={problems} failure={failure} onFollow={list.follow} />
        )}
        {offer && policy && (
          <div id="team-offer" tabIndex={-1} className="outline-none">
            <MissingProfilesOffer
              team={draft.team.trim()}
              profiles={offer}
              then="create"
              adding={saving}
              onAdd={() => create(policy.hash)}
            />
          </div>
        )}

        <Field label="Team name" htmlFor="team-name" hint="Shown on the Agents page and on New task." error={teamProblem}>
          <Input
            id="team-name"
            autoFocus
            value={team}
            readOnly={saving}
            onChange={(e) => setTeam(e.target.value)}
            {...(teamProblem && { "aria-invalid": true, "aria-describedby": errorId("team-name"), className: "border-danger" })}
          />
        </Field>

        {ready && <TeamDescription list={list} team={team} saving={saving} />}

        <StartFrom
          copy={copy}
          templates={templates}
          saving={saving}
          onChoose={startFrom}
          replacing={replacing}
          onKeep={() => setReplacing(null)}
          onReplace={() => replacing && replaceWith(replacing.to)}
        />

        {!ready ? (
          !loadError && (
            <p aria-live="polite" className="text-[13px] text-ink-muted">
              {loadingAgents}
            </p>
          )
        ) : (
          <TeamAgents
            list={list}
            problems={problems}
            missing={missing}
            engines={ready.engines}
            policy={ready.policy}
            saving={saving}
            onAdd={() => list.add(newAgent(draft, ready.engines), hasProblems)}
          />
        )}
      </div>
    </SideSheet>
  );
}

// startOf is the team a form starts with: a copy of the template, or Blank.
const startOf = (template: TeamTemplate | undefined, roster: Agent[]) => (template ? copyTemplate(template, roster) : blankTeam(roster));

interface StartFromProps {
  copy: string; // the template copied, or "" for Blank
  templates?: TeamTemplate[]; // none while they load
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
    if (segment === "copy" && !copying) onChoose(templates ? (templates[0]?.id ?? "") : FIRST_TEMPLATE);
  };
  const to = replacing && templates ? templateToCopy(templates, replacing.to) : undefined;
  const from = templates && templateToCopy(templates, copy);
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
          {copying && !templates && (
            <Select aria-label="Template" disabled className="w-56 max-sm:w-full">
              <option>{loadingTemplates}</option>
            </Select>
          )}
          {copying && templates && (
            <Select
              aria-label="Template"
              value={chosen}
              aria-disabled={saving || undefined}
              onChange={(e) => !saving && onChoose(e.target.value)}
              className="w-56 max-sm:w-full"
            >
              <TemplateOptions templates={templates} />
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

// TemplateOptions are the templates in their groups: Your templates, newest
// first, only when you have one, then Built in.
function TemplateOptions({ templates }: { templates: TeamTemplate[] }) {
  const { yours, builtIn } = templateGroups(templates);
  const options = (group: TeamTemplate[]) =>
    group.map((template) => (
      <option key={template.id} value={template.id}>
        {template.team}
      </option>
    ));
  return (
    <>
      {yours.length > 0 && <optgroup label="Your templates">{options(yours)}</optgroup>}
      <optgroup label="Built in">{options(builtIn)}</optgroup>
    </>
  );
}

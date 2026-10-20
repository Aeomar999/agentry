import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/atoms/Button";
import { Input } from "@/components/atoms/Input";
import { errorId, Field } from "@/components/molecules/Field";
import { MissingProfilesOffer } from "@/components/molecules/MissingProfilesOffer";
import { SideSheet } from "@/components/molecules/SideSheet";
import { useToast } from "@/components/organisms/Toasts";
import {
  checkTeam,
  discardEditDetail,
  discardEditQuestion,
  editCustomTeam,
  editFooterLine,
  editTitle,
  type EditRefusal,
  type LoadedTeam,
  loadingTeam,
  loadTeam,
  missingProfiles,
  newAgent,
  notFoundDetail,
  notFoundTitle,
  openQuestion,
  type Problem,
  reloadQuestion,
  saveAgain,
  summaryTitle,
  type TeamDraft,
  teamLoadFailed,
  unsaved,
  whyNotCreated,
  whyNotSaved,
} from "@/lib/custom-team";
import { useCloseGuard } from "@/lib/close-guard";
import { teamsOf } from "@/lib/teams";
import { builtInTemplateOf, isBuiltInTeam, teamLink, yourLinkedTemplate } from "@/lib/templates";
import { RemoveTeamButton, RemoveTeamQuestion, useRemoveTeam } from "./RemoveTeam";
import {
  LinkTeamOffer,
  SaveTemplateButton,
  SaveTemplateForm,
  UpdateTemplateButton,
  UpdateTemplateRefusal,
  useSaveTemplate,
  useUpdateTemplate,
} from "./SaveAsTemplate";
import {
  DiscardQuestion,
  ErrorSummary,
  Footer,
  LoadFailure,
  TeamAgents,
  TeamDescription,
  type TeamReads,
  undoId,
  useTeamReads,
  useTeamRows,
} from "./TeamPanelParts";

// The Edit team panel (docs/design/custom-teams.md, "2. Edit a team"): a
// team you have, at /agents?team=<name>, in the New team panel's rows.
// Changes are a draft until Save changes saves the whole team in one
// request, against the version the panel loaded; a refusal keeps
// everything typed and says what to do about it.

interface EditTeamPanelProps {
  team: string; // as the link names it, in any case
  adding?: boolean; // + Add agent: the team opens with a new row added and open
  onClose: () => void;
  onSaved: (team: string) => Promise<void>; // shows the team saved on the page
  onRemoved: (team: string) => Promise<void>; // shows the page without the team, as it was saved
}

const TITLE_ID = "team-panel-title";

// EditTeamPanel reads what the team's form needs; nothing shows until it has.
export function EditTeamPanel(props: EditTeamPanelProps) {
  return <EditTeamForm {...props} {...useTeamReads()} />;
}

type EditTeamFormProps = EditTeamPanelProps & TeamReads;

// A refusal the panel shows in a box of its own, rather than as problems
// or the missing-profile offer.
type Refused = Extract<EditRefusal, { kind: "changed" | "cut short" | "failed" }>;

function EditTeamForm(props: EditTeamFormProps) {
  const { roster, templates, policy, engines, loadError, onRetry, reloadRoster, adding, onClose, onSaved, onRemoved } = props;
  const notify = useToast();
  const ready = roster && templates && policy && engines ? { roster, templates, policy, engines } : undefined;
  // The team to load once everything is in, and whether focus then goes to it.
  const [toLoad, setToLoad] = useState<{ team: string; refocus: boolean } | null>({ team: props.team, refocus: false });
  const [reloading, setReloading] = useState(false);
  const [loaded, setLoaded] = useState<LoadedTeam | null>(null);
  const [team, setTeam] = useState("");
  const list = useTeamRows();
  const [tried, setTried] = useState(false); // Save changes was pressed, so problems show
  const [serverProblems, setServerProblems] = useState<Problem[]>([]);
  const [refused, setRefused] = useState<Refused | null>(null);
  const [offer, setOffer] = useState<string[] | null>(null); // the profiles a 409 asked for
  const [saving, setSaving] = useState(false);

  // The team loads as it is now once everything is in: on opening, and
  // after a reload. + Add agent adds its row to the team as it first loads.
  // No team by that name: the title says so, and takes focus.
  if (ready && toLoad !== null) {
    const fresh = loadTeam(ready.roster, toLoad.team);
    setToLoad(null);
    setLoaded(fresh);
    setTeam(fresh.team);
    setTried(false);
    setServerProblems([]);
    list.start(fresh.agents);
    if (fresh.agents.length === 0) list.focus(TITLE_ID);
    else if (adding && !loaded) list.add(newAgent(fresh, ready.engines), () => false);
    else if (toLoad.refocus) list.focus("team-name");
  }

  const draft: TeamDraft = { team, agents: list.agents };
  const found = ready && loaded ? checkTeam(draft, ready.roster, ready, loaded) : [];
  // Two coordinators, as a team made agent by agent can have, are said at once.
  const twoCoordinators = list.agents.filter((agent) => agent.coordinates).length > 1;
  // A removed agent tasks needed is no problem once it is back.
  const stillRemoved = (loaded?.agents ?? []).filter((row) => list.removed.includes(row.key)).map((row) => row.name);
  const problems = tried
    ? [...found, ...serverProblems.filter((p) => !p.removed || stillRemoved.includes(p.removed.agent))]
    : found.filter((p) => twoCoordinators && p.field === "agents");
  const missing = policy ? missingProfiles(draft, policy) : [];
  const changed = loaded !== null && unsaved(loaded, draft);
  const removal = useRemoveTeam(loaded?.team ?? props.team, roster, list.focus, async (removed) => {
    await onRemoved(removed);
    guard.leave();
  });
  const coordinator = loaded?.agents.find((row) => row.coordinates);
  const linked = roster ? teamLink(roster, loaded?.team ?? props.team) : undefined;
  const template = useSaveTemplate(loaded?.team ?? props.team, coordinator?.description ?? "", changed, linked, list.focus);
  const update = useUpdateTemplate(loaded?.team ?? props.team, changed, list.focus);
  const running = saving || removal.busy || template.busy || template.linking || update.busy;
  const guard = useCloseGuard(changed, running, onClose);
  const onlyTeam = ready !== undefined && teamsOf(ready.roster).length <= 1;
  const hasProblems = (key: string) => problems.some((p) => typeof p.field === "object" && p.field.row === key);
  const teamProblem = problems.find((p) => p.field === "team")?.message;
  const notFound = loaded !== null && loaded.agents.length === 0;
  // A team under a built-in template's name can't be saved as a template.
  const savesAsTemplate = ready !== undefined && loaded !== null && !isBuiltInTeam(ready.templates, loaded.team);
  // Other coordinators read a built-in template's description, when the team was made from one, over its coordinator's.
  const builtIn = ready && coordinator && builtInTemplateOf(ready.roster, ready.templates, coordinator.name);
  // A team linked to a template of yours you still have can update it.
  const updatesTemplate =
    ready !== undefined && loaded !== null && yourLinkedTemplate(ready.roster, ready.templates, loaded.team) !== undefined;

  // reload reads the roster again, then loads the team named from it.
  async function reload(name: string, refocus: boolean) {
    setReloading(true);
    await reloadRoster();
    setReloading(false);
    setToLoad({ team: name, refocus });
  }

  // follow takes a summary link to its field, or to the stub of a removed
  // agent tasks still need, where Undo keeps it.
  function follow(problem: Problem) {
    const stub = problem.removed && loaded?.agents.find((row) => row.name === problem.removed?.agent);
    if (stub) list.focus(undoId(stub.key));
    else list.follow(problem);
  }

  async function save(policyHash?: string) {
    if (!ready || !loaded) return;
    setTried(true);
    setServerProblems([]);
    setRefused(null);
    const now = checkTeam(draft, ready.roster, ready, loaded);
    if (now.length > 0) {
      setOffer(null);
      list.openRows(now);
      return list.focus("team-summary");
    }
    await sending(() => editCustomTeam(loaded, draft, policyHash), loaded, draft, policyHash);
  }

  // sending saves the team, closing the panel once it is saved, or says why it wasn't.
  async function sending(send: () => Promise<unknown>, from: LoadedTeam, sent: TeamDraft, policyHash?: string) {
    setSaving(true);
    try {
      await send();
    } catch (failure) {
      setSaving(false);
      setOffer(null);
      const why = whyNotSaved(failure, from, sent);
      if (why.kind === "missing profiles" && policyHash === undefined) {
        setOffer(why.profiles);
        return list.focus("team-offer");
      }
      if (why.kind === "problems") {
        setServerProblems(why.problems);
        list.openRows(why.problems);
      } else if (why.kind === "missing profiles") {
        setRefused({ kind: "failed", title: summaryTitle(0, true), detail: whyNotCreated(failure) });
      } else {
        setRefused(why);
        // The team is saved: its rows have their new names now.
        if (why.kind === "cut short") reload(sent.team.trim(), false);
      }
      list.focus("team-summary");
      return;
    }
    notify("success", `Saved the ${sent.team.trim()} team`);
    await onSaved(sent.team.trim());
    guard.leave();
  }

  const loading = !ready || !loaded || reloading;
  // A link followed with changes asks about where it goes instead.
  const opening = openQuestion(guard.opening);
  // Remove team's question, in the footer or under This team. One question
  // at a time: Back or a link held while it asks puts the close question in
  // its place until that is answered.
  const removeAsks = removal.asking !== null && !guard.asking;
  const removeHolds = removeAsks || removal.busy;
  // Save as a template's form, in the same places, the same way.
  const templateAsks = template.asking !== null && !guard.asking;
  const templateHolds = templateAsks || template.busy || template.linking;
  // Nothing else saves while any of them asks or runs, Update my template too.
  const holds = removeHolds || templateHolds || update.busy;
  // Update my template's refusal is old once anything else asks or runs.
  if (update.refused && (saving || removeHolds || templateHolds)) update.forget();
  const removeQuestion = (place: "footer" | "body") =>
    loaded && (
      <RemoveTeamQuestion
        place={place}
        removal={removal}
        team={loaded.team}
        agents={loaded.agents.length}
        changed={changed}
        saving={saving}
      />
    );
  const templateForm = (place: "footer" | "body") => <SaveTemplateForm place={place} template={template} changed={changed} />;
  const footer = guard.asking ? (
    <DiscardQuestion
      question={opening?.question ?? discardEditQuestion(loaded?.team ?? props.team)}
      detail={opening ? opening.detail : loaded ? discardEditDetail(loaded, draft) : ""}
      discard={opening && "Discard and open"}
      onKeep={guard.keepEditing}
      onDiscard={guard.leave}
    />
  ) : notFound ? undefined : loadError ? (
    <Footer onCancel={guard.ask} />
  ) : (
    // While Remove team asks, or Save as a template's form is open, it takes
    // the footer's place; on a phone it is under This team, and the footer
    // stays, off.
    <>
      {removeAsks && <div className="max-sm:hidden">{removeQuestion("footer")}</div>}
      {templateAsks && <div className="max-sm:hidden">{templateForm("footer")}</div>}
      {updatesTemplate && !removeAsks && !templateAsks && (
        <div className="max-sm:hidden">
          <UpdateTemplateRefusal place="footer" update={update} />
        </div>
      )}
      <div className={removeAsks || templateAsks ? "sm:hidden" : undefined}>
        <Footer
          text={loading ? "Loading…" : editFooterLine(tried ? problems.length : 0, saving, changed)}
          saving={saving}
          off={loading || !changed}
          locked={holds}
          primary={{ label: "Save changes", busy: "Saving…", onClick: () => save() }}
          onCancel={guard.ask}
          start={
            !loading && (
              <>
                <RemoveTeamButton place="footer" removal={removal} only={onlyTeam} off={saving || templateHolds || update.busy} />
                {savesAsTemplate && (
                  <SaveTemplateButton place="footer" template={template} changed={changed} off={saving || removeHolds || update.busy} />
                )}
                {updatesTemplate && (
                  <UpdateTemplateButton place="footer" update={update} changed={changed} off={saving || removeHolds || templateHolds} />
                )}
              </>
            )
          }
        />
      </div>
    </>
  );

  const title = notFound ? notFoundTitle(props.team) : editTitle(loaded?.team ?? props.team);
  return (
    <SideSheet
      title={title}
      titleId={TITLE_ID}
      focusTitle={!adding}
      onClose={removeAsks ? removal.keep : templateAsks ? template.cancel : guard.ask}
      canClose={!running}
      footer={footer}
    >
      {/* On a phone every input and select in the panel is 44px tall, for thumbs. */}
      <div className="flex flex-col gap-5 max-sm:[&_[data-field]]:h-11">
        <p aria-live="polite" className="sr-only">
          {list.notice}
        </p>
        {loadError && <LoadFailure title={teamLoadFailed} error={loadError} onRetry={onRetry} />}
        {!loadError && (!ready || !loaded) && (
          <p aria-live="polite" className="text-[13px] text-ink-muted">
            {loadingTeam(props.team)}
          </p>
        )}
        {notFound && <NotFound onBack={guard.leave} />}
        {ready && loaded && !notFound && (
          <>
            <LinkTeamOffer template={template} off={saving || removeHolds || templateAsks || update.busy} />
            {refused?.kind === "changed" && (
              <ChangedElsewhere
                refusal={refused}
                changed={changed}
                onReload={() => {
                  setRefused(null);
                  reload(loaded.team, true);
                }}
                onKeep={() => {
                  // The version stays the one loaded, so saving is refused
                  // again until you reload: saving over the team would
                  // delete any agent added elsewhere.
                  setRefused(null);
                  list.focus(TITLE_ID);
                }}
              />
            )}
            {refused?.kind === "cut short" && (
              <CutShort
                refusal={refused}
                busy={saving || reloading || holds}
                onSaveAgain={() => sending(() => saveAgain(loaded), loaded, loaded)}
              />
            )}
            {((tried && problems.length > 0) || refused?.kind === "failed") && (
              <ErrorSummary
                title={refused?.kind === "failed" ? refused.title : summaryTitle(problems.length, true)}
                problems={problems}
                failure={refused?.kind === "failed" ? refused.detail : ""}
                onFollow={follow}
              />
            )}
            {offer && (
              <div id="team-offer" tabIndex={-1} className="outline-none">
                <MissingProfilesOffer
                  team={loaded.team}
                  profiles={offer}
                  then="save"
                  adding={saving}
                  disabled={holds}
                  onAdd={() => save(ready.policy.hash)}
                />
              </div>
            )}

            <Field
              label="Team name"
              htmlFor="team-name"
              hint={
                team.trim() !== loaded.team
                  ? `Renames ${loaded.team}: its agents and their tasks move with it.`
                  : "Shown on the Agents page and on New task."
              }
              error={teamProblem}
            >
              <Input
                id="team-name"
                value={team}
                readOnly={saving}
                onChange={(e) => setTeam(e.target.value)}
                {...(teamProblem && { "aria-invalid": true, "aria-describedby": errorId("team-name"), className: "border-danger" })}
              />
            </Field>

            <TeamDescription list={list} team={team} saving={saving} builtIn={builtIn} />

            <TeamAgents
              list={list}
              loaded={loaded}
              problems={problems}
              missing={missing}
              engines={ready.engines}
              policy={ready.policy}
              saving={saving}
              builtIn={builtIn?.team}
              onAdd={() => list.add(newAgent(draft, ready.engines), hasProblems)}
            />

            {/* On a phone the footer is Cancel and Save changes alone: the team's own actions come last here. */}
            <section aria-labelledby="team-this" className="flex flex-col items-stretch gap-2 border-t border-divider pt-4 sm:hidden">
              <h3 id="team-this" className="text-[12px] font-medium text-ink-secondary">
                This team
              </h3>
              {savesAsTemplate &&
                (templateAsks ? (
                  templateForm("body")
                ) : (
                  <SaveTemplateButton
                    place="body"
                    template={template}
                    changed={changed}
                    off={saving || removeHolds || update.busy}
                    className="flex-col items-stretch"
                  />
                ))}
              {updatesTemplate && !templateAsks && (
                <>
                  <UpdateTemplateButton
                    place="body"
                    update={update}
                    changed={changed}
                    off={saving || removeHolds || templateHolds}
                    className="flex-col items-stretch"
                  />
                  <UpdateTemplateRefusal place="body" update={update} />
                </>
              )}
              {removeAsks ? (
                removeQuestion("body")
              ) : (
                <RemoveTeamButton
                  place="body"
                  removal={removal}
                  only={onlyTeam}
                  off={saving || templateHolds || update.busy}
                  className="flex-col items-stretch"
                />
              )}
            </section>
          </>
        )}
      </div>
    </SideSheet>
  );
}

// NotFound is the body of a panel whose link names no team you have: never
// an empty Edit panel. The title says it; this says why and goes back.
function NotFound({ onBack }: { onBack: () => void }) {
  return (
    <div className="flex flex-col items-start gap-3">
      <p className="text-[13px] text-ink-secondary">{notFoundDetail}</p>
      <Button size="sm" onClick={onBack}>
        Back to the Agents page
      </Button>
    </div>
  );
}

interface ChangedElsewhereProps {
  refusal: Extract<EditRefusal, { kind: "changed" }>;
  changed: boolean; // reloading would lose your changes, so it asks first
  onReload: () => void;
  onKeep: () => void;
}

// ChangedElsewhere is a save refused because the team changed since the
// panel loaded it: reload it as it is now, asking first in place, or keep
// your changes here to copy them first. It takes focus, as the summary does.
function ChangedElsewhere({ refusal, changed, onReload, onKeep }: ChangedElsewhereProps) {
  const [asking, setAsking] = useState(false);
  const keep = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (asking) keep.current?.focus();
  }, [asking]);
  return (
    <div
      id="team-summary"
      tabIndex={-1}
      aria-labelledby="team-summary-title"
      className="flex flex-col gap-2 rounded-md border border-danger/40 bg-danger-soft px-3 py-2.5 outline-none focus-visible:border-focus"
    >
      <h3 id="team-summary-title" className="text-[13px] font-medium text-danger-ink">
        {refusal.title}
      </h3>
      <p className="text-[13px] text-ink-secondary">{refusal.detail}</p>
      {asking ? (
        <div role="group" aria-labelledby="team-reload" className="flex flex-wrap items-center gap-2">
          <p id="team-reload" className="mr-auto text-[13px] font-medium text-ink-primary">
            {reloadQuestion}
          </p>
          <Button ref={keep} size="sm" onClick={() => setAsking(false)}>
            Keep editing
          </Button>
          <Button size="sm" variant="danger" onClick={onReload}>
            Reload
          </Button>
        </div>
      ) : (
        <span className="flex flex-wrap gap-2">
          <Button size="sm" onClick={() => (changed ? setAsking(true) : onReload())}>
            Reload the team
          </Button>
          <Button size="sm" variant="ghost" onClick={onKeep}>
            Keep my changes here
          </Button>
        </span>
      )}
    </div>
  );
}

interface CutShortProps {
  refusal: Extract<EditRefusal, { kind: "cut short" }>;
  busy: boolean; // the team is reloading or saving again, or Remove team asks or runs
  onSaveAgain: () => void;
}

// CutShort is a save that saved the team but left some of its tasks to
// move: the panel has the team as saved now, and saving it again moves them.
function CutShort({ refusal, busy, onSaveAgain }: CutShortProps) {
  return (
    <div
      id="team-summary"
      tabIndex={-1}
      aria-labelledby="team-summary-title"
      className="flex flex-col items-start gap-2 rounded-md border border-warning/40 bg-warning-soft px-3 py-2.5 outline-none focus-visible:border-focus"
    >
      <h3 id="team-summary-title" className="text-[13px] font-medium text-warning-ink">
        {refusal.title}
      </h3>
      <p className="text-[13px] text-ink-secondary">{refusal.detail}</p>
      <Button size="sm" disabled={busy} onClick={onSaveAgain}>
        Save again
      </Button>
    </div>
  );
}

import { KeyboardEvent, ReactNode, useEffect, useRef, useState } from "react";
import type { TeamTemplate } from "@/api/types";
import { Badge } from "@/components/atoms/Badge";
import { Button } from "@/components/atoms/Button";
import { MissingProfilesOffer } from "@/components/molecules/MissingProfilesOffer";
import { useToast } from "@/components/organisms/Toasts";
import { addTeam, missingProfilesOf } from "@/lib/teams";
import {
  deleteTemplate,
  importTemplate,
  savedLine,
  type TemplateFailure,
  templateExportURL,
  templateGroups,
  whyTemplateNotDeleted,
  whyTemplateNotImported,
} from "@/lib/templates";
import { cn } from "@/lib/cn";

// The Add a team dialog's list (docs/design/custom-teams.md, "3. The Add a
// team list", decisions 5 and 7): Build your own, then Your templates,
// newest first, when you have one, then Built in. Every template card adds
// its team as it is or opens the New team panel copying it. Adding a team
// whose profiles policy.toml lacks is refused with the names of those
// profiles; it can then be added with them, in one change of the policy
// made against the version the page saw (policyHash). The offer shows in
// the refused card, next to what you clicked. A template of yours can also
// be exported as a file, or deleted after a question in its card; Build
// your own imports one. While any request runs, nothing else can start.

interface AddTeamProps {
  templates: TeamTemplate[];
  existing: string[];
  policyHash: string;
  onBuild: (template?: string) => void; // opens the New team panel, blank or copying the template
  onAdded: (team: string) => void;
  onTemplatesChanged: () => Promise<unknown>; // reads the templates again, once one is deleted or imported
}

// What runs, if anything: adding, or deleting, the template with id, or an import.
type Busy = { doing: "add" | "delete"; id: string } | { doing: "import" };

interface NeedsProfiles {
  id: string; // the template refused
  team: string;
  profiles: string[];
}

const buildId = "team-build";
const addId = (template: TeamTemplate) => `template-add-${template.id}`;

export function AddTeam({ templates, existing, policyHash, onBuild, onAdded, onTemplatesChanged }: AddTeamProps) {
  const notify = useToast();
  const [busy, setBusy] = useState<Busy | null>(null);
  const [needs, setNeeds] = useState<NeedsProfiles | null>(null);
  const deletion = useDeleteTemplate(onTemplatesChanged, setBusy);
  const { yours, builtIn } = templateGroups(templates);
  const offer = useRef<HTMLDivElement>(null);
  // The card may be the last one, its offer below the dialog's fold.
  useEffect(() => {
    if (needs) offer.current?.scrollIntoView({ block: "nearest" });
  }, [needs]);

  // A built-in template is named by its team, one of yours by its id, as POST /api/teams takes them.
  async function add(template: TeamTemplate, addMissingProfiles = false) {
    setBusy({ doing: "add", id: template.id });
    try {
      await addTeam(template.userMade ? template.id : template.team, addMissingProfiles ? policyHash : undefined);
      notify("success", `Added the ${template.team} team`, "Pick it on New task; its coordinator takes the task.");
      onAdded(template.team);
    } catch (failure) {
      const profiles = missingProfilesOf(failure);
      if (profiles && !addMissingProfiles) setNeeds({ id: template.id, team: template.team, profiles });
      else notify("danger", "The team wasn't added", (failure as Error).message);
      setBusy(null);
    }
  }

  const card = (template: TeamTemplate, at: number) => (
    <TemplateCard
      key={template.id}
      template={template}
      has={existing.includes(template.team)}
      busy={busy}
      asking={deletion.asking === template.id}
      onAdd={() => add(template)}
      onChange={() => onBuild(template.id)}
      onAskDelete={() => deletion.ask(template.id)}
    >
      {deletion.asking === template.id && (
        <DeleteQuestion
          template={template}
          deleting={busy?.doing === "delete"}
          failure={deletion.failure}
          onKeep={deletion.keep}
          onDelete={() => deletion.remove(template, nextFocus(yours, at))}
        />
      )}
      {needs?.id === template.id && (
        <div ref={offer}>
          <MissingProfilesOffer
            team={needs.team}
            profiles={needs.profiles}
            adding={busy?.doing === "add" && busy.id === needs.id}
            disabled={busy !== null}
            onAdd={() => add(template, true)}
          />
        </div>
      )}
    </TemplateCard>
  );

  return (
    <div className="flex flex-col gap-3">
      <p className="text-[12px] text-ink-muted">
        Each team comes with a coordinator and its specialists, each with a prompt written for the work. Add one as it is, change it first,
        or build your own.
      </p>
      <h3 className="text-[12px] font-medium text-ink-secondary">Build your own</h3>
      <BuildYourOwn busy={busy} setBusy={setBusy} onBuild={() => onBuild()} onImported={onTemplatesChanged} />
      {yours.length > 0 && (
        <>
          <h3 className="text-[12px] font-medium text-ink-secondary">Your templates</h3>
          <div className="grid gap-3 sm:grid-cols-2">{yours.map(card)}</div>
        </>
      )}
      <h3 className="text-[12px] font-medium text-ink-secondary">Built in</h3>
      <div className="grid gap-3 sm:grid-cols-2">{builtIn.map(card)}</div>
    </div>
  );
}

// nextFocus is where focus goes once yours[at] is deleted: the next of
// your templates' Add this team, the one before it when it was the last,
// or Build a team when none is left.
function nextFocus(yours: TeamTemplate[], at: number): string {
  const next = yours[at + 1] ?? yours[at - 1];
  return next ? addId(next) : buildId;
}

// useDeleteTemplate is the delete question's state: the template it asks
// about, if any, and the last failure. Keep puts focus back on the card's
// Delete template; a delete reads the templates again and then moves focus.
// Either button shows only once the question has gone, so focus moves
// after that render.
function useDeleteTemplate(onDeleted: () => Promise<unknown>, setBusy: (busy: Busy | null) => void) {
  const notify = useToast();
  const [asking, setAsking] = useState<string | null>(null);
  const [failure, setFailure] = useState<TemplateFailure | null>(null);
  const [focusing, setFocusing] = useState<string | null>(null);
  useEffect(() => {
    if (focusing) document.getElementById(focusing)?.focus();
  }, [focusing]);

  const ask = (id: string) => {
    setAsking(id);
    setFailure(null);
    setFocusing(null);
  };

  const keep = () => {
    if (asking) setFocusing(deleteId(asking));
    setAsking(null);
    setFailure(null);
  };

  async function remove(template: TeamTemplate, focusAfter: string) {
    setBusy({ doing: "delete", id: template.id });
    setFailure(null);
    try {
      await deleteTemplate(template.id);
    } catch (failure) {
      setFailure(whyTemplateNotDeleted(failure));
      setBusy(null);
      return;
    }
    notify("success", `Deleted the ${template.team} template`);
    await onDeleted(); // resolves once the list without it shows
    setAsking(null);
    setBusy(null);
    setFocusing(focusAfter);
  }

  return { asking, failure, ask, keep, remove };
}

const deleteId = (id: string) => `template-delete-${id}`;

interface TemplateCardProps {
  template: TeamTemplate;
  has: boolean; // you have a team of its name
  busy: Busy | null;
  asking: boolean; // the delete question shows, in the actions' place
  onAdd: () => void;
  onChange: () => void; // Change it first
  onAskDelete: () => void;
  children?: ReactNode; // the delete question and the missing-profile offer, when they show
}

// TemplateCard is a template: its name, what it's for, its agents and, for
// one of yours, when it was saved, its export link and Delete template.
function TemplateCard({ template, has, busy, asking, onAdd, onChange, onAskDelete, children }: TemplateCardProps) {
  const name = template.team;
  const off = busy !== null;
  const saved = template.userMade ? savedLine(template) : undefined;
  return (
    <div role="group" aria-label={name} className="flex flex-col gap-2 rounded-lg border border-divider bg-surface-0 p-4">
      <span className="flex items-center gap-2">
        <span className="text-[14px] font-semibold text-ink-primary">{name}</span>
        {has && <Badge>you have it</Badge>}
        {template.userMade && !asking && (
          <button
            type="button"
            id={deleteId(template.id)}
            aria-label={`Delete template, ${name}`}
            disabled={off}
            onClick={onAskDelete}
            className={cn("ml-auto", QUIET_ACTION, "hover:text-danger-ink")}
          >
            Delete template
          </button>
        )}
      </span>
      <p className="text-[12px] leading-5 text-ink-secondary">{template.description}</p>
      <span className="flex flex-wrap gap-1.5">
        {template.agents.map((a) => (
          <span
            key={a.name}
            title={a.description}
            className={cn(
              "rounded-sm px-1.5 font-mono text-[11px] leading-5",
              a.coordinates ? "bg-brand-soft text-brand-ink" : "bg-surface-3 text-ink-secondary",
            )}
          >
            {a.name}
          </span>
        ))}
      </span>
      {saved && <p className="text-[12px] text-ink-muted">{saved}</p>}
      {!asking && (
        <span className="mt-auto flex flex-wrap items-center gap-2 pt-1">
          <Button
            id={addId(template)}
            size="sm"
            variant={has ? "ghost" : "primary"}
            aria-label={`${has ? "Add another" : "Add this team"}, ${name}`}
            disabled={off}
            onClick={onAdd}
          >
            {busy?.doing === "add" && busy.id === template.id ? "Adding…" : has ? "Add another" : "Add this team"}
          </Button>
          <Button size="sm" variant="ghost" aria-label={`Change it first, ${name}`} disabled={off} onClick={onChange}>
            Change it first
          </Button>
          {template.userMade && (
            // A plain link: the sign-in cookie lets the download in.
            <a
              href={templateExportURL(template.id)}
              download
              aria-label={`Export, ${name}`}
              aria-disabled={off || undefined}
              onClick={(event) => off && event.preventDefault()}
              className={cn("ml-auto", QUIET_ACTION)}
            >
              Export
            </a>
          )}
        </span>
      )}
      {children}
    </div>
  );
}

// 12px muted text with a 28px tall hit area, as the team headings' actions.
const QUIET_ACTION =
  "flex min-h-7 cursor-pointer items-center text-[12px] text-ink-muted hover:text-ink-primary disabled:cursor-default disabled:opacity-50";

interface DeleteQuestionProps {
  template: TeamTemplate;
  deleting: boolean;
  failure: TemplateFailure | null;
  onKeep: () => void;
  onDelete: () => void;
}

// DeleteQuestion asks in the card, in a group named by the question, focus
// on Keep; Esc is Keep, not the dialog's close. A failure says so in the
// card, with the server's words, and takes focus.
function DeleteQuestion({ template, deleting, failure, onKeep, onDelete }: DeleteQuestionProps) {
  const keep = useRef<HTMLButtonElement>(null);
  const failed = useRef<HTMLDivElement>(null);
  useEffect(() => keep.current?.focus(), []);
  useEffect(() => {
    if (failure) failed.current?.focus();
  }, [failure]);
  const question = `template-delete-question-${template.id}`;
  const escape = (event: KeyboardEvent) => {
    if (event.key !== "Escape") return;
    event.stopPropagation();
    if (!deleting) onKeep();
  };
  return (
    <div role="group" aria-labelledby={question} onKeyDown={escape} className="mt-auto flex flex-col gap-2 pt-1">
      <div>
        <p id={question} className="text-[13px] font-medium text-ink-primary">
          Delete the {template.team} template?
        </p>
        <p className="text-[12px] text-ink-secondary">Teams you made from it stay as they are.</p>
      </div>
      {failure && (
        <div ref={failed} tabIndex={-1} className="text-[12px] text-danger-ink outline-none focus-visible:outline-focus">
          <p className="font-medium">{failure.title}</p>
          <p>{failure.detail}</p>
        </div>
      )}
      <span className="flex gap-2">
        <Button ref={keep} size="sm" disabled={deleting} onClick={onKeep}>
          Keep
        </Button>
        <Button size="sm" variant="danger" disabled={deleting} onClick={onDelete}>
          {deleting ? "Deleting…" : "Delete"}
        </Button>
      </span>
    </div>
  );
}

interface BuildYourOwnProps {
  busy: Busy | null;
  setBusy: (busy: Busy | null) => void;
  onBuild: () => void;
  onImported: () => Promise<unknown>;
}

// BuildYourOwn opens the New team panel blank, or imports a template from
// a file exported from an Agentry. A refused import says why under the
// button, in the server's words: a name you have already is said as a save says it.
function BuildYourOwn({ busy, setBusy, onBuild, onImported }: BuildYourOwnProps) {
  const notify = useToast();
  const file = useRef<HTMLInputElement>(null);
  const [failure, setFailure] = useState<TemplateFailure | null>(null);

  async function importFile(chosen: File | undefined) {
    if (!chosen) return;
    setBusy({ doing: "import" });
    setFailure(null);
    try {
      const imported = await importTemplate(chosen);
      notify("success", `Imported ${imported.team} to your templates`);
      await onImported();
    } catch (failure) {
      setFailure(whyTemplateNotImported(failure));
    } finally {
      setBusy(null);
      if (file.current) file.current.value = ""; // so the same file can be picked again
    }
  }

  const off = busy !== null;
  return (
    <div
      role="group"
      aria-label="A team of your own"
      className="flex flex-col items-start gap-2 rounded-lg border border-divider bg-surface-0 p-4"
    >
      <span className="text-[14px] font-semibold text-ink-primary">A team of your own</span>
      <p className="text-[12px] leading-5 text-ink-secondary">Start blank or copy a template, then change any agent.</p>
      <span className="flex flex-wrap gap-2">
        <Button id={buildId} size="sm" disabled={off} onClick={onBuild}>
          Build a team
        </Button>
        <Button size="sm" variant="ghost" disabled={off} onClick={() => file.current?.click()}>
          {busy?.doing === "import" ? "Importing…" : "Import a template"}
        </Button>
      </span>
      <input
        ref={file}
        type="file"
        accept=".json,application/json"
        hidden
        data-testid="template-file"
        onChange={(event) => importFile(event.target.files?.[0])}
      />
      {failure && (
        <div role="alert" className="text-[12px] text-danger-ink">
          <p className="font-medium">{failure.title}</p>
          <p>{failure.detail}</p>
        </div>
      )}
    </div>
  );
}

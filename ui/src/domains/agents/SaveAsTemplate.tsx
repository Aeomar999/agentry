import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/atoms/Button";
import { Input } from "@/components/atoms/Input";
import { errorId, Field } from "@/components/molecules/Field";
import { useToast } from "@/shell/Toasts";
import { cn } from "@/lib/cn";
import {
  checkTemplateName,
  linkTeam,
  type SavedTemplate,
  saveAsTemplate,
  type TemplateFailure,
  type TemplateForm,
  type TemplateRefusal,
  updateTemplate,
  whyTeamNotLinked,
  whyTemplateNotSaved,
  whyTemplateNotUpdated,
} from "@/domains/agents/templates";
import type { Place } from "./RemoveTeam";

// Save as a template in the Edit team panel (docs/design/custom-teams.md,
// "2. Edit a team", decisions 5, 7 and 12): a button that turns into a
// small form in place, saving the team as last saved, never the draft. Like
// Remove team, it lives in the footer on desktop and at the end of the body
// under This team on a phone; both places are drawn and CSS shows one.

// useSaveTemplate is the form's state: where it was opened, if it is, what
// is typed, the request while it runs, and the last refusal. team is the
// team as last saved, and description its coordinator's, for the form's
// defaults; changed says the panel has unsaved changes, so save refuses as
// the form says. Cancel and success put focus back on the button it came
// from. A template saved that the team couldn't be linked to is unlinked,
// with Link this team's request and refusal, until it is linked, here or
// as linked says (the template id the roster has the team linked to now),
// or another is saved.
export function useSaveTemplate(
  team: string,
  description: string,
  changed: boolean,
  linked: string | undefined,
  focus: (id: string) => void,
) {
  const notify = useToast();
  const [asking, setAsking] = useState<Place | null>(null);
  const [form, setForm] = useState<TemplateForm>({ name: "", description: "" });
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<TemplateRefusal | null>(null);
  const [unlinked, setUnlinked] = useState<{ saved: SavedTemplate; place: Place } | null>(null);
  const [linking, setLinking] = useState(false);
  const [linkRefused, setLinkRefused] = useState<TemplateFailure | null>(null);

  function ask(place: Place) {
    setForm({ name: team, description });
    setRefused(null);
    setUnlinked(null);
    setAsking(place);
  }

  function cancel() {
    if (asking) focus(idOf(asking, "button"));
    setAsking(null);
    setRefused(null);
  }

  async function save() {
    if (changed) return;
    const empty = checkTemplateName(form.name);
    if (empty) return setRefused({ kind: "name", message: empty });
    setRefused(null);
    setBusy(true);
    let saved: SavedTemplate;
    try {
      saved = await saveAsTemplate(team, form);
    } catch (failure) {
      setRefused(whyTemplateNotSaved(failure));
      return;
    } finally {
      setBusy(false);
    }
    if (saved.notLinked && asking) {
      setUnlinked({ saved, place: asking });
      setLinkRefused(null);
      setAsking(null);
      focus(LINK_ID);
      return;
    }
    notify("success", `Saved ${saved.team} to your templates`, "Add it again from Add a team.");
    cancel();
  }

  async function link() {
    if (!unlinked) return;
    setLinkRefused(null);
    setLinking(true);
    try {
      await linkTeam(team, unlinked.saved.id);
    } catch (failure) {
      setLinkRefused(whyTeamNotLinked(failure));
      return;
    } finally {
      setLinking(false);
    }
    notify("success", `Linked the ${team} team to ${unlinked.saved.team}`, "It keeps the template's features.");
    setUnlinked(null);
    focus(idOf(unlinked.place, "button"));
  }

  // A link made elsewhere, from another tab say, shows in the roster as it reloads.
  const stillUnlinked = unlinked?.saved.id === linked ? null : unlinked;

  return { asking, form, busy, refused, unlinked: stillUnlinked, linking, linkRefused, ask, change: setForm, cancel, save, link };
}

export type TeamTemplateSave = ReturnType<typeof useSaveTemplate>;

const idOf = (place: Place, part: string) => `team-template-${place}-${part}`;
const LINK_ID = "team-link";

interface SaveTemplateButtonProps {
  place: Place;
  template: TeamTemplateSave;
  changed: boolean; // it saves the team as last saved, so your changes go first
  off: boolean; // while the team saves, or Remove team or Update my template asks or runs
  className?: string;
}

// SaveTemplateButton opens the form, or says beside it why it can't.
export function SaveTemplateButton({ place, template, changed, off, className }: SaveTemplateButtonProps) {
  return (
    <span className={cn("flex items-center gap-x-3 gap-y-1", className)}>
      <Button
        id={idOf(place, "button")}
        disabled={changed || off || template.linking}
        aria-describedby={changed ? idOf(place, "why") : undefined}
        onClick={() => template.ask(place)}
        className="max-sm:h-11 max-sm:w-full"
      >
        Save as a template
      </Button>
      {changed && (
        <span id={idOf(place, "why")} className="text-[12px] text-ink-muted">
          Save your changes first.
        </span>
      )}
    </span>
  );
}

interface SaveTemplateFormProps {
  place: Place;
  template: TeamTemplateSave;
  changed: boolean; // edits made with the form open: Cancel, then Save changes
}

// SaveTemplateForm is the form in place, focus on Template name where it
// was opened. A name to change is said under Template name, any other
// refusal above the buttons; either takes focus as it shows, and what was
// typed stays. With unsaved changes Save template is off, saying why as the
// button that opens the form does.
export function SaveTemplateForm({ place, template, changed }: SaveTemplateFormProps) {
  const name = useRef<HTMLInputElement>(null);
  const failure = useRef<HTMLDivElement>(null);
  const here = template.asking === place;
  const { form, busy, refused } = template;
  useEffect(() => {
    if (here) name.current?.focus();
  }, [here]);
  useEffect(() => {
    if (!here || !refused) return;
    (refused.kind === "name" ? name : failure).current?.focus();
  }, [here, refused]);
  const nameError = refused?.kind === "name" ? refused.message : undefined;
  const hint = idOf(place, "hint");
  return (
    <form
      aria-label="Save as a template"
      onSubmit={(event) => {
        event.preventDefault();
        template.save();
      }}
      className="flex flex-col gap-2"
    >
      <div className="grid grid-cols-2 gap-3 max-sm:grid-cols-1">
        <Field label="Template name" htmlFor={idOf(place, "name")} error={nameError}>
          <Input
            ref={name}
            id={idOf(place, "name")}
            value={form.name}
            readOnly={busy}
            onChange={(e) => template.change({ ...form, name: e.target.value })}
            aria-describedby={nameError ? `${errorId(idOf(place, "name"))} ${hint}` : hint}
            {...(nameError && { "aria-invalid": true, className: "border-danger" })}
          />
        </Field>
        <Field label="What it's for" htmlFor={idOf(place, "description")}>
          <Input
            id={idOf(place, "description")}
            value={form.description}
            readOnly={busy}
            onChange={(e) => template.change({ ...form, description: e.target.value })}
            aria-describedby={hint}
          />
        </Field>
      </div>
      <p id={hint} className="text-[11px] text-ink-muted">
        Keeps each agent&apos;s prompt and settings and which one coordinates; not knowledge or where they run.
      </p>
      {refused?.kind === "failed" && (
        <div ref={failure} tabIndex={-1} className="text-[12px] text-danger-ink outline-none focus-visible:outline-focus">
          <p className="font-medium">{refused.title}</p>
          <p>{refused.detail}</p>
        </div>
      )}
      <div className="flex items-center justify-end gap-3 max-sm:flex-wrap">
        {changed && (
          <span id={idOf(place, "save-why")} className="text-[12px] text-ink-muted max-sm:w-full">
            Save your changes first.
          </span>
        )}
        <Button variant="ghost" disabled={busy} onClick={template.cancel} className="max-sm:h-11 max-sm:flex-1">
          Cancel
        </Button>
        <Button
          type="submit"
          variant="primary"
          disabled={busy || changed}
          aria-describedby={changed ? idOf(place, "save-why") : undefined}
          className="max-sm:h-11 max-sm:flex-1"
        >
          {busy ? "Saving…" : "Save template"}
        </Button>
      </div>
    </form>
  );
}

interface LinkTeamOfferProps {
  template: TeamTemplateSave;
  off: boolean; // while the team saves, or anything else asks or runs
}

// LinkTeamOffer is a template saved from the team that the team couldn't
// be linked to, at the top of the panel's body, where it stays until the
// team is linked: why, and Link this team. It takes focus as it shows, and
// a refusal is said in it in the server's words.
export function LinkTeamOffer({ template, off }: LinkTeamOfferProps) {
  const { unlinked, linking, linkRefused } = template;
  if (!unlinked) return null;
  return (
    <div
      id={LINK_ID}
      tabIndex={-1}
      aria-labelledby={`${LINK_ID}-title`}
      className="flex flex-col items-start gap-2 border border-warning/40 bg-warning-soft px-3 py-2.5 outline-none focus-visible:border-focus"
    >
      <h3 id={`${LINK_ID}-title`} className="text-[13px] font-medium text-warning-ink">
        Saved {unlinked.saved.team} to your templates, but this team isn&apos;t linked to it yet
      </h3>
      <p className="text-[13px] text-ink-secondary">{unlinked.saved.notLinked}</p>
      {linkRefused && (
        <div role="alert" className="text-[13px] text-danger-ink">
          <p className="font-medium">{linkRefused.title}</p>
          <p>{linkRefused.detail}</p>
        </div>
      )}
      <Button size="sm" disabled={linking || off} onClick={template.link}>
        {linking ? "Linking…" : "Link this team"}
      </Button>
    </div>
  );
}

// useUpdateTemplate is Update my template's state: the request while it
// runs, and the last refusal with where it was pressed. Like Save as a
// template it saves the team as last saved, so changed holds it. Success
// puts focus back on the button pressed; forget drops an old refusal.
export function useUpdateTemplate(team: string, changed: boolean, focus: (id: string) => void) {
  const notify = useToast();
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<{ place: Place; failure: TemplateFailure } | null>(null);

  async function update(place: Place) {
    if (changed) return;
    setRefused(null);
    setBusy(true);
    try {
      const updated = await updateTemplate(team);
      notify("success", `Updated ${updated.team} in your templates`, `It has the ${team} team as last saved.`);
      focus(updateIdOf(place, "button"));
    } catch (failure) {
      setRefused({ place, failure: whyTemplateNotUpdated(failure) });
    } finally {
      setBusy(false);
    }
  }

  return { busy, refused, update, forget: () => setRefused(null) };
}

export type TeamTemplateUpdate = ReturnType<typeof useUpdateTemplate>;

const updateIdOf = (place: Place, part: string) => `team-update-${place}-${part}`;

interface UpdateTemplateButtonProps {
  place: Place;
  update: TeamTemplateUpdate;
  changed: boolean; // it saves the team as last saved, so your changes go first
  off: boolean; // while the team saves, or anything else asks or runs
  className?: string;
}

// UpdateTemplateButton saves the team over the template of yours it is
// linked to, or says beside it why it can't.
export function UpdateTemplateButton({ place, update, changed, off, className }: UpdateTemplateButtonProps) {
  return (
    <span className={cn("flex items-center gap-x-3 gap-y-1", className)}>
      <Button
        id={updateIdOf(place, "button")}
        disabled={changed || off || update.busy}
        aria-describedby={changed ? updateIdOf(place, "why") : undefined}
        onClick={() => update.update(place)}
        className="max-sm:h-11 max-sm:w-full"
      >
        {update.busy ? "Updating…" : "Update my template"}
      </Button>
      {changed && (
        <span id={updateIdOf(place, "why")} className="text-[12px] text-ink-muted">
          Save your changes first.
        </span>
      )}
    </span>
  );
}

// UpdateTemplateRefusal is a refused update where it was pressed, as Save
// as a template's form says its refusals, taking focus as it shows.
export function UpdateTemplateRefusal({ place, update }: { place: Place; update: TeamTemplateUpdate }) {
  const failure = useRef<HTMLDivElement>(null);
  const refused = update.refused?.place === place ? update.refused.failure : null;
  useEffect(() => {
    if (refused) failure.current?.focus();
  }, [refused]);
  if (!refused) return null;
  return (
    <div
      ref={failure}
      id={updateIdOf(place, "failure")}
      tabIndex={-1}
      className="text-[12px] text-danger-ink outline-none focus-visible:outline-focus"
    >
      <p className="font-medium">{refused.title}</p>
      <p>{refused.detail}</p>
    </div>
  );
}

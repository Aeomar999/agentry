import { useEffect, useRef, useState } from "react";
import type { TeamTemplate } from "@/api/types";
import { Button } from "@/components/atoms/Button";
import { Input } from "@/components/atoms/Input";
import { errorId, Field } from "@/components/molecules/Field";
import { useToast } from "@/components/organisms/Toasts";
import { cn } from "@/lib/cn";
import { checkTemplateName, saveAsTemplate, type TemplateForm, type TemplateRefusal, whyTemplateNotSaved } from "@/lib/templates";
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
// from.
export function useSaveTemplate(team: string, description: string, changed: boolean, focus: (id: string) => void) {
  const notify = useToast();
  const [asking, setAsking] = useState<Place | null>(null);
  const [form, setForm] = useState<TemplateForm>({ name: "", description: "" });
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<TemplateRefusal | null>(null);

  function ask(place: Place) {
    setForm({ name: team, description });
    setRefused(null);
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
    let saved: TeamTemplate;
    try {
      saved = await saveAsTemplate(team, form);
    } catch (failure) {
      setRefused(whyTemplateNotSaved(failure));
      return;
    } finally {
      setBusy(false);
    }
    notify("success", `Saved ${saved.team} to your templates`, "Add it again from Add a team.");
    cancel();
  }

  return { asking, form, busy, refused, ask, change: setForm, cancel, save };
}

export type TeamTemplateSave = ReturnType<typeof useSaveTemplate>;

const idOf = (place: Place, part: string) => `team-template-${place}-${part}`;

interface SaveTemplateButtonProps {
  place: Place;
  template: TeamTemplateSave;
  changed: boolean; // it saves the team as last saved, so your changes go first
  off: boolean; // while the team saves, or Remove team asks or runs
  className?: string;
}

// SaveTemplateButton opens the form, or says beside it why it can't.
export function SaveTemplateButton({ place, template, changed, off, className }: SaveTemplateButtonProps) {
  return (
    <span className={cn("flex items-center gap-x-3 gap-y-1", className)}>
      <Button
        id={idOf(place, "button")}
        disabled={changed || off}
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

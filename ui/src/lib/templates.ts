import { api, ApiError, send } from "@/api/client";
import type { Agent, TeamTemplate } from "@/api/types";
import { sameName, whyNotCreated } from "./custom-team";

// Your templates (docs/design/custom-teams.md, decision 5): a team saved as
// it was last saved, to add again from Add a team. Saving, deleting,
// exporting and importing one, and the list in its groups.

// A refused template change, as the form or card shows it: a name to
// change, under Template name, or anything else in the server's words.
export type TemplateRefusal = { kind: "name"; message: string } | TemplateFailure;
export type TemplateFailure = { kind: "failed"; title: string; detail: string };

export interface TemplateForm {
  name: string; // the template's, which a team made from it is called
  description: string; // What it's for
}

// checkTemplateName is why the name can't be saved, before asking the
// server, or undefined when it can.
export const checkTemplateName = (name: string) => (name.trim() === "" ? "Give the template a name." : undefined);

// isBuiltInTeam says whether team has a built-in template's team name, in
// any case: no template of yours may have it, so such a team isn't saved as one.
export const isBuiltInTeam = (templates: TeamTemplate[], team: string) =>
  templates.some((template) => !template.userMade && sameName(template.team, team));

// builtInTemplateOf is the built-in template, with a description, the
// agent named was made from: the server describes the agent's team by that
// description rather than the agent's own (coordinator/brief.go teamDescription).
export function builtInTemplateOf(roster: Agent[], templates: TeamTemplate[], agent: string): TeamTemplate | undefined {
  const made = roster.find((a) => a.name === agent)?.template;
  return templates.find((template) => !template.userMade && template.id === made && template.description !== "");
}

// saveAsTemplate saves the team, as it was last saved, as your template.
export function saveAsTemplate(team: string, form: TemplateForm): Promise<TeamTemplate> {
  return api("POST", `/api/teams/${encodeURIComponent(team)}/template`, { name: form.name.trim(), description: form.description.trim() });
}

// whyTemplateNotSaved is a refused save as the form shows it: the server
// refuses a name a template has, in any case, with a 409 in the copy deck's
// words, which go under the name.
export function whyTemplateNotSaved(failure: unknown): TemplateRefusal {
  if (failure instanceof ApiError && failure.status === 409) return { kind: "name", message: failure.message };
  return failed("Couldn't save the template", failure);
}

// deleteTemplate deletes your template id. The teams made from it stay.
export function deleteTemplate(id: string): Promise<unknown> {
  return api("DELETE", `/api/teams/templates/${encodeURIComponent(id)}`);
}

export const whyTemplateNotDeleted = (failure: unknown) => failed("Couldn't delete the template", failure);

// templateExportURL is where your template id downloads from as a file;
// the page's sign-in cookie lets it in, so it can be a plain link.
export const templateExportURL = (id: string) => `/api/teams/templates/${encodeURIComponent(id)}/export`;

// importTemplate saves a template exported as a file. The server reads it
// and refuses one that isn't a template; one with an id you have replaces it.
export async function importTemplate(file: File): Promise<TeamTemplate> {
  const response = await send("/api/teams/templates/import", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: file,
  });
  return response.json() as Promise<TeamTemplate>;
}

export const whyTemplateNotImported = (failure: unknown) => failed("Couldn't import the template", failure);

const failed = (title: string, failure: unknown): TemplateFailure => ({ kind: "failed", title, detail: whyNotCreated(failure) });

// templateGroups splits the templates into yours, newest first, and the
// built-in ones, as the server lists them, for Your templates and Built in.
export function templateGroups(templates: TeamTemplate[]): { yours: TeamTemplate[]; builtIn: TeamTemplate[] } {
  // The server lists yours in the order first saved, so of two saved in
  // the same second the later one is newer.
  const yours = templates
    .map((template, at) => ({ template, at, savedAt: Date.parse(template.savedAt ?? "") || 0 }))
    .filter(({ template }) => template.userMade)
    .sort((a, b) => b.savedAt - a.savedAt || b.at - a.at)
    .map(({ template }) => template);
  return { yours, builtIn: templates.filter((template) => !template.userMade) };
}

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

// savedLine is where and when your template was saved, as its card says
// it: `12 Oct`, with the year when it isn't this one. One saved by an older
// Agentry names no team; one with no date has no line.
export function savedLine(template: TeamTemplate, now = new Date()): string | undefined {
  const saved = new Date(template.savedAt ?? "");
  if (Number.isNaN(saved.getTime())) return undefined;
  const year = saved.getFullYear() === now.getFullYear() ? "" : ` ${saved.getFullYear()}`;
  const date = `${saved.getDate()} ${MONTHS[saved.getMonth()]}${year}`;
  return template.from ? `Saved ${date} from the ${template.from} team.` : `Saved ${date}.`;
}

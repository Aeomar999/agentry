# Build and edit a custom team

A design note for the user and for the engineer who builds it (task-178). The problem, measures and limits are in [the spec](custom-teams/spec.md). The final words on screen are in [the copy deck](custom-teams/copy.md), and the wireframes carry them. Strings quoted below are the deck's; if one here ever differs, the deck wins.

Wireframes:

- [New team, desktop](custom-teams/new-team-desktop.svg): Copy a template chosen, four agents, one open.
- [Edit a team, desktop](custom-teams/edit-team-desktop.svg): rename the team, a renamed agent open, a removed agent, a new one, Save as a template, Remove team, the team heading's actions, and the footer with nothing changed.
- [Add a team list](custom-teams/add-team-list.svg): your templates beside the built-in ones, and deleting one of yours; the coordinator chip's contrast fix.

Every state is drawn too; see [States](#states). The design was critiqued in [critique.md](custom-teams/critique.md).

## The direction, in a line

**One team panel on `/agents`.** A side panel opens over the right of the Agents page (`/agents?team=new` or `/agents?team=<name>`), holds the whole team as a list of agent rows that open in place, and saves the team in one checked request. The existing Add a team dialog stays the way in: it gains your templates and a way to build a team of your own, and hands over to the panel.

## What already exists, and what is reused

- `AgentsPage.tsx`: team headings with `TeamHeading` (`+ Add agent`, `Remove team`), the Add a team `Dialog` listing built-in templates with `Add this team` / `Add another`, the `MissingProfilesOffer` inside the refused template's card, `RemoveDialog`, `ColumnBadge`, `Badge`.
- `AgentForm.tsx`: the fields and their hints (Name, Description, Colour radiogroup, "Its work counts as", Coordinates, Engine, Model, Policy profile with the "(missing from policy.toml)" option), and its inline remove confirm. The panel's agent row is these fields, rearranged; the hints are the same text, shortened where the deck says.
- `agents.AddTeam`: a second copy of a template gets a numbered team (`Visionaries 2`) and numbered agent names (`engineer-2`). Copy a template and Blank do the same numbering on the client, so a new team is valid from the start.
- Agent names are global (`/api/agents/{name}`), so "duplicate name" means against every agent you have, not only the team.

## Flows

### 1. New team

Who: you, on `/agents`, wanting a team of your own. Entry points:

- **Add a team → `Build a team`** (the card "A team of your own", first in the dialog under "Build your own") opens the panel with Blank chosen.
- **Add a team → `Change it first`** on a template card opens the panel with Copy a template chosen and that template picked.

Both close the Add a team dialog and open the panel, so only one surface is ever open.

Steps:

1. **Name the team.** `Team name`, focused when the panel opens. Checked as you leave the field: empty, or a team name you have already (any case) is an error under the field. A copy suggests the template's name, numbered if taken (`Software 2`).
2. **Choose how to start.** `Start from` is a two-way segmented choice, the existing `SegmentedControl`:
   - **Blank**: one agent row, named `coordinator`, or `coordinator-2`, `coordinator-3` if taken (numbered as copies are, so a second Blank team starts valid), checked as the coordinator, its prompt empty (see *The default coordinator prompt*).
   - **Copy a template**: a select of templates, grouped "Your templates" then "Built in"; closed, it shows the template's name alone. Picking one fills the agent rows from it: every field, with names numbered where taken. Switching template, or switching to Blank, after you have changed rows asks first, in place under the control: `Replace your 3 changed agents with Research's?` · `Keep mine` · `Replace`. Nothing is replaced without that.
3. **Shape the agents.** The agents list shows one row per agent, collapsed: colour dot, name, its description on one line, column badge, the coordinator radio, and a remove button. Opening a row (the row's name button, `aria-expanded`) shows its fields in place; one row open at a time is the default, and opening another closes the first only if the first has no error.
   - **`+ Add agent`** (under the list) adds a row at the end, open, with a free colour, Building, the default engine and profile, and focus in its Name.
   - **Remove** on a row (a 28px trash button named `Remove {agent}`) takes it out at once; a one-line stub stays in its place, `{agent} removed` with an `Undo` button, for as long as the panel is open, and focus goes to Undo. Removing the coordinator leaves no radio checked, which is the "no coordinator" error on save. Removing the last row leaves `No agents yet. Add one: it coordinates the team.`; the next agent added is checked as the coordinator.
4. **Save.** `Create team`, never disabled while the panel is ready (a disabled button can't say why; it is off only while loading, see decision 11). The whole team goes in one request. While it runs the button reads `Creating…` and is off; Cancel, Close and Esc are off; the fields are read-only (decision 8). Success closes the panel, scrolls the page to the new team's heading and shows the toast `Added the {team} team` · `Pick it on New task; its coordinator takes the task.` (today's words). Failure keeps the panel and everything typed; see *Errors*.

Where it goes wrong: a name already taken (team or agent), no coordinator, a profile policy.toml lacks (the offer), an empty prompt on an agent that isn't the coordinator, the server refusing. Each is handled under *Errors*.

### 2. Edit a team

Entry: **`Edit team`** in every `TeamHeading`, first, beside `+ Add agent` and `Remove team`, all three in today's quiet look (12px ink-muted text buttons, 28px hit areas). It opens the panel for that team (`?team=<name>`). `+ Add agent` opens the same panel with a new row added and open, instead of going to `/agents/new` (the spec's "no trip to /agents/new"). The agent page `/agents/{name}` and its form stay as they are, for knowledge, where it runs and the full prompt editor.

The panel is the New team panel titled `Edit the {team} team`, with the team's agents loaded, collapsed, and without Start from. The open row ends with `Knowledge, where it runs and a taller prompt editor:` and the link `Open {agent}'s page`; in New team that line reads `Knowledge and where it runs: on its own page, once the team is created.` Changes are a draft until **`Save changes`**, which is off, with `No changes to save.` beside it, until something changed:

- **Rename the team**: edit Team name. Once it differs, the hint says what follows: `Renames {old}: its agents and their tasks move with it.`
- **Rename an agent**: edit its Name; the hint is AgentForm's, `Renames {old}: it keeps its prompt, settings and what it knows, and its tasks go on with the new name.` A renamed row shows `was {old}` and an `edited` badge while collapsed; a changed row shows `edited`, an added one `new`, a removed one the stub `{agent} will be removed when you save` with Undo, all until you save, so every change can be seen before saving. Agentry doesn't rewrite the prompt on a rename, and the open row says so under the prompt.
- **Change a prompt**: in the open row's Prompt box; long prompts scroll inside it. `Open {agent}'s page` goes to `/agents/{name}` for the full editor; with unsaved changes it asks first (`Open {agent}'s page and discard your changes?` · `Keep editing` · `Discard and open`). A new, unsaved agent has no page yet, so its row says `Knowledge and where it runs: on its own page, once you save.`
- **Add / remove agents**: as in New team. Removing an agent an unfinished task needs is refused by the server on save, naming the task, as `RemoveDialog` does today.
- **Change the coordinator**: pick another row's radio.
- **`Save as a template`** (footer, secondary): opens a small inline form in the footer: `Template name` (default: the team name), one line `What it's for` (default: the coordinator's description), `Save template` · `Cancel`. It saves the team **as last saved**; with unsaved changes the button is off and says why beside it: `Save your changes first.` A template keeps each agent's name, description, prompt, engine, model, profile, colour, column and coordinator; not knowledge and not where they run. Success: toast `Saved {template} to your templates` · `Add it again from Add a team.`
- **`Remove team`** (footer, danger, far side): the AgentForm inline confirm, in the footer: `Remove the {team} team and its {n} agents?` / `Their prompts and what they learned go too.` · `Keep` · `Remove`. Off for your only team, with `Can't remove your only team: tasks need its coordinator.` beside it. Success closes the panel and toasts `Removed the {team} team`.
- **Close with unsaved changes** asks first, in the footer: `Discard your changes to {team}?` / `The team renamed, 1 agent changed, 1 added and 1 removed. They aren't saved.` · `Keep editing` · `Discard`, focus on Keep editing ([unsaved changes](custom-teams/states-unsaved-and-replace.svg)). Browser Back asks the same (decision 11).

On a phone the footer keeps only Cancel and Save changes; Save as a template and Remove team, with their reasons and questions, sit at the end of the body under the heading `This team` (decision 12).

### 3. The Add a team list

The existing `Dialog` (`Add a team`, wide), in three groups, in this order:

1. **"Build your own"**: one card, `A team of your own`, with `Start blank or copy a template, then change any agent.` Button `Build a team` opens the panel with Blank chosen.
2. **"Your templates"**: only when you have one. Each card is a built-in card (name, description, agent chips, coordinator chip) plus a line `Saved {date} from the {team} team.` and `Delete template` as a quiet text button at the card's top right, named `Delete template, {template}`. Newest first. The coordinator chip's text moves from `text-brand` to `text-brand-ink` on every card (the Badge brand variant's ink): on brand-soft over surface-0 that is #994c27, 5.7:1, in light (today 2.6:1) and #f87f44, 5.9:1, in dark (today 5.4:1); the lowest case, light on surface-2, is 5.1:1. One class change, no new token.
3. **"Built in"**: today's cards, unchanged.

Every template card, built-in or yours, has the same two actions: **`Add this team`** (or `Add another` when you have one, as today; one action, the reuse measure) and **`Change it first`**, which opens the panel copying it. Each is named with its card, `Add this team, {template}` and `Change it first, {template}`, so a list of buttons tells them apart. The missing-profile offer stays inside the card that was refused, as today.

**Delete a template of yours:** `Delete template` turns the card's footer into a confirm, in place, focus on Keep: `Delete the {template} template?` / `Teams you made from it stay as they are.` · `Keep` · `Delete`. Delete removes the card; focus goes to the next card's `Add this team`, or to `Build a team` if none is left; toast `Deleted the {template} template`. A failure shows `Couldn't delete the template` and the server's words in the card. Built-in templates have no delete.

## Per agent: the fields in an open row

On desktop: Name and Description side by side, the Prompt across the full row, then the settings in two columns (Colour | Its work counts as; Engine + Model | Policy profile). Tab order is that reading order. One column on a phone. Every open row in the drawings uses this layout ([long prompt](custom-teams/states-long-prompt.svg) says why).

| Field | Control | Rule (checked on leaving the field, and all again on save) |
|---|---|---|
| Name | text, mono, lowercased as typed | required; lowercase letters, digits and dashes, 31 at most; unique among all your agents and the team's other rows |
| Description | text, one line | optional; `One line. The coordinator reads it to choose who works next.` |
| Prompt | the existing `CodeViewer` (Monaco, markdown, word wrap), `minHeight` 180 (8 lines), `maxHeight` 500 (24 lines), then scrolls inside the box; `tabFocusMode` on so Tab leaves it | required, except the coordinator's (empty = default prompt) |
| Colour | the AgentForm radiogroup of swatches, each named by its colour | a free one by default; two agents may share one (the name is always shown) |
| Its work counts as | select: `Building` / `Review` / `Shipping`, with a short hint per choice | the coordinator's is ignored: the select is disabled with `Not used: a coordinator hands the work out.` |
| Coordinator | a radio in the row header, `name="coordinator"`, inside one `<fieldset>` with the legend `Agents · one coordinates` | exactly one checked to save |
| Engine | select, from `/api/config` | |
| Model | text with the engine's suggestions | optional |
| Policy profile | select: `default ({profile})`, the policy's profiles, and `{profile} (missing from policy.toml)` when the agent names one it lacks | a missing profile is allowed only by accepting the offer |

The coordinator radio is a native radio group across the rows (same `name`), so arrow keys move between agents and only one can be checked. Each radio shows `Coordinator`; its accessible name is `Coordinator, {agent}`, which starts with the visible word (2.5.3). A row that coordinates shows the `coordinator` brand badge collapsed, as the team cards do.

### The default coordinator prompt

When the coordinator's Prompt is empty, the box shows, as placeholder text in ink-muted (the Input atom's placeholder is ink-faint, 2.6:1, so this box overrides it) and as the hint below it: `Empty: it uses Agentry's coordinator prompt for a team of these agents.` and a `See the default` link that opens the text read-only in the same box (its content is epic item 5, out of scope). Only the coordinator may be saved with an empty prompt.

### The missing-profile offer

On a copy, the client knows the template's profiles and the policy's, so the row says it before save: the profile select shows `{profile} (missing from policy.toml)` and the hint is AgentForm's, ending `Create team offers to add it, or pick another.` On save, the server refuses with 409 and `missingProfiles` as `POST /api/teams` does today; the panel then shows `MissingProfilesOffer` at the top of the panel body (where the error summary goes), its button `Add the profile and create the team` (New) or `Add the profile and save the team` (Edit), which resends with `addMissingProfiles` and the policy hash. Fixing every row's profile by hand also clears it. Before save, a warning line under the agents legend counts the agents and names the profile, and closed rows say `profile missing` (icon and words, warning-ink); it is a warning, not a counted problem. A profile no default has (a typo, or one since removed from policy.toml) is not offered: it is an "unknown profile" error, fixed only by picking another. Drawn in [missing-profile offer](custom-teams/states-missing-profile.svg).

## Errors

- Under each field, in danger-ink with an icon, tied by `aria-describedby`, for example: `Use only lowercase letters, digits and dashes.` / `You have an agent called {agent} already, in the {team} team. Pick another name.` / `You have a team called {team} already. Pick another name.` / `Pick one agent to coordinate.` The deck's Validation table has them all.
- A collapsed row with an error shows `1 problem` (`{n} problems`) in danger-ink beside its name, so errors in closed rows are seen.
- On a failed save: a summary at the top of the panel body, `Couldn't create the team: 2 things to fix` (Edit: `Couldn't save the team: …`), one link per error in screen order (`Team name: you have a team called Docs already`), each opening the row and focusing the field. The summary takes focus; the footer repeats the count in text (`2 things to fix, listed at the top.`). Its title is danger-ink on danger-soft (5.9:1) and its links ink-primary, underlined (the link blue is 4.3:1 on danger-soft).
- Fields with an error get a danger border (#ef4444, 3.8:1), `aria-invalid` and the sentence; colour is never the only signal.
- Two coordinators can't be made in the panel (one radio group), but today's agent form can leave a team with two. Edit team opens such a team with no radio checked, both rows badged, and the error under the legend at once: `{a} and {b} both coordinate. Pick the one that takes the team's tasks.`
- A server error that isn't about a field (network, a 5xx): the same summary, `Couldn't save the team` (or `Couldn't create the team`) with the server's words, or `Couldn't reach Agentry. Check it's running, then try again.` for a failed fetch. Nothing typed is lost; Save stays on to try again.
- A removed agent still needed by a task: the server's 409 shows in the summary, quoted, and on the agent's removed stub: `Still needed by {task}. Finish it first, or undo to keep {agent}.`, with the task linked and Undo.
- The team changed somewhere else since the panel opened (decision 3): `Couldn't save the team: it changed since you opened it`, with `Reload the team` and `Keep my changes here` ([opening from a link](custom-teams/states-url.svg) E).

## Is a team added from a built-in template a custom team?

**Yes: every team opens in the same panel.** A team is only the agents that share `team`; there's no team record, and a template-made team's agents are already editable one by one on their own pages. Allowing the same edits together, with the team checks, is safer than the agent-by-agent route it replaces, and a second kind of team would need a rule the user can't see. What the spec rules out, changing the built-in template itself, stays out: editing a team never touches the template, and `Change it first` makes a copy.

One thing must hold: a team's features follow its coordinator's `template` (`teamMadeFrom`, `Roster.TemplateOf`): the Visionaries team turns Ideas on. So when you pick another agent as coordinator, or rename the team, the team keeps its template id (the server moves `template` to the new coordinator). Removing the team, or every agent the template made, is the only way to lose it.

## Decisions for the engineer

1. **A side panel, not a dialog and not inline on the card.** The panel is a modal side sheet, 680px wide on desktop and full screen under 640px, at `/agents?team=new` or `?team=<name>` so a reload or a link reopens it (history and loading: decision 11). Focus is trapped and the page behind is inert under a flat scrim; Esc and Close ask first when something changed. *Rejected:* a centred dialog (a team of five open rows is taller than any dialog, and nesting it over Add a team would stack two modals); inline on the team card (the org chart would reflow as you type, a new team has no card yet, and errors would scatter across the page).
2. **Agents are edited inline, as accordion rows in the panel, not in a nested dialog.** One row open at a time; a row with an error stays open; collapsed rows show name, badges, description and any `1 problem`; an open row puts the prompt across its full width. *Rejected:* a dialog per agent (a modal on a modal, focus juggling on every agent, and the coordinator radio group would be split across surfaces, so "exactly one" couldn't be a real radio group).
3. **The team is saved all at once, in one checked request.** A new route takes the whole team: `PUT /api/teams/{name}` (or `POST /api/teams` with `agents` for a new one) with the team name, its agents (each with its current name and, when renamed, the name it had), the policy hash when accepting the missing-profile offer, and **the team's version as the panel loaded it**: the id of the last event that touched any of its agents (or a hash of them). The server checks every rule (one coordinator, names unique against all agents, team name free, profiles known, removed agents not needed by an unfinished task, the version still current) and writes the result as one batch of events under one `Propose`, so it lands whole or not at all. **There is no partial failure:** a refusal returns every problem at once (field-level ones keyed by row and field, the rest as text), the panel shows them and keeps everything typed. A version mismatch (an agent edited on its own page, `+ Add agent` on `/agents/new`, another tab) is a 409 "changed elsewhere", which the panel shows as [states-url.svg](custom-teams/states-url.svg) E: reload, asking first, or keep the changes here. *Rejected:* agent by agent with the existing routes; a failure halfway leaves a team that breaks the rules (moving the coordinator is two writes, one of which can fail and leave two or none), renames have to be ordered against new names, and nothing can roll back.
4. **Renames and tasks.** Tasks name their coordinator agent (`tasks.Task.Coordinator`), not their team, so renaming a team moves no task: they follow their agents. An agent rename goes through the same rename the existing route does, so its tasks go on with the new name. Two things for the server: a team keeps its template id when it is renamed or its coordinator changes (so Ideas stays on for Visionaries), and an open task whose coordinator stops coordinating should move to the new coordinator; *open, for tech-lead to decide.* Past decisions and reports keep the old names as written.
5. **User templates: where they appear, the routes the UI needs, and delete.** They appear in the Add a team dialog under "Your templates", between "Build your own" and "Built in", and in the panel's template select under "Your templates". The UI needs three routes (the shape, not the storage):
   - **List**: built-in and yours together, each marked as one or the other; yours carry the saved date and the source team's name. Either `GET /api/teams/templates` extended with these fields or a sibling route.
   - **Create**: from a team **as last saved** (not the panel's draft), with a template name and an optional description. Refused with 409 when the name matches one of yours or a built-in.
   - **Delete**: one of yours, by name. Built-ins can't be deleted.
   `Add this team` on one of yours goes through `POST /api/teams`, as a built-in does, including the missing-profile 409 and `addMissingProfiles`. Delete is a quiet button on the card that turns the card's actions into an in-place confirm, focus on Keep, Esc is Keep; no second dialog.
6. **Focus.**
   - Panel opens: focus on Team name (New) or on the panel title (Edit, so screen readers hear which team; also the not-found title). Closes: focus back to the button that opened it, or to the new team's heading after Create team.
   - Add agent: the new row opens and its Name takes focus.
   - Remove agent: focus goes to the stub's Undo; Undo puts it back on the restored row's name button.
   - Failed save: the error summary at the top of the panel body takes focus (`tabindex="-1"`, no `role="alert"`: the focus announces it); each link opens its row and focuses its field.
   - Template delete: focus to the next card's first button, or `Build a team`.
   - Confirms in place (Replace, Remove team, Delete template, Discard changes, Reload the team) start on the safe button.
7. **Every question is asked in place, never in a second dialog.** Discard changes, Remove team, Save as a template and a refused removal take over the panel's footer; Replace takes the space under Start from; Delete template takes the card's actions; Reload the team asks inside its box. The footer is sticky on desktop, so its question is always in view, and it becomes a group named by the question. On a phone, Remove team and Save as a template ask at the end of the body instead (decision 12).
8. **While saving, the panel is read-only and can't close.** Cancel, Close and Esc are off until the one request answers (it is short), so the result is never lost. Fields are `readonly`, not disabled, so they stay readable, focusable and read out; radios, swatches, selects, Add agent, remove and Undo are `aria-disabled`. Success closes the panel; a failure makes everything editable again, as it was. *Rejected:* editable during the save; edits made meanwhile would have to be replayed on top of the saved team (renames, a new team becoming an Edit panel) for a gain of a second or two.
9. **The prompt editor in the panel runs with Monaco's `tabFocusMode` on** and `alwaysConsumeMouseWheel: false`, so Tab and the wheel never get stuck in it. Its height is 8 to 24 lines; the agent's own page keeps the tall editor.
10. **Text colours use the `*-ink` tokens, and `Banner` and `Button` danger switch to them in task-178.** The summary title, field errors and `Button` danger text are drawn in danger-ink (#9c3739 light, #eb8587 dark), warnings in warning-ink, the info offer's title in info-ink. Today `Banner` titles use the plain tone (danger 3.2:1, info 3.5:1 in light) and `Button` danger uses `text-danger` (3.8:1 in light). Changing `Banner` titles and `Button` danger to the ink tokens is a two-line change and is in scope for task-178; without it the summary title fails 1.4.3.
11. **The URL, history and loading.**
    - Opening the panel from the page pushes one history entry (`?team=…`). Browser Back, or the phone's back gesture, with no changes closes the panel. With changes, the panel puts the entry back at once and asks Close's question in the footer, focus on Keep editing. Closing any other way (Close, Cancel, Discard, a save) goes back over our entry, so Back never reopens a closed panel. Opened from a link or a reload there is no entry of ours: closing replaces `?team=…` with `/agents`. Reload and closing the tab get the browser's own `beforeunload` prompt when something changed.
    - Opened cold, Edit shows its title from the link and `Loading the {team} team…` (polite) until the agents, engines (`/api/config`), profiles (`/api/policy`) and templates are in; no fields show, and Save changes is off with `Loading…` beside it. New team lets you type the Team name and pick Start from at once; the template select says `Loading templates…` and Create team is off with `Loading…` until engines and profiles are in.
    - A failed load shows `Couldn't load the team` (New: `Couldn't load what a new team needs`) with the server's words and `Try again`; only Cancel is in the footer.
    - No team by that name (renamed or removed, an old link, another tab): the panel title is `Couldn't find the {team} team`, with no fields and `Back to the Agents page`. Never an empty Edit panel.
    Drawn in [opening from a link](custom-teams/states-url.svg).
12. **Phone footer and short viewports (WCAG 1.4.10 Reflow, 2.4.11 Focus Not Obscured).** Under 640px the sticky footer holds only Cancel and the primary button, side by side in one 44px row (about 70px with padding). Save as a template and Remove team, their reasons and their questions, move to the end of the body, after `+ Add agent`, under the h3 `This team` (Edit only). When the viewport is under about 500px tall (`max-height: 500px`: 400% zoom at 320 by 256, a phone held sideways, the on-screen keyboard), the header and footer stop sticking and scroll with the body; the footer comes last. Drawn in [phone](custom-teams/phone-390.svg) 3 and 5.
13. **One scroll rule.** The panel body is its own scroll container between the header and the footer, so a focused field, or a summary link's target, never lands under either. No `scroll-margin` or `scroll-padding` compensation anywhere. Under 500px of height the whole panel is the one scroller and nothing sticks (decision 12), so the same holds.

## For the writer

Everything on screen is settled in [the copy deck](custom-teams/copy.md). The critique's fixes need two more changes there:

- **Add this team names.** The Add a team table should give `Add this team` and `Add another` the accessible names `Add this team, {template}` and `Add another, {template}`, as `Change it first` has (critique n9).
- **Drop "Toast, saved with later edits"** (`Saved the {team} team` · `Your later changes aren't saved yet.`): fields are read-only while saving (decision 8), so there are no later changes.

## Open questions and assumptions

For tech-lead:

- **An open task whose coordinator is replaced:** does it move to the new coordinator (decision 4)? Assumed yes in the drawings; the server work in task-178 needs the answer.
- **Primary buttons fail contrast in light:** `Button` primary is white on `bg-brand` (#fc6d26), about 2.9:1. App-wide, not new here; the wireframes draw it as it is. Worth its own task (a darker brand fill in light, as `--color-focus` already does).

Assumptions:

- Template names must differ from built-in names, to keep the Add a team list unambiguous (the deck has the error).
- Input borders (divider on surface) are under 3:1 in both themes (WCAG 1.4.11), and the Input placeholder is ink-faint (2.6:1 in light). App-wide; the panel only overrides the coordinator prompt's placeholder.
- The server has no prompt length limit today; if it gets one, it sends the number and the deck's `[server]` error is shown under the box.
- The Input atom is 32px tall; the wireframes draw 36px on desktop and 44px on a phone (the phone size meets 2.5.8 with room for thumbs). The phone size is a panel-only class, not an atom change.
- The team's version for decision 3 can be read from the event log the server already keeps; if it can't, a hash of the team's agents as loaded does the same job.

## States

Each drawing has numbered notes for the behaviour that isn't visible.

- **Empty** · [states-empty.svg](custom-teams/states-empty.svg): New team with Blank chosen (one `coordinator` row, numbered if taken, its prompt empty, the default prompt note), the list with no agents left, the Agents page before any team of your own (every heading already has Edit team), and Add a team with no "Your templates" group at all.
- **Long prompt** · [states-long-prompt.svg](custom-teams/states-long-prompt.svg): a 61-line prompt at its 24-line limit scrolling inside the box, with the line count beside the label, and the same row collapsed to one line. Also where the open row's layout is explained.
- **Validation errors** · [states-validation.svg](custom-teams/states-validation.svg): the summary with focus and its five links (team name taken, no coordinator, duplicate name in the team, a name another team's agent has, unknown profile), each error under its field, `1 problem` on closed rows, where focus lands after a link, and two coordinators found on opening a team.
- **Missing-profile offer** · [states-missing-profile.svg](custom-teams/states-missing-profile.svg): Visionaries copied, its five agents on a profile policy.toml lacks, said under the legend, on closed rows and in the open row before save; `MissingProfilesOffer` at the top of the panel after the 409; the offer while adding.
- **Saving and server errors** · [states-saving-error.svg](custom-teams/states-saving-error.svg): `Saving…` with the panel unable to close and the fields read-only; a failed save with the server's words and everything kept; a save refused because a removed agent is needed by a task; Remove team refused in its footer confirm, naming the tasks.
- **Opening from a link** · [states-url.svg](custom-teams/states-url.svg): Edit and New while loading, a failed load with Try again, no team by that name, the save refused because the team changed somewhere else (with the reload question), and browser Back with unsaved changes.
- **Unsaved changes and replace** · [states-unsaved-and-replace.svg](custom-teams/states-unsaved-and-replace.svg): Esc or Close with changes, `Open drafter's page` with changes, and switching template after changing agents.
- **Phone, 390px** · [phone-390.svg](custom-teams/phone-390.svg): the panel full screen, rows on two lines, an open row in one column, the one-row footer, This team at the end of Edit's body, a 320 by 256 frame where nothing sticks, and Add a team in one column.
- **Dark theme** · [dark-new-team.svg](custom-teams/dark-new-team.svg): the New team panel in the dark tokens, with a field error, and every text colour's contrast listed.

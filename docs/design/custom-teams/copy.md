# Build and edit a custom team: copy deck

The final words for [the team panel design](../custom-teams.md), for [the spec](spec.md). This is the one list of strings for the work: the design note's quoted drafts and `[draft]` marks are settled here, and the wireframes carry these words. Grouped by screen and state, each table names the wireframe and the element, the final text, its accessible name when that differs, plurals and a length limit.

Wireframes: [New team](new-team-desktop.svg), [Edit a team](edit-team-desktop.svg), [Add a team](add-team-list.svg), [empty](states-empty.svg), [long prompt](states-long-prompt.svg), [validation](states-validation.svg), [missing profile](states-missing-profile.svg), [saving and errors](states-saving-error.svg), [unsaved and replace](states-unsaved-and-replace.svg), [opening from a link](states-url.svg), [phone](phone-390.svg), [dark](dark-new-team.svg).

**Who reads it.** The one person who runs Agentry, a developer, putting together a team of their own in one sitting and changing it later. They know their agents by name and know what a prompt is; they don't know, and shouldn't need to, the rules a team must follow or how the server stores it. So: say what a choice does to their team, name the rule only when it is broken, and say how to fix it.

**Voice.** Plain, short, sentence case, no exclamation marks, no blame. The app already says "you" and writes agent names lowercase (`writer`); team and template names keep the case you typed (`Docs`). Buttons are a verb and its object. Errors are calm: what is wrong, then the one thing to do. Success is a quiet fact.

**Conventions in the tables**

- `{team}`, `{agent}`, `{n}` are values filled in at runtime. Text in a code span is exactly what shows.
- *Today* means the string ships already and stays. *Changed* means it differs from the draft in the design note or the wireframe, with the reason in Notes.
- Badges are written lowercase; the `Badge` atom shows them in capitals. Screen readers read the lowercase text.
- "Max" is characters at the 390px layout (342px of text at 12px is about 52 characters a line); where it says "lines", the string may wrap that many. Nothing is cut except where it says *truncates*.
- Accessible names start with the visible words (WCAG 2.5.3), then add what makes them unique: `Edit team, Docs`.
- `[server]` marks words the server must supply. Don't ship a placeholder for them: until the server sends a message, show the title alone.

## Glossary

| Term | Means | Use | Don't use |
|---|---|---|---|
| team | the agents that share a team name, with one coordinator | "the Docs team", `New team`, `Edit team` | squad, group, crew |
| agent | one AI worker in a team, by its lowercase name | `writer`, "4 agents" | bot, member, worker (a worker is a machine) |
| coordinator / coordinates | the one agent that takes the team's tasks and hands each step to another | badge `coordinator`, radio `Coordinator`, "one coordinates" | lead (except a name like `tech-lead`), manager, owner |
| template | a ready-made team to add: built in, or saved by you | "Your templates", "Built in", `Save as a template` | preset, blueprint, my template |
| prompt | an agent's standing instructions, read at the start of every turn | `Prompt` | system prompt, instructions, role prompt |
| profile | what an agent may do without asking you, from `policy.toml` | `Policy profile`, "profile missing" | permissions, role, policy (alone) |
| policy.toml | your policy file, named as it is named on disk | `policy.toml`; "your policy file" only in today's offer title | the policy, config |
| Its work counts as | which board column the agent's tasks show in: Building, Review, Shipping | AgentForm's options and hints, word for word | column (on screen), stage, lane |
| knowledge | what an agent has learned, on its own page | "knowledge" (its panel is titled Knowledge) | memory, notes |
| where it runs | the machines an agent may run on (its page's "Runs on") | "where it runs" | workers, nodes, machines (alone) |
| changed / new / removed | an unsaved difference in the panel | badges `edited`, `new`; stub "will be removed when you save" | dirty, pending, draft (on screen) |
| save / create | Create team makes a new team; Save changes saves an edited one | `Create team`, `Save changes` | submit, apply, OK, done |
| remove / delete | remove a team or an agent (as today); delete a template | `Remove team`, `Remove {agent}`, `Delete template` | erase, destroy, discard (for these) |

Words the team uses that never show: *panel*, *row*, *accordion*, *draft*, *409*, *payload*, *batch*, *event*, *hash*, *template id*.

**Why "Building / Review / Shipping", not "Builds / Reviews / Ships".** The spec's shorthand names the column values. On screen the select sits under "Its work counts as", where AgentForm already says `Building`, `Review`, `Shipping` and each hint starts with that word, and the team cards' `ColumnBadge` says `review` / `shipping`. Keeping those words means the same choice reads the same on the agent page, in the panel and on the cards. *Changed* in every wireframe (`Builds` → `Building`, the `Reviews` badge → `review`).

**Why "Save as a template", not "Save as my template".** The template then shows under "Your templates": "my" on the button and "your" on the list name the same thing two ways, and nowhere else does the app speak as the user. *Changed* everywhere.

## Agents page: team headings

[states-empty.svg](states-empty.svg) B, [edit-team-desktop.svg](edit-team-desktop.svg) inset A.

| Where | Text | Accessible name | Max | Notes |
|---|---|---|---|---|
| Heading action 1 (new) | `Edit team` | `Edit team, {team}` | | Opens the panel. Every heading, including teams added from a built-in template |
| Heading action 2 | `+ Add agent` | `Add agent to {team}` | | Today's words. Now opens the panel with a new agent open. The accessible name drops the "+" (read as "plus") and names the team; *changed* from none |
| Heading action 3 | `Remove team` | `Remove team, {team}` | | Today's words; *changed* accessible name, so three headings' buttons tell apart in a list of buttons |
| Header actions | `Add a team`, `New agent` | | | Today. New agent stays for a single agent |

## The team panel: header and team

[new-team-desktop.svg](new-team-desktop.svg), [edit-team-desktop.svg](edit-team-desktop.svg), [states-empty.svg](states-empty.svg) A, [phone-390.svg](phone-390.svg) 1 and 3.

| Where | Text | Accessible name / aria | Max | Notes |
|---|---|---|---|---|
| Panel title, new | `New team` | the panel's `aria-labelledby` | | |
| Panel title, edit | `Edit the {team} team` | as visible | truncates past 24 characters on a phone | The saved name, until saved |
| Close (×) | (icon) | `Close` | | Asks first with changes (see [Unsaved changes](#unsaved-changes)) |
| Team name label | `Team name` | | | |
| Team name hint, new | `Shown on the Agents page and on New task.` | | 1 line | *Changed* from "Shown on its card…": the page shows a team as a heading, not a card |
| Team name hint, edit, unchanged | `Shown on the Agents page and on New task.` | | 1 line | |
| Team name hint, edit, renamed | `Renames {old}: its agents and their tasks move with it.` | | 2 lines | Replaces the hint once the name differs from the saved one, as AgentForm's rename hint does. *Changed* "." to ":" to match AgentForm |
| Team name value on a copy | `{template}`, or `{template} {n}` if taken | | | e.g. `Writing 2`. Blank suggests nothing |
| Start from label | `Start from` | radiogroup name | | New team only |
| Start from choice 1 | `Blank` | | | |
| Start from choice 2 | `Copy a template` | | | |
| Blank hint | `A coordinator to start; add the agents it hands work to.` | | 2 lines | |
| Copy hint | `Fills every agent from the template; change any of them below.` | | 2 lines | |
| Template select | `{template}` | `Template` | | Option groups `Your templates`, then `Built in`. The closed select shows the name alone, as a native select does (question 1) |
| Template select, loading | `Loading templates…` | `Template` | | Off until the templates arrive. Blank works meanwhile |
| Blank's first agent | `coordinator`, or `coordinator-{n}` if taken | | | The name only, not shown as a label. Numbered as copies are (`coordinator-2`, then `-3`), so a second Blank team starts valid |

## The agents list

| Where | Text | Accessible name / aria | Plural | Max | Notes |
|---|---|---|---|---|---|
| Fieldset legend | `Agents · one coordinates` | the radio group's legend | | | Final; was `[draft]` |
| Count, right of legend, new | `{n} agents` | | `1 agent` | | |
| Count, edit, with changes | `{n} agents · {c} changed` | | `1 changed` | 24 | Counts added, removed and edited agents; the team name isn't counted |
| Coordinator radio label | `Coordinator` | `Coordinator, {agent}` | | | *Changed* accessible name from "{name} coordinates": it must start with the visible word. A screen reader says "Coordinator, editor, radio button, 1 of 4, checked" |
| Coordinator badge | `coordinator` | | | | Today, on the team cards |
| Column badge | `review` / `shipping` | | | | Today's `ColumnBadge`; none for Building. *Changed* from `Reviews` |
| Row name button | `{agent}` | `{agent}`, then the marks, e.g. `drafter, edited, was writer, 1 problem` | | names ≤31 | `aria-expanded` says open or collapsed |
| Mark: renamed | `was {old}` | | | | Until saved |
| Mark: changed | `edited` | | | | Badge. Until saved |
| Mark: added | `new` | | | | Badge. Until saved |
| Mark: errors | `1 problem` | | `{n} problems` | | Danger ink, with the icon |
| Mark: profile | `profile missing` | | | | Warning ink, with the icon; not counted as a problem |
| Description on a closed row | the agent's description | | | truncates | One line, ellipsis |
| Remove agent (trash) | (icon) | `Remove {agent}` | | | Today's words from the team cards |
| Removed stub, new team | `{agent} removed` · `Undo` | Undo: `Undo, keep {agent}` | | | |
| Removed stub, edit | `{agent} will be removed when you save` · `Undo` | Undo: `Undo, keep {agent}` | | 2 lines | *Changed* from "removed when you save", which read as done already |
| Empty list | `No agents yet. Add one: it coordinates the team.` | | | 2 lines | *Changed*: says how to start and what the first agent becomes, not only the rule |
| Add agent | `+ Add agent` | `Add agent` | | | |
| Add agent hint, Blank | `Add the agents it hands work to.` | | | | Beside Add agent while the team is one coordinator |
| Missing-profile line under legend | `{n} agents use {profile}, which policy.toml doesn't have. {Action} offers to add it.` | | `1 agent uses`; two profiles: `use read-mostly and research-only, which policy.toml doesn't have. … offers to add them.` | 3 lines | `{Action}` is `Create team` or `Save changes`. A warning, not an error |

## An open agent

[new-team-desktop.svg](new-team-desktop.svg), [states-long-prompt.svg](states-long-prompt.svg), [phone-390.svg](phone-390.svg) 2. Labels and hints are AgentForm's unless *changed*.

| Field | Label | Hint | Placeholder | Notes |
|---|---|---|---|---|
| Name | `Name` | `Lowercase letters, digits and dashes.` | none | Today |
| Name, renamed (edit) | `Name` | `Renames {old}: it keeps its prompt, settings and what it knows, and its tasks go on with the new name.` | | Today's AgentForm words |
| Description | `Description` | `One line. The coordinator reads it to choose who works next.` | none | Today. Two lines in the desktop column; the drawings that cut it short now carry it whole |
| Prompt | `Prompt` | `Its standing instructions, at the start of every turn. Tab moves to the next field.` | none | *Changed* from "Markdown. 8 lines high, grows to 24…": how the box grows is seen, not read; what a prompt is and that Tab leaves the box are not. First half is the agent page's Prompt panel aside |
| Prompt line count | `{n} lines` | | | `1 line`. In Edit, `{n} lines · changed` once it differs from the saved prompt |
| Prompt, coordinator, empty | `Prompt` | `Empty: it uses Agentry's coordinator prompt for a team of these agents.` | the same sentence, in ink-muted | Final; was `[draft]`. Link under it: `See the default`; it shows the default read-only in the box, and becomes `Hide the default` |
| Prompt note, edit | | `Renaming an agent doesn't change its prompt.` | | Edit only, under the hint |
| Prompt note, edit, renamed | | `Renaming doesn't change its prompt: it may still say "{old}".` | | Replaces the line above once the Name differs. *Changed* from a question ("The prompt still says…?") |
| Prompt too long | | error: `This prompt is too long: at most {n} characters.` | | `[server]`: the server has no limit today; if it gets one, it sends {n}. Don't draw or ship it until then |
| Colour | `Colour` | `{Colour}. Agents can share a colour.` | | e.g. `Blue. Agents can share a colour.` *Changed* from "Its name is always shown beside it", which reads as the colour's name; this says the one thing you might wonder |
| Colour swatch | (swatch) | | | Accessible name: the colour, today (`blue`) |
| Its work counts as | `Its work counts as` | AgentForm's hint for the choice, e.g. `Building. A change it makes is checked again by the team's reviewers before the task is done. Its tasks show in In progress on the board.` | | Options `Building`, `Review`, `Shipping` (today). The panel shows the hint's first sentence and the column line; see below |
| Its work counts as, short hint | | Building: `Checked by the team's reviewers. Its tasks show in In progress.` Review: `Its approval is needed to finish. Its tasks show in Review.` Shipping: `Ships checked work, such as deploying. Its tasks show in Shipping.` | | For the panel's narrow column, two lines (the desktop drawing cut AgentForm's hint mid-sentence). The option already names the column, so the hint starts with what it means |
| Its work counts as, coordinator | (disabled) | `Not used: a coordinator hands the work out.` | | *Changed* from "A coordinator doesn't work on tasks.", which isn't true: it works on every task, by handing it on |
| Engine | `Engine` | | | Today |
| Model | `Model` | `Empty for the engine's default.` | none | Today. In the drawings the hint sits under both fields, so it reads `Model: empty for the engine's default.` |
| Policy profile | `Policy profile` | `What it may do without asking you.` | | Today's hint minus ", from policy.toml" (the column is too narrow; the option names already say policy.toml) |
| Policy profile options | `default ({profile})`, then the profiles, then `{profile} (missing from policy.toml)` | | | Today |
| Policy profile, missing | | `{profile} (missing from policy.toml): every call it makes is denied until it's back. {Action} offers to add it, or pick another.` | | AgentForm's words with the panel's ending; `{Action}` is `Create team` or `Save changes` |
| Policy profile, unknown | option `{profile} (not in policy.toml)` | error below | | A name no default profile has |
| Its page, new team | `Knowledge and where it runs: on its own page, once the team is created.` | | | *Changed* from "Machines and knowledge…once the team is made": the page calls them Knowledge and Runs on |
| Its page, new agent in Edit | `Knowledge and where it runs: on its own page, once you save.` | | | |
| Its page, edit | `Knowledge, where it runs and a taller prompt editor:` then the link `Open {agent}'s page` | link as visible | | *Changed* from "Open on its own page", which says nothing out of context. The link goes to the saved name's page. Where the link sits above (edit-team-desktop.svg), the line under it is `For knowledge and where it runs.` |

## Footer

| Where | Text | Accessible name / aria | Max | Notes |
|---|---|---|---|---|
| New: primary | `Create team` | | | Final. Never disabled |
| New: busy | `Creating…` | | | Off while busy |
| New: left of the buttons | `Creates the team and its {n} agents.` | | one agent: `Creates the team and its coordinator.` | *Changed* from "Saves the whole team at once.", which describes the mechanism; this says what you get |
| Edit: primary | `Save changes` | | | Off until something changed (design) |
| Edit: primary, off | reason beside it: `No changes to save.` | button `aria-describedby` the reason | | Shown while nothing has changed. Settled (question 3) |
| Loading: primary off | reason beside it: `Loading…` | `aria-describedby` | | While the team or what a new team needs is loading |
| Edit: busy | `Saving…` | | | |
| Cancel | `Cancel` | | | Off while busy. Asks first with changes |
| Busy line, footer | `Creating the team…` / `Saving the team…` | polite live region | | Drawn in [saving](states-saving-error.svg) A |
| Problem count | `{n} things to fix, listed at the top.` | | `1 thing to fix, listed at the top.` | |
| Save as a template | `Save as a template` | | | *Changed* from "Save as my template" (see above) |
| Save as a template, off | reason beside it: `Save your changes first.` | button `aria-describedby` the reason | | On a phone the reason sits under the button, in the body (below) |
| Remove team | `Remove team` | | | Danger text button |
| Remove team, off (last team) | reason beside it: `Can't remove your only team: tasks need its coordinator.` | `aria-describedby` | 2 lines | *Changed* from "Your only team stays: every task needs a coordinator.", which said a rule, not what you can't do |
| Phone: end of the body, heading | `This team` | the section's heading (h3) | | Edit only. Under 640px the footer keeps only the primary button and Cancel; Save as a template and Remove team, their reasons and their questions move here, after `+ Add agent` |

### Save as a template (the footer becomes a form)

[edit-team-desktop.svg](edit-team-desktop.svg) inset B.

| Where | Text | Notes |
|---|---|---|
| Field 1 | `Template name` | Prefilled with the team name |
| Field 2 | `What it's for` | Prefilled with the coordinator's description; one line, optional |
| Hint under both | `Keeps each agent's prompt and settings and which one coordinates; not knowledge or where they run.` | *Changed* to the glossary's words |
| Buttons | `Save template` · `Cancel` | Busy: `Saving…` |
| Error, name taken | `You have a template called {template} already. Pick another name.` | Under the field |
| Error, built-in name | `{template} is a built-in template. Pick another name.` | |
| Error, empty | `Give the template a name.` | |
| Error, server | `Couldn't save the template` + the server's words | In the footer, above the buttons |
| Toast | `Saved {template} to your templates` · `Add it again from Add a team.` | |

### Remove team (the footer becomes a question)

[edit-team-desktop.svg](edit-team-desktop.svg) inset C, [states-saving-error.svg](states-saving-error.svg) D.

| Where | Text | Notes |
|---|---|---|
| Question | `Remove the {team} team and its {n} agents?` | RemoveDialog's title, with the count. `its 1 agent` |
| Second line | `Their prompts and what they learned go too.` | Today. With unsaved changes: `Their prompts, what they learned and your unsaved changes go too.` |
| Buttons | `Keep` · `Remove` | Focus on Keep. Busy: `Removing…` |
| Refused | `It wasn't removed: "{server's words}"` then `Tasks: {task}, {task}` as links | Today's banner title; the server's words quoted as sent. Remove turns off until something changes |
| Toast | `Removed the {team} team` | Today |

## Validation

[states-validation.svg](states-validation.svg). Under each field, danger ink with the icon, tied by `aria-describedby`. The hint stays under the error.

| Problem | Under the field | Summary link (`{where}: {what}`) | Notes |
|---|---|---|---|
| Team name empty | `Give the team a name.` | `Team name: give the team a name` | Only on leaving after typing, or on save |
| Team name taken | `You have a team called {team} already. Pick another name.` | `Team name: you have a team called {team} already` | Any case counts as the same name |
| No coordinator | `Pick one agent to coordinate.` (under the legend) | `Agents: pick one agent to coordinate` | |
| Two coordinators (found on opening) | `{a} and {b} both coordinate. Pick the one that takes the team's tasks.` | `Agents: {a} and {b} both coordinate` | Shown at once, before any save. Three or more: `{a}, {b} and {c} all coordinate.` |
| Agent name empty | `Give this agent a name.` | `New agent: give it a name` | |
| Agent name, wrong letters | `Use only lowercase letters, digits and dashes.` | `{agent}: use only lowercase letters, digits and dashes` | |
| Agent name too long | `Use 31 characters or fewer.` | `{agent}: use 31 characters or fewer` | |
| Same name twice in the team | `Another agent in this team is called {agent}. Pick another name.` | `{agent} (second one): another agent in this team is called {agent}` | Only the later row is marked. *Changed* "(the second)" to "(second one)" |
| Name another team's agent has | `You have an agent called {agent} already, in the {team} team. Pick another name.` | `{agent}: you have an agent called {agent} already, in {team}` | Names are shared by all your agents |
| Prompt empty (not the coordinator) | `Write a prompt: only the coordinator can do without one.` | `{agent}: write a prompt` | |
| Prompt empty (the coordinator a team has, edit) | `Write a prompt: Agentry's coordinator prompt is only for a new agent.` | `{agent}: write a prompt` | Agentry writes its coordinator prompt only when it adds the agent |
| Unknown profile | `policy.toml has no profile called {profile}. Pick one it has.` | `{agent}: policy.toml has no profile called {profile}` | |

| Where | Text | Accessible name / aria | Plural | Notes |
|---|---|---|---|---|
| Summary title, new | `Couldn't create the team: {n} things to fix` | heading in a `tabindex="-1"` region that takes focus | `1 thing to fix` | *Changed* from "The team wasn't saved: fix 5 things" to the app's `Couldn't {verb} the {thing}` titles, and "create" for a new team, so it names the button you pressed |
| Summary title, edit | `Couldn't save the team: {n} things to fix` | | | |
| Summary links | as in the table above, in screen order | each link's text | | One link per problem; a link may wrap to two lines on a phone |

## Missing profile

[states-missing-profile.svg](states-missing-profile.svg). Before save, the agents list and the open row say it (above). After the server's refusal, today's `MissingProfilesOffer` moves to the top of the panel body.

| Where | Text | Notes |
|---|---|---|
| Offer title | `The {team} team needs {profiles}, which your policy file doesn't have` | Today. `{profiles}` is `the read-mostly profile` / `the a and b profiles` (`profilesNamed`) |
| Offer body, new | `Agentry adds the default text to policy.toml, keeping a backup, then creates the {team} team.` | Today's, ending "creates" |
| Offer body, edit | `Agentry adds the default text to policy.toml, keeping a backup, then saves your changes.` | |
| Offer button, new | `Add the profile and create the team` | Two or more: `Add the profiles and create the team`. *Changed* from the design's "Add the profile and save the team" for New, to name the footer's verb |
| Offer button, edit | `Add the profile and save the team` | Plural as above |
| Offer button, busy | `Adding…` | Today |
| Policy changed meanwhile | `Couldn't create the team` + `[server]` words such as `policy.toml changed since you opened this. Check it, then save again.` | `[server]`: the server must send this; until it does, show its own message |

## Opening the panel from a link

[states-url.svg](states-url.svg). The panel opens from `?team=new` or `?team={team}`, so a reload, a link or Back can open it before anything has loaded.

| Where | Text | Accessible name / aria | Notes |
|---|---|---|---|
| Loading, edit: panel title | `Edit the {team} team` | | From the link, so it shows at once |
| Loading, edit: body | `Loading the {team} team…` | polite live region | Fields aren't shown until the agents, engines, profiles and templates are in |
| Loading, new: agents list | `Loading engines and profiles…` | polite live region | Team name and Start from work meanwhile; the template select says `Loading templates…` |
| Load failed, edit: title | `Couldn't load the team` | | Today's `Couldn't load the {thing}` banner. Body: the server's words, or the network line below |
| Load failed, new: title | `Couldn't load what a new team needs` | | Body as above. Team name keeps what was typed |
| Load failed, network | `Couldn't reach Agentry. Check it's running, then try again.` | | As for a failed save |
| Load failed: button | `Try again` | | Today's words (ActivityFeed) |
| Not found: panel title | `Couldn't find the {team} team` | the panel's `aria-labelledby` | `{team}` is the name in the link |
| Not found: body | `It may have been renamed or removed since this link was made. Its agents, if any are left, are on the Agents page.` | | 3 lines on a phone |
| Not found: button | `Back to the Agents page` | | Closes the panel. Close (×) does the same |

### Changed somewhere else

[states-url.svg](states-url.svg) E. The team was changed after the panel opened, in another tab, on an agent's page or by `+ Add agent` on `/agents/new`. The save is refused; nothing typed changes until you choose.

| Where | Text | Notes |
|---|---|---|
| Summary title | `Couldn't save the team: it changed since you opened it` | In the summary place, focus on it |
| Body | `The {team} team was changed somewhere else after you opened it. Copy anything you want to keep, then reload to see the team as it is now.` | `[server]` words, when the server sends them, go after it, quoted |
| Buttons | `Reload the team` · `Keep my changes here` | Focus on the title; `Keep my changes here` closes the box and leaves Save on, so the next save is refused again until you reload |
| Reload asks first | `Reload and lose your changes?` · `Keep editing` · `Reload` | In place, focus on Keep editing |
| Renamed or removed meanwhile | the not-found state above | When reloading finds no team by that name |

## Saving and server errors

[states-saving-error.svg](states-saving-error.svg).

| Where | Text | Notes |
|---|---|---|
| Save failed, title | `Couldn't save the team` (edit) / `Couldn't create the team` (new) | *Changed* from "The team wasn't saved", as above |
| Save failed, body | the server's message as sent | |
| Save failed, network | `Couldn't reach Agentry. Check it's running, then try again.` | Ours, for a failed fetch only |
| Removed agent still needed: summary link | `{agent}: still needed by {task}` | then the server's message, quoted |
| Removed agent still needed: on the stub | `Still needed by {task}. Finish it first, or undo to keep {agent}.` | `{task}` links to the task. Two tasks: `by task-182 and task-190. Finish them first…` |
| Toast, saved | `Saved the {team} team` | |
| Toast, saved with later edits | `Saved the {team} team` · `Your later changes aren't saved yet.` | The panel stays open with them |
| Toast, created | `Added the {team} team` · `Pick it on New task; its coordinator takes the task.` | Today |

## Unsaved changes

[states-unsaved-and-replace.svg](states-unsaved-and-replace.svg).

| Where | Text | Notes |
|---|---|---|
| Close with changes, edit | `Discard your changes to {team}?` | Final |
| Its second line | `{changes}. They aren't saved.` | `{changes}` joins, in this order: `the team renamed`, `{n} agents changed` (`1 agent changed`), `{n} added`, `{n} removed`; e.g. `The team renamed, 1 agent changed, 1 added and 1 removed. They aren't saved.` Each agent counts once, in its own group: edited, new or removed. Sentence case on the first word |
| Close with changes, new | `Discard this new team?` | |
| Its second line | `It hasn't been created yet.` | |
| Buttons | `Keep editing` · `Discard` | Focus on Keep editing |
| Open a page with changes | `Open {agent}'s page and discard your changes?` | |
| Its second line | `Save first to keep them; the page shows what is saved.` | |
| Buttons | `Keep editing` · `Discard and open` | |
| A task link with changes | `Open {task} and discard your changes?` · `Keep editing` · `Discard and open` | |

## Replace the agents with another template

| Where | Text | Notes |
|---|---|---|
| Question | `Replace your {n} changed agents with {template}'s?` | `your changed agent` for one |
| Its second line | `{template}'s {m} agents replace {old}'s; your changes to {agents} go.` | e.g. `Research's 4 agents replace Writing's; your changes to writer, researcher and fact-checker go.` One line on desktop |
| Question, to Blank | `Replace your {n} changed agents with a blank team?` | |
| Its second line | `A blank team has one agent, its coordinator.` | |
| Buttons | `Keep mine` · `Replace` | Focus on Keep mine |
| Under the question | none | The drawing's "… the rows below stay as they are …" describes the state; it isn't on screen |

## Add a team

[add-team-list.svg](add-team-list.svg), [states-empty.svg](states-empty.svg) C, [phone-390.svg](phone-390.svg) 4.

| Where | Text | Accessible name | Max | Notes |
|---|---|---|---|---|
| Dialog title | `Add a team` | | | Today |
| Intro | `Each team comes with a coordinator and its specialists, each with a prompt written for the work. Add one as it is, change it first, or build your own.` | | 4 lines | *Changed* end, from "…or add agents of your own with New agent" |
| Group 1 heading | `Build your own` | | | *Changed* from "Build your own team", which the card title repeated |
| Its card title | `A team of your own` | card group name | | |
| Its card body | `Start blank or copy a template, then change any agent.` | | 2 lines | |
| Its button | `Build a team` | | | |
| Its second button | `Import a template` | | | Opens a picker for `.json` files. Busy `Importing…` |
| Group 2 heading | `Your templates` | | | Only when you have one |
| Card line | `Saved {date} from the {team} team.` | | | `{date}` as `12 Oct`; `12 Oct 2025` outside this year |
| Delete template | `Delete template` | `Delete template, {template}` | | |
| Delete confirm | `Delete the {template} template?` · `Teams you made from it stay as they are.` | | 3 lines | |
| Its buttons | `Keep` · `Delete` | | | Focus on Keep. Busy `Deleting…` |
| Delete failed | `Couldn't delete the template` + the server's words | | | In the card |
| Toast | `Deleted the {template} template` | | | |
| Group 3 heading | `Built in` | | | |
| Badge | `you have it` | | | Today |
| Card button 1 | `Add this team` / `Add another` | | | Today. Busy `Adding…` |
| Card button 2 | `Change it first` | `Change it first, {template}` | | Final; was `[draft]`. Opens the panel copying it |
| Empty | no "Your templates" group | | | Nothing to say until there is one |

## Screen-reader announcements

Polite live region in the panel unless it says otherwise. Focus moves (in the design's decision 6) announce themselves and aren't repeated here.

| When | Announcement | Notes |
|---|---|---|
| Add agent | none | Focus lands in the new agent's Name, which is announced |
| Remove agent | `{agent} removed. Undo is next.` (new) / `{agent} will be removed when you save. Undo is next.` (edit) | Focus moves to Undo |
| Undo | `{agent} is back.` | Focus on its name button |
| Replace template | `Replaced with {template}'s {n} agents.` | |
| Replace with Blank | `Replaced with a blank team: one agent, its coordinator.` | Added in task-178: Blank's replace needs words too |
| Creating / saving | `Creating the team…` / `Saving the team…` | The footer line |
| Created / saved | the toast's title | Toasts are already a live region |
| Failed save | none | Focus moves to the summary; its heading and links are read |
| Template deleted | the toast's title | |

## Questions for the designer

1. **The template select's " · Built in".** A native select shows only the chosen option's text when closed; the group name shows in the open list. The drawings show `Writing · Built in` closed. Either drop it from the drawings (my preference) or say the closed select is custom.
2. **"Its work counts as" hint in the narrow column.** AgentForm's hints are two sentences, up to 150 characters; the panel's column fits about 46 a line. I wrote short hints (above); the drawings show the Building one. If you'd rather keep AgentForm's words whole, the column needs three lines there.
3. **Save changes off until something changes** while Create team is never off ("a disabled button can't say why"). With nothing changed there is nothing to say, so it is fine, but the off button could carry `No changes to save.` beside it as Save as a template does. Words ready if wanted.

## Answered

None yet.

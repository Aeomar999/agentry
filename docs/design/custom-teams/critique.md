# Critique: build and edit a custom team (the team panel)

I checked this against [the spec](spec.md), [the design note](../custom-teams.md) (flows, States, Decisions for the engineer, open questions), [the copy deck](copy.md) and all eleven wireframes. I rendered each wireframe to PNG and read it at full size: [New team](new-team-desktop.svg), [Edit a team](edit-team-desktop.svg), [Add a team](add-team-list.svg), [empty](states-empty.svg), [long prompt](states-long-prompt.svg), [validation](states-validation.svg), [missing profile](states-missing-profile.svg), [saving and errors](states-saving-error.svg), [unsaved and replace](states-unsaved-and-replace.svg), [phone](phone-390.svg) and [dark](dark-new-team.svg). I compared them with today's `AgentsPage.tsx` (`TeamHeading`, the Add a team dialog), `AgentForm.tsx`, the routes in `internal/web/server.go`, the built-in teams in `internal/agents/teams.go` and the tokens in `globals.css`. I recomputed every contrast ratio the design states from the tokens, using WCAG relative luminance and `color-mix` as a linear mix in sRGB.

Severities: **blocker** (can't ship), **major** (fails a spec measure or WCAG AA, or can't be built as written), **minor** (fix in this build), **nit** (polish).

The coordinator has already settled the writer's three questions: drop ` · Built in` from the closed picker, keep the two-line hints, and show `No changes to save.` beside the disabled Save changes. I don't reopen them. The contrast of the app-wide primary button (white on brand, 2.86:1 in light) belongs to its own task. I note it here but don't block on it.

## Summary

The direction is right and very thorough. One panel holds the whole team. A real radio group enforces "exactly one coordinator". The team saves in one request that lands whole or not at all. Every question is asked in place, and the safe answer has focus. Together these meet the spec's validity and "nothing lost" measures better than any agent-by-agent route could. Coverage of the states the spec lists is complete, the look rules hold, and the contrast numbers check out.

There are no blockers and two majors, both about behaviour the wireframes don't yet pin down:

1. On a phone, and at 400% zoom, the stacked sticky footer leaves almost no room for the content. At 320 × 256 it covers all of it.
2. The panel is driven by the URL, but the design never says what happens on browser Back, while the team loads, or when `?team=` names a team that no longer exists. Back on a phone can lose typed work, which breaks the spec's "nothing lost" measure.

## What works well

- **One surface, one save.** The modal side panel at `?team=` keeps the page and avoids a modal on a modal. It also lets the coordinator radio be one native group across rows (decisions 1 and 2). Saving in one checked request removes every half-saved state (decision 3). The rejected options are argued with reasons, not taste.
- **Error prevention built into the controls.** Copying a template numbers taken names on the client, so a copy is valid from the start. A team with two coordinators can't be made, and a team found with two opens with neither radio checked and the error already shown. Removing an agent leaves a stub with Undo instead of asking. Replacing changed rows asks first, with focus on `Keep mine`. Create team is never disabled, so the user is always told why a save failed.
- **Error recovery.** The summary takes focus, lists the problems in screen order, and each link opens its row and focuses the field. Closed rows show `1 problem`. The footer repeats the count. A removed agent that a task still needs is refused on its own stub, with the task linked and Undo beside it. The server's words are quoted as sent. Everything typed is kept.
- **Visible system status.** The `was writer` / `edited` / `new` marks, the "will be removed when you save" stub and `4 agents · 3 changed` show every unsaved change before saving. Saving shows `Saving…`, a polite live line and a panel that can't close.
- **Missing profile.** The design warns before the save (a line under the legend and `profile missing` on closed rows, in warning-ink with an icon) and then reuses today's `MissingProfilesOffer` after the 409. It correctly tells a default profile that policy.toml lacks (offered) apart from an unknown name (an error).
- **Long prompt.** Pass 2's full-width prompt, 8 to 24 lines with `tabFocusMode` and `alwaysConsumeMouseWheel: false`, avoids both keyboard and scroll traps. A collapsed row stays one line whatever the prompt's length.
- **Templates.** "Your templates" sits between "Build your own" and "Built in". Delete is confirmed in place with focus on Keep, and focus moves to the next card afterwards. `Change it first` on every card gives the spec's reuse measure in one action.
- **Copy.** The deck is consistent and plain. It keeps AgentForm's words (`Building / Review / Shipping`, the rename hints), its accessible names start with the visible text (2.5.3), and it marks `[server]` strings instead of shipping placeholders.
- **Look rules** hold in all eleven drawings. There are no `filter`, shadow or gradient elements, and the only mention of one is the comment "flat scrim: no blur, no shadow". There are no left accent stripes or coloured left borders. Every corner is `rx` 2 to 4; the larger `r` values are the colour dots, radios and note markers. Everything is one page plus the panel, with no new navigation; the query parameter is allowed by the spec.
- **Decision 10** finds the real `Banner` and `Button` danger contrast gaps and fixes them with existing tokens.

## Coverage against the spec

| Spec asks for | Where | |
|---|---|---|
| New team: name, Blank / Copy a template | new-team-desktop, states-empty A | ✓ |
| Add / remove agents | new-team-desktop 6–7, states-empty (empty list) | ✓ |
| Every agent field: name, prompt, engine, model, profile, colour, coordinator, column | new-team-desktop open row and inset | ✓ |
| Missing-profile offer | states-missing-profile A–C | ✓ |
| Edit: add / remove / rename agents, prompts, rename team, remove team, Save as a template | edit-team-desktop, insets B and C | ✓ (open row drawn in the pass 1 layout, m2) |
| Add a team with your templates and delete | add-team-list, states-empty C | ✓ |
| Empty, long prompt | states-empty, states-long-prompt | ✓ |
| Validation: no coordinator, two coordinators, duplicate name, team name taken, unknown profile | states-validation A–C | ✓ |
| Saving, server error | states-saving-error A–D | ✓ |
| Unsaved changes | states-unsaved-and-replace | ✓ (not browser Back, M2) |
| Phone 390 | phone-390 | ✓ (sticky footer, M1) |
| Dark | dark-new-team | ✓ |
| Loading, team not found | — | missing (M2) |
| Copy deck | copy.md | ✓ |
| Decisions for the engineer | custom-teams.md | ✓ (gaps m7, m8) |

## Contrast, checked

The design's stated numbers are correct:

| Pair | Stated | Measured |
|---|---|---|
| danger-ink #9c3739 on danger-soft (light) | 5.9 | 5.92 |
| danger-ink #eb8587 on surface-0 / danger-soft (dark) | 6.6 / 5.9 | 6.62 / 5.87 |
| link on danger-soft (light) | 4.3, so not used | 4.29 |
| warning-ink #96490f on white | 6.5 | 6.45 |
| warning #b45309 select border on white | 5:1 | 5.02 |
| info-ink on info-soft (light) | 5.3 | 5.28 |
| brand-ink chip on brand-soft, over surface-0 / surface-2 (light) | 5.7 / 5.1 | 5.68 / 5.06 |
| danger field border #ef4444 on white / dark surface-0 | 3.8 / 4.5 | 3.76 / 4.49 (≥3:1 non-text) |
| focus ring, light (`--color-focus` mix) on white / surface-1 | 3:1+ | 3.95 / 3.79 |
| focus ring, dark | 5.9 | 5.91 |
| ink-muted, light, on surface-0 / surface-1 | — | 4.83 / 4.63 |
| ink-muted, dark, on surface-0 / surface-2 | 5.3 / 4.9 | 5.34 / 4.86 |

One case to watch: light ink-muted on **surface-2 is 4.28:1**, which fails. The only ink-muted text on surface-2 in the drawings is the disabled coordinator column select (`Building`), and disabled controls are exempt. Keep hints and the "No changes to save." reason off surface-2 in light. The app-wide gaps the designer already raised remain open but don't block this design: primary button 2.86:1, Input borders under 3:1, and the ink-faint placeholder at 2.63:1.

## Findings

### Major

**M1. Phone and 400% zoom: the sticky footer covers the content.** `phone-390.svg` frames 1 and 3, note 2 and note 5; the phone's "No sideways scroll" note sets the 320px / 400% zoom bar (1.4.10).
On a phone, New team's footer stacks two 44px buttons, about 130px with padding. Edit's footer stacks Save changes, Cancel, the reason line, then Save as a template and Remove team, about 210px. Add the 56px header and Edit keeps about 266px stuck to the screen. At 390 × 844 that is already a third of the screen. With the on-screen keyboard open, the field being typed into gets roughly 200px. At 400% zoom (320 × 256 CSS px), the header and footer are taller than the viewport, so the body can't be seen at all. That fails Reflow (1.4.10) and makes Focus Not Obscured (2.4.11) impossible to meet with scroll-padding alone. The in-place questions (Discard, Remove team, the template form) take over this footer and "grow upwards", so the same problem gets worse.
*Fix:* under 640px, the sticky footer holds only the primary action and Cancel, side by side in one 44px row. Save as a template and Remove team move to the end of the body, after `+ Add agent`, as a non-sticky "Team" section, and their in-place questions open there. When the viewport is shorter than about 480px (zoom, landscape, keyboard), the header and footer stop sticking and scroll with the body. Draw the Edit phone footer again on these rules, and draw one frame at 320 × 256.

**M2. The panel follows the URL, but Back, loading and a missing team are undefined.** custom-teams.md decision 1 (`?team=new`, `?team=<name>`, "a reload or a link reopens it"), states-unsaved-and-replace "Leaving the page".
- **Back.** Opening the panel changes the URL. If that pushes a history entry, browser Back (and the phone's back gesture, the most common way out of a full-screen sheet) changes the URL and closes the panel without the discard question, so typed work is lost. That breaks the spec's "nothing lost" measure. If it doesn't push an entry, Back leaves `/agents` altogether. The note covers in-app links, reload and closing the tab, but not Back.
- **Loading.** Opening `?team=Docs` cold, for example after a reload, needs the agents, `/api/config` (engines), `/api/policy` (profiles) and the templates before the rows can show. Nothing is drawn or said for this: not what shows, whether Save is off, or what Copy a template does while templates load. A failed load isn't covered either (`Couldn't load the team`, Try again).
- **Team not found.** `?team=Docs` after Docs was renamed to Handbook or removed is reachable through Back after a successful rename, an old link or another tab. Without a defined state the panel is a dead end, or worse, an empty Edit panel for a team that doesn't exist.
*Fix:* (a) Opening the panel pushes one history entry. Back with no changes closes it. Back with changes puts the entry back and asks the same in-place question as Close (`Discard your changes to {team}?`, focus on Keep editing). Closing in any other way replaces the entry, so Back doesn't reopen a closed panel. (b) Draw a loading frame: header and Team name as skeleton or `Loading the team…` (polite), rows not shown yet, Save off. Add the load-failure banner with `Try again`. (c) Add a not-found state in the panel: `There's no team called {team}. It may have been renamed or removed.` with a Close button (writer to settle the words). Put all three in the copy deck and in States.

### Minor

**m1. The design note and the drawings' notes still carry strings the copy deck replaced, including one that breaks 2.5.3.** custom-teams.md lines 128 and 144 and the new-team-desktop note 5 still give the coordinator radio's accessible name as `"{name} coordinates"`. A name that doesn't start with the visible `Coordinator` fails Label in Name; the deck's `Coordinator, {agent}` is right. The add-team-list note 3 gives `"Delete the Handbook template"` against the deck's `Delete template, {template}`. custom-teams.md also still says "The team wasn't saved: fix 2 things", "Save as my template" (four times), "Open on its own page", "Machines and knowledge…", "A coordinator doesn't work on tasks." and "Build your own team". The note says the deck wins, but an engineer building from decisions and drawing notes will copy these.
*Fix:* replace them in custom-teams.md and the SVG notes with the deck's words, or remove the quoted strings and point to the deck.

**m2. Edit a team's open row is the rejected pass 1 layout.** `edit-team-desktop.svg`, the `drafter` row: the prompt sits in the left column at about 36 characters a line, and the settings are cut down to "as in the New team panel". Every other drawing uses the pass 2 layout (Name | Description, the prompt across the row, settings in two columns), and states-long-prompt note 1 says "All pass-2 drawings use this layout." The main Edit drawing is the one an engineer opens first.
*Fix:* redraw the `drafter` row in the pass 2 layout, with the rename hint under Name and the prompt note under the prompt.

**m3. The team heading's actions are drawn two ways.** edit-team-desktop inset A draws `Edit team` as a bordered button and `Remove team` in danger-ink. states-empty B draws all three as 12px ink-muted text buttons with a 28px hit area, which is today's `TeamHeading` look. Its note 6 says the same.
*Fix:* pick one. I'd keep today's quiet text actions (states-empty B), because the page has one heading per team and three bordered or red buttons on each would be loud. Then redraw inset A to match.

**m4. Blank names its agent `coordinator`, which collides the second time.** states-empty A, note 3. Agent names are global, so the second Blank team starts with a duplicate-name error on its only row before the user has typed anything.
*Fix:* number it as copies are numbered (`coordinator-2`), and say so in flow 1 step 2 and in the deck.

**m5. Scroll offsets are specified twice.** states-validation note 7 sets `scroll-margin-top` to the header height on the target. phone-390 note 2 sets "scroll-padding top and bottom = their heights". If the body is its own scroller between the header and footer (phone note 2: "The body scrolls between header and footer"), neither is needed. If both are set, the offset is doubled and the focused field lands a header's height too low.
*Fix:* state one rule in the decisions: the body is the scroll container between a fixed header and footer, so no offsets are needed. Otherwise use `scroll-padding` on the scroller only, and include the bottom so forward Tab doesn't hide a field under the footer (2.4.11).

**m6. Editing fields during a save is costly to build for little gain.** Decision 8 and states-saving-error note 2. Edits made while the request runs must be reapplied on top of the newly saved team: `was` marks against new names, a renamed team in the URL, and a New panel that becomes an Edit panel. The request "is short", so the gain is small.
*Fix:* make the fields read-only (not disabled, so they stay readable and focusable) while saving. If the designer keeps them editable, spell out the rules for reapplying edits after a rename and after a create.

**m7. Nothing guards against a team changed elsewhere since the panel opened.** Decision 3 sends the whole team, with a policy hash only. If an agent is edited on its own page in another tab, or added through `/agents/new?team=` (which stays), between opening and saving, the PUT overwrites it silently or brings removed agents back.
*Fix:* send a version of the team (for example a hash of its agents as loaded) with the PUT. The server refuses a mismatch with 409 and its own words. The panel shows them in the summary with `[server]` copy, as it does for "policy.toml changed".

**m8. The template routes are missing from the decisions.** Decision 5 says where user templates appear but not what the UI calls. The engineer needs: a list that marks yours apart from built-in, with saved date and source team (or an extended `GET /api/teams/templates`); create from the team as last saved; delete; and confirmation that `Add this team` on one of yours goes through `POST /api/teams` like a built-in, including the missing-profile 409.
*Fix:* add these to decision 5. They are the shape the UI needs, not a storage format, so they stay within the spec.

**m9. The discard question's count is ambiguous.** copy.md "Its second line" gives the order `the team renamed`, `{n} agents changed`, `{n} added`, `{n} removed`, but its own example and the drawing (states-unsaved-and-replace A) say `3 agents changed and the team renamed`. Those three are one edited, one removed and one new, so "changed" there counts all three.
*Fix:* settle on one: either `The team renamed, 1 agent changed, 1 added and 1 removed. They aren't saved.`, or a total, `The team renamed and 3 agents changed.`, the same way the header count `3 changed` totals them. Then match the example and the drawing.

**m10. Decision 10's scope is still open, and the drawings depend on it.** The wireframes draw `Banner` titles and `Button` danger in the ink tokens.
*Fix:* tech-lead to put the two-line change in task-178 (my recommendation: it is small, and without it the summary title is 3.2:1) or split it out. If split out, the panel uses danger-ink locally, as the decision says.

### Nit

- **n1.** The closed template select still shows `Writing · Built in` in six drawings (new-team-desktop, states-validation, states-unsaved-and-replace, phone-390, dark-new-team, and the add-team-list note). The coordinator decided to drop it, so redraw with the name alone.
- **n2.** `No changes to save.` beside the disabled Save changes is decided but not drawn. Add it to edit-team-desktop or the states, and to the deck's Footer table.
- **n3.** The edit-team-desktop inset B caption still reads "SAVE AS MY TEMPLATE".
- **n4.** states-missing-profile: Ideas is "Visionaries copied", but its count reads `3 agents` and three rows show. Visionaries has five agents (visionary-lead, product-strategist, user-advocate, technologist, skeptic). Show five (and `5 agents use read-mostly`), or say two were removed.
- **n5.** states-missing-profile note 4 and states-saving-error note 3 point to a "Banner text" section in custom-teams.md that doesn't exist. Point them to decision 10.
- **n6.** The new agent is `illustrator` in edit-team-desktop and `critic` in states-unsaved-and-replace, for the same Docs edit.
- **n7.** `Undo` on the removed stub is a bordered button in edit-team-desktop and an underlined link in states-empty and states-validation. Use the button everywhere: it is a button, and it gets a clear target.
- **n8.** dark-new-team shows `edited` on `writer` in a New team panel where light new-team-desktop doesn't, and it drops the Model and Policy profile hints. Make the two match.
- **n9.** `Add this team` / `Add another` repeat on every card with no unique accessible name (today's behaviour). Give them `Add this team, {template}` as `Change it first` already has.

## Decisions for the engineer: are they buildable?

Yes, with the gaps above. The side panel (1), accordion rows with one native radio group (2), one checked request that lands whole (3), renames following agents with the template id kept (4), and in-place questions with focus on the safe answer (6, 7) are clear and consistent with the drawings. What to add: Back and URL behaviour, loading and not-found (M2); the phone footer rule (M1); the scroll container (m5); read-only while saving (m6); a team version in the PUT (m7); the template routes (m8). Decision 4's assumption, that an open task moves to the new coordinator, still needs tech-lead's answer before task-178's server work.

## Blockers and majors

- **M1** Phone and 400% zoom: the stacked sticky footer leaves no room for the content (1.4.10, 2.4.11).
- **M2** Back, loading and team-not-found are undefined for the URL-driven panel. Back can lose typed work.

First pass result: changes requested

## Round 2

I re-checked the rework in a9e4b280..8aaf8b05 against [the spec](spec.md), [the design note](../custom-teams.md), [the copy deck](copy.md) and all twelve wireframes, including the new [opening from a link](states-url.svg). I rendered each one to PNG and read the tall ones in halves. I grepped the SVGs and the note for the strings the deck replaced and for the look rules: corners are `rx` 2 to 4 only, and there is no `filter`, gradient or shadow anywhere. I don't reopen what is settled: the writer's three questions, the app-wide primary-button contrast, and decision 4's open-task assumption.

### Round-1 findings

| | Finding | Status | Where it was checked |
|---|---|---|---|
| M1 | Phone and 400% zoom: sticky footer covers the content | **Fixed** | phone-390 frame 3: one 44px row with Cancel and Save changes, and Save as a template and Remove team (with `Save your changes first.`) under the h3 `This team` after `+ Add agent`. Frame 5 draws 320 × 256 with nothing sticky and the footer in the flow. Decision 12 sets the rule as `max-height: 500px`, a media query and no keyboard guess. Header and footer at 320 × 256 no longer take the viewport (1.4.10), and focus can't land under a sticky bar (2.4.11). |
| M2 | Back, loading, failed load, not found | **Fixed** | states-url A–F and decision 11. Opening pushes one entry. Back with changes puts it back and asks Close's question, with focus on Keep editing. Every other way of closing goes back over the entry, and a cold open replaces `?team=` instead. Edit loading shows no fields, so nothing typed can be lost. New lets you type while the select says `Loading templates…`. A failed load has `Try again` and only Cancel. Not found is its own titled state, and the title takes focus. Each has its words in the deck. |
| m1 | Old strings in the note and SVG notes (2.5.3) | **Fixed** | No `{name} coordinates`, `Save as my template`, `fix 2 things`, `Open on its own page`, `Machines and knowledge` or ` · Built in` is left in the note or any SVG. The radio's name is `Coordinator, {agent}` everywhere. |
| m2 | Edit's open row in the pass 1 layout | **Fixed** | edit-team-desktop `drafter`: Name with the rename hint, Description, the prompt across the row with `7 lines · changed` and the rename note, then the two-column settings. |
| m3 | Heading actions drawn two ways | **Fixed** | edit-team-desktop inset A and states-empty B are both quiet 12px text buttons with 28px hit areas, and Edit team comes first. |
| m4 | Blank's `coordinator` collides | **Fixed** | Flow 1 step 2, states-empty note 3 and the deck: `coordinator-{n}` if taken. |
| m5 | Scroll offsets set twice | **Fixed** | Decision 13 is one rule: the body is the scroller, with no `scroll-margin` or `scroll-padding`. The validation and phone notes now say the same. |
| m6 | Editing during the save | **Fixed** | Decision 8 and states-saving-error A, note 2. Inputs are `readonly` and the other controls `aria-disabled`, with the rejected option argued. |
| m7 | Team changed elsewhere | **Fixed** | Decision 3 adds the team's version to the request. states-url E shows the 409 with `Reload the team` (it asks first) and `Keep my changes here`. One new minor follows from it (r1). |
| m8 | Template routes | **Fixed** | Decision 5 lists list, create (from the team as last saved, with a 409 on a taken name) and delete. `Add this team` goes through `POST /api/teams` with the missing-profile 409. |
| m9 | Discard count ambiguous | **Fixed** | The deck, flow 2, states-unsaved-and-replace A and states-url F all say `The team renamed, 1 agent changed, 1 added and 1 removed.`, and each agent counts once. |
| m10 | Decision 10's scope | **Fixed** (designer's side) | Decision 10 now puts the `Banner` title and `Button` danger ink change in task-178. Tech-lead should keep it in that task's scope. |
| n1 | ` · Built in` on the closed select | Fixed | |
| n2 | `No changes to save.` not drawn | Fixed | edit-team-desktop inset D, plus the deck's Footer table |
| n3 | Inset B caption | Fixed | |
| n4 | Visionaries' five agents | Fixed | states-missing-profile A: five rows, `5 agents use read-mostly` |
| n5 | "Banner text" pointer | Fixed | Points to decision 10 |
| n6 | `illustrator` / `critic` | Fixed | `critic` in both (`illustrator` is left only as a chip on the Handbook template card, which is right) |
| n7 | Undo as a link | Fixed | A bordered button in every drawing |
| n8 | Dark differs from light | Fixed | No `edited`, and the Model and Policy profile hints are there |
| n9 | `Add this team` names | **Partly fixed** | The design note asks for `Add this team, {template}` and `Add another, {template}` ("For the writer"), but copy.md's Add a team table still has no accessible name for them (r3) |

### New findings

No blockers or majors. All of these are **for the build**: none needs another design round.

**r1. Minor: "Keep my changes here" leads to a save that can never succeed.** states-url E, note 5, and copy.md "Changed somewhere else". Once the user keeps their changes, every save is refused until they reload, and reloading drops the changes. The only way out is to copy the work by hand ("Copy anything you want to keep"), and the box doesn't say *what* changed elsewhere, so the user has to remember their own edits and guess what changed on the other side (recognition rather than recall). This is rare for one person, but it costs the most when it does happen.
*Fix for the build:* have the 409 name the agents that changed elsewhere (`writer and critic changed somewhere else.`), and the box list them, so the user knows what to compare. Keep the button as it is, but its words in the deck should say the next save will be refused, for example under the buttons: `Saving will still be refused until you reload.`

**r2. Minor: the phone footer has no place for the problem count.** phone-390 frame 3 (a failed save) draws the one-row footer with no `2 things to fix, listed at the top.`, while copy.md's Footer table and states-validation note 6 put the count in the footer. Decision 12 doesn't say where it goes under 640px.
*Fix for the build:* say in decision 12 that under 640px the count line is left out (the summary has focus and is at the top of the body), or that it sits above the buttons only while there are problems. I'd leave it out, because a second footer line brings back M1's height problem.

**r3. Nit: the copy deck lags the design note's "For the writer" list.** copy.md's Add a team table (`Card button 1`) has no accessible names `Add this team, {template}` / `Add another, {template}`. The row `Toast, saved with later edits` is still there, although decision 8 removes the case. "Questions for the designer" still lists the three questions the coordinator settled, under `Answered: None yet.`
*Fix for the build:* the writer makes the two changes and moves the three questions to Answered with the coordinator's answers. The engineer should follow the note on these until the deck is updated.

**r4. Nit: a stated ratio is slightly high.** states-empty note 4 says the placeholder is ink-muted "4.8:1 on surface-1". The box is `#fafafa` (surface-1), where light ink-muted is 4.63:1. 4.83 is on surface-0. It still passes. Correct the number.

### Checked and fine

- **Coverage.** Every flow and state the spec lists is now drawn, including loading and not found. Loading for New keeps Team name and Start from usable, and Create team is off only then, with `Loading…` as the reason.
- **Consistency.** Strings in states-url match copy.md's "Opening the panel from a link" word for word. Decision 6 now covers the not-found title's focus, and decision 6's safe-first rule covers `Reload and lose your changes?`.
- **WCAG 2.2 AA.** Focus is drawn on the safe button in every new question (Keep editing in E and F, Try again in C). The phone's two footer buttons are 44px. The new banners use danger-ink on danger-soft (5.9:1). The loading lines are polite live regions. Nothing new is told by colour alone. Reflow at 320 × 256 now holds.
- **Look rules.** Flat, square (`rx` ≤ 4), no shadows, glows, gradients or accent stripes. Still one page plus the panel, and the address bar in F is only a drawing device.
- **Decisions for the engineer** are buildable as written. Decisions 11 to 13 are concrete: the history entry, `max-height: 500px`, and the body as the scroller. Decision 3's version can come from the event log the server already keeps.

### Blockers and majors

None. M1 and M2 are fixed, and the round-1 minors and nits are fixed apart from n9, which is partly fixed. The four new findings (r1 to r4) are for the build.

Verdict: approved

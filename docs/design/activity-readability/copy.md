# Readable Activity and Handoffs: copy deck

The final words for [Direction A, Author bands](flows.md), for [the spec](../../../spec.md). Every string in [flows.md › Strings](flows.md#strings) is here, grouped by screen and state, with where it shows, the final text, its accessible name when that differs, plurals and a length limit. Wireframes: [desktop](activity-desktop.svg), [phone](activity-mobile.svg), [Handoffs](handoffs.svg), [states](states.svg), [dark](activity-dark.svg).

**Who reads it.** One person running a board of AI agents on their own work, usually catching up in the morning after a night of turns. They scan for *who* did *what*, then read the few entries that matter. So: the author's name first, plain verbs, short facts, nothing to decode.

**Voice.** Plain, short, sentence case, no exclamation marks, no blame. The board already says "you" for the person and lowercase agent names (`engineer`, `reviewer`); we keep both. Calm in errors (say what happened, then the one thing to do). Quiet in success: a fact, not a cheer.

**Conventions in the tables**

- `{name}` is a value filled in at runtime. Text in a code span is exactly what shows.
- *Changed* means different from the placeholder in flows.md, with the reason in Notes. *Today* means the string already ships and stays as it is.
- Badges are written lowercase; the `Badge` atom shows them in capitals (`RUNNING`). Screen readers read the lowercase text.
- "Max" is characters, at the 390px layout unless it says desktop. Where nothing is said, the string fits both.
- "sr-only" means visually hidden text that screen readers read (Tailwind `sr-only`).

## Glossary

The words the board already uses, kept the same here. Use these, and only these, for these things.

| Term | Means | Use | Don't use |
|---|---|---|---|
| Activity | the tab: everything that happened on a task | `Activity`, "the activity of turn-6" | feed, log, timeline (in the UI) |
| Handoffs | the tab: who asked whom to do what | `Handoffs`, "a handoff" | transfers, messages |
| turn | one go of one agent at the task, with an id like `turn-6` | `turn-6`, "this turn", "every turn" | run, session, job |
| agent | an AI worker on the board, by its name (`engineer`) | the name, lowercase as the agent is named | bot, AI, worker (on the board a worker is a machine) |
| you | the person running the board | `you`, lowercase, as everywhere else on the board | the user, human, me, admin |
| Agentry | the board itself, for what it does on its own (worktree ready, held, files fetched) | `Agentry` | system, the loop, worker, internal |
| team | the group an agent works in; no team is `Software` | `Software`, `Design` | squad, group |
| coordinator / reviewer / deployer | the role words on a band (see [Roles](#roles)) | lowercase, after the team | lead, QA, ops |
| brief | what an agent is told to do: a message to an agent | "Read its brief" (today) | instructions, prompt |
| report | what an agent says when its turn ends: the text under `Finished` | `Finished` row; "the report" in prose | summary, output |
| action | something an agent ran or did with a tool (`Bash`, `Edit`) | `Actions` filter, `{n} actions` | tool call, command (except for Bash itself) |
| row | one line or entry in Activity | `{n} rows` (today) | event, item, entry (in the UI) |
| tokens | what a turn used, as `41k tokens` | `{n} tokens` (today) | usage, cost |
| Workspace | the task's files and commits view | `Workspace` | repo view, editor |
| commit | a commit of the task's branch, by its short SHA | the SHA, `commit 3f2a9c1` in accessible names | hash, revision |
| shared files | the task's files kept outside git | "the shared files" (today) | attachments, uploads |

Words the team uses that never show: *band*, *section*, *author*, *fold*, *rail*, *sticky*, *read model*, *event*.

## Activity toolbar

| Where | Text | Accessible name / aria | Plural | Max | Notes |
|---|---|---|---|---|---|
| Filter segment 1 | `Everything` | | | | Today |
| Filter segment 2 | `Actions` | | | | Today |
| Filter segment 3 | `Messages` | | | | Today |
| Row count, right | `{n} rows` | | `1 row`, `0 rows` | 10 | Today. Only `ink-faint` → `ink-muted` changes |
| Narrowed chip | `{turn}` `{agent}` | | | 30 | Today: the turn id and the agent's name |
| Narrowed chip × | (icon) | `Show every turn` | | | Today |
| Back to top button, floating | `Newest` | `Back to the newest rows` | | 6 | Visible text today. The name says what it does and contains the visible word, so "click Newest" still works by voice |

## Author bands (turns)

The band is the author header. Order, left to right: chip, name, team · role, turn id, `continued`, `on {worker}`, status, tokens, issue text.

| Where | Text | Accessible name / aria | Plural | Max | Notes |
|---|---|---|---|---|---|
| Chip | first letter of the agent's name, capital: `E` | `aria-hidden` | | 1 | The name is always beside it in text |
| Name | `{agent}`, as named: `engineer` | | | 120px on phone, then ellipsis | |
| Team and role | `{team} · {role}`: `Software · reviewer` | | | 28 | Middle dot with a space each side, as elsewhere on the board |
| Team, no role | `{team}`: `Software` | | | 16 | |
| Team, blank | `Software` | | | | Matches `DEFAULT_TEAM` in lib/teams.ts |
| Role, coordinator | `coordinator` | | | | The agent coordinates a team |
| Role, Review column | `reviewer` | | | | Column `review` |
| Role, Shipping column | `deployer` | | | | Column `ship`; the agent form describes it as "Shipping checked work, such as deploying it" |
| Turn id | `{turn}`: `turn-6` | | | 9 | `ink-faint` → `ink-muted` |
| Continued | `continued` | (part of the band name, below) | | | Muted, after the turn id. A turn's later rows above a `you` band |
| Remote | `on {worker}`: `on studio` | | | 24, desktop | Today. Shown only when the turn ran on another machine |
| Status, running | `running` | | | | Today. `info` badge |
| Status, ended | `ended` | | | | Today |
| Status, stopped | `stopped by you` | | | | Today. `warning` badge |
| Status, issue | `issue` | | | | Today. `danger` badge, then the issue text |
| Issue text | `{issue}`, as raised | | | one line, truncated | Today; full text in the `Something went wrong` row |
| Tokens | `{n} tokens`: `18k tokens`, `950 tokens`, `3.1M tokens` | | always `tokens` | 12 | Today; numbers from `tokens()` |
| Duration in band (phone only) | `{duration}` / `{duration}…` | | | 7 | On phone the rail has no room; same text as the rail |

### Band accessible name

The band is a button. Its name starts with what's shown first, the agent, so it matches what's on screen (WCAG 2.5.3) and so screen-reader users hear the author first. Commas, not `·`, because some screen readers say "dot".

| Case | Accessible name |
|---|---|
| Turn, all of it | `{agent}, {team} team, {role}, {turn}, {status}. Show only this turn` → "reviewer, Software team, turn-6, running. Show only this turn" |
| No role | drop `{role},` → "engineer, Software team, turn-5, ended. Show only this turn" |
| Continued | add `continued` after the turn → "reviewer, Software team, turn-6, continued, running. Show only this turn" |
| Remote | add `on {worker}` after the turn |
| Issue | `issue: {issue text}` as the status |
| Already narrowed to this turn (button disabled) | drop `. Show only this turn` |

*Changed* from "{turn}, {agent}, {team · role}, {status}. Show only this turn": agent first, as it's shown first and it's what the reader is looking for; "team" spelled out so "Software" isn't heard as a word on its own; `·` replaced by commas.

The `<section>` around each author section takes the band as its label (`aria-labelledby`), so the section is announced as, for example, "reviewer, Software team, turn-6, running" without "Show only this turn". Give the label element the facts and the button the extra sentence (sr-only), so `aria-labelledby` picks up just the facts.

### Roles

- `coordinator` when the agent coordinates a team (`Agent.coordinates`).
- else `reviewer` for the Review column, `deployer` for Shipping.
- else no role, just the team.
- **If the role is the agent's own name, leave it out.** An agent named `reviewer` shows `reviewer  Software`, not `reviewer  Software · reviewer`; a coordinator named `coordinator` shows `coordinator  Software`. The same word twice in one line reads as a glitch, and the name already says it. The wireframes show `reviewer · Software · reviewer`: the designer should update them (see [Questions](#questions-for-the-designer)). In the accessible name, drop the role in the same case.

## The you and Agentry bands

| Where | Text | Accessible name / aria | Max | Notes |
|---|---|---|---|---|
| You chip | `Y` | `aria-hidden` | 1 | Neutral chip |
| You band name | `you` | heading text `you` | | Lowercase, as the board says it everywhere ("stopped by you", "waiting for you"). A heading, not a button |
| You section label | | `you` | | `aria-labelledby` the heading |
| Agentry chip | `A` | `aria-hidden` | 1 | Neutral chip |
| Agentry band name | `Agentry` | heading text `Agentry` | | Capitalised: it's the product's name |
| Agentry section label | | `Agentry` | | |
| Another agent's band outside a turn (rare) | `{agent}` `{team} · {role}` | heading: `{agent}, {team} team, {role}` | | Same as a turn band without turn facts; not a button |

No team, role or turn facts on either: there is nothing true to say there.

## Time rail

| Where | Text | Accessible name / aria | Max | Notes |
|---|---|---|---|---|
| Row time | `{hh:mm}`: `06:15` | | 5 | Today, from `clockTime()` |
| Duration, done | `{duration}`: `48m`, `40s`, `1h 12m` | | 7 | Today. `ink-faint` → `ink-muted` |
| Duration, live | `{duration}…`: `18m…` | | 7 | Today. `success` colour; the ellipsis means "still going" |
| Now row time | `now` | | 3 | Today |

The rail stays plain text for screen readers; the time and duration are read after each row's content, which is the order they're in. No extra wording.

## Rows

### Messages

| Where | Text | Accessible name / aria | Max | Notes |
|---|---|---|---|---|
| From | `{from}`: `coordinator`, `you` | | 120px phone | Today |
| Arrow | → (icon) | sr-only ` to ` | | *Added.* The arrow is `aria-hidden`, so today a screen reader hears "coordinator engineer". It should hear "coordinator to engineer" |
| To | `{to}`: `engineer`, `engineer on studio` | | 120px phone | Today |
| Leads to | `{turn}`: `turn-6` | `Show only {turn}` | 9 | Today's button; the name now says what it does |
| Body | the message, as compact Markdown | | folded at ~10 lines | |

### Actions

| Where | Text | Accessible name / aria | Max | Notes |
|---|---|---|---|---|
| Tool | `{tool}`, as logged: `Bash`, `Read`, `Edit` | | | Today |
| Command | the command, in a code box | none: the tool name just above says what it is | folded at 3 lines | |
| Badge, approved and finished | `done` | | | Today |
| Badge, approved and going | `running` | | | Today |
| Badge, denied | `denied` | | | Today |
| Badge, asked, not answered | `waiting for you` | | | Today |
| Badge, being judged | `deciding` | | | Today. *Added to the list*: it's missing from flows' Strings but shows when the policy hasn't decided yet |
| Reason | `{reason}`, as logged | | 2 lines desktop | Today. `ink-faint` → `ink-muted`: it's text people read |

`approved` from `ActionBadge` never shows in Activity (approved actions show `done` or `running`); no change.

### Reports, waiting, issues

| Where | Text | Max | Notes |
|---|---|---|---|
| Report title | `Finished` | | Today |
| Report title with tokens | `Finished, {n} tokens`: `Finished, 41k tokens` | 24 | Today |
| Waiting title | `Waiting` | | Today. The question follows as the body |
| Issue title | `Something went wrong` | | Today. The reason follows as the body |

### Notes

Kept exactly as `lib/activity.ts` writes them today (`Read its brief`, `Worktree ready on {worker}: {branch}`, `Held in {place}`, `Moved on`, `Copied {from} into the shared files`, …). They're UI strings, but the scope says no change to what is logged, and they already read well. One line, truncated with an ellipsis; a note with a body folds at 4 lines.

flows.md's `Fetched {n} shared files` doesn't exist in the code: today it's `Fetching {machine}:{path} into the shared files` and `Copied {from} into the shared files`, one row per file. Use those; the wireframes' `Fetched 2 shared files` is just a sketch.

## Fold buttons

| Where | Text | Accessible name / aria | Plural | Max | Notes |
|---|---|---|---|---|---|
| Under a folded entry | `Show all · {n} lines`: `Show all · 38 lines` | `Show all {n} lines of this {kind}`, `aria-expanded="false"`, `aria-controls` | always ≥4, so always `lines` | 22 | Placeholder kept. `Show all` matches `Clamp` today |
| Under an opened entry | `Show less` | `Show less of this {kind}`, `aria-expanded="true"` | | 9 | Today (`Clamp`) |

`{kind}` is what the entry is, so a list of buttons makes sense out of context (a screen reader's buttons list would otherwise read "Show all 38 lines" ten times): `brief` (a message to an agent), `message` (to or from you), `report`, `question`, `issue`, `note`, `command`, `code`. Handoff cards use `brief`.

The count can't be 1: commands fold at 3 lines and everything else later, so only the plural is needed. If the build ever shows `1`, write `Show all · 1 line`.

**What a screen reader hears.**

- Opening: nothing extra. Focus stays on the button; its name changes to "Show less of this report" and `aria-expanded` turns true, which screen readers announce ("expanded"). Don't add a live region: it would say the same thing twice.
- Closing: the same, "Show all 38 lines of this report, collapsed". When the feed scrolls back to the entry's top, focus stays on the button, so nothing needs announcing.

## Code blocks

| Where | Text | Accessible name / aria | Max | Notes |
|---|---|---|---|---|
| Language label, top right | `{language}` as written after the fence, lowercase: `bash`, `ts`, `go` | | 10, then truncate | A fence with no language shows nothing |
| Copy button | (icon) | `Copy code` | | *Changed* from `Copy`: verb and object. A tooltip shows the same |
| After copying | (check icon, 1.5s) | `Copied`, also in a polite live region | | Today's word (`CodeFence`, `CommandLine`) |
| Copy fails (plain http, no clipboard) | (nothing changes) | | | As `CodeFence` today: the text can still be selected |
| Monaco fails | (nothing: plain mono stays) | | | No message, by design |

Commands have no Copy button, by design.

## Links

| Kind | Shows | Accessible name | Notes |
|---|---|---|---|
| External (`https://`, `http://`, `mailto:`) | its label, underlined, then ↗ | `{label} (opens in a new tab)` | The ↗ icon is `aria-hidden`; the words are sr-only text after the label |
| Repo file | its label (mono when it's a path), ↗ | `{label} (opens in a new tab)` | Opens the Workspace |
| Task id the board knows | `task-21`, mono, no ↗ | `task-21` | Same tab |
| Commit of the task | `3f2a9c1`, mono, ↗ | `commit 3f2a9c1 (opens in a new tab)` | *Added* `commit`: a bare hash says nothing out of context, and screen readers read it letter by letter |
| Any other SHA, unknown task id | plain code | | Not a link |
| `javascript:`, `data:` and other unsafe links | the label as plain text | | No hint, no warning: it reads as a sentence |

"(opens in a new tab)" is the placeholder kept: it's the common wording, and it says what happens, not where the link goes. Bare URLs show as written; a very long one wraps.

## The now row

| Where | Text | Max | Notes |
|---|---|---|---|
| A turn just started, no rows yet | `{agent} is starting…` | 32 | From flows; matches the `{agent} is working` pattern |
| An agent at work | `{agent} is working` | 32 | Today (TaskPage) |
| Another team has the task | `Waiting on {team}, from another team` | 40 | Today (TaskPage); unchanged, listed so it isn't lost |

The now row is text, not live-announced: an announcement on every turn start would interrupt the person while they read.

## Handoffs

### Top line

| Where | Text | Accessible name | Max | Notes |
|---|---|---|---|---|
| While a turn runs | `{agent} is working now · watch the terminal` | `{agent} is working now. Watch the terminal` | wraps on phone | Today. The name keeps the visible words; the full stop makes the pause screen readers otherwise skip |

### Card

| Where | Text | Accessible name / aria | Plural | Max | Notes |
|---|---|---|---|---|---|
| From | chip + `{from}` | chip `aria-hidden` | | 120px phone | |
| Arrow | → | sr-only ` to ` | | | *Added*, as in Activity |
| To | chip + `{to}` | chip `aria-hidden` | | 120px phone | |
| Time | `{hh:mm}` | | | 5 | `ink-faint` → `ink-muted` |
| Title | the brief's first heading, as written | the card's heading (`h3`) | | 2 lines | No heading, no title: the body starts at once |
| Body | the brief, compact Markdown | | | folded at 8 lines | |
| Card label | | `<article aria-label="{from} to {to}, {hh:mm}">` | | | So a screen reader's list of articles names each one |

### Card footer

The footer is the button that opens the turn.

| Where | Text | Notes |
|---|---|---|
| A step that led to a turn | `Led to` | Today |
| A step said from a turn | `From` | Today |
| Turn | `{turn}` | Today |
| Agent | `{agent}` | Today, after the turn id |
| Status | badge: `running`, `issue` | Today (no badge once ended) |
| Duration | `{duration}`, only once ended | Today |
| Actions | `{n} actions` | `1 action`, `0 actions`. Today |
| Tokens | `{n} tokens` | Today, only when above 0 |
| End | `Activity ›` | Today. ›, not >, is the icon, `aria-hidden` |

Phone: `Led to {turn} · Activity ›` (no agent, duration, counts or tokens; the design drops them). The facts are joined by ` · ` in the footer as drawn.

**Footer accessible name**: `Open the activity of {turn}, {agent}` → "Open the activity of turn-6, reviewer". *Changed* from "Open {turn}'s activity": `turn-6's` reads badly aloud, the board already says "Open the terminal of {task}", and the agent says who did the work. Add `, running` or `, issue` when the badge shows, so the name carries what the badge says.

A step with no turn has no footer; nothing to write.

## States

### Activity

| State | Where | Text | Accessible name / aria | Notes |
|---|---|---|---|---|
| Loading | sr-only, with `aria-busy="true"` on the feed | `Loading activity…` | | From flows. The grey blocks are `aria-hidden` |
| Error | message title | `Couldn't load the activity` | `role="alert"` on the message | *Changed*: no full stop, as every other `Couldn't load the …` banner on the board |
| Error | detail, muted mono | `{error}`, as the server said it: `GET /api/tasks/task-40: 502 Bad Gateway` | | Never alone: the title says it in words |
| Error | button | `Try again` | | *Changed* from `Retry`: plain words a non-native reader knows, and what the button does |
| Retrying | sr-only | `Loading activity…` again, then the feed | | The busy state says it; no extra announcement. If it fails again, the alert re-announces `Couldn't load the activity` |
| Empty, a new task | `EmptyState` title | `Nothing here yet` | | Today |
| Empty, a new task | `EmptyState` body | `Every message, turn and action shows here as it happens.` | | *Changed*: `tool call` → `action`, the toolbar's own word |
| Empty after a filter (Actions or Messages on, nothing to show) | `EmptyState` title | `No actions yet` / `No messages yet` | | *Added*: today it says `Nothing here yet`, which isn't true when Everything has rows. Same component, no design change |
| Empty after a filter | `EmptyState` body | `Choose Everything to see every row.` | | Names the control by its label |
| Running turn, no rows | band + now row | `{agent} is starting…` | | See [The now row](#the-now-row) |
| Monaco fails | | (nothing) | | By design |
| Message with no text | | the `from → to` line alone | | No filler |
| Report with no text | | `Finished` or `Finished, {n} tokens` alone | | |
| Question with no text | | `Waiting` alone | | |
| Commits not loaded | | (nothing: SHAs stay plain) | | |

### Handoffs

| State | Where | Text | Notes |
|---|---|---|---|
| Loading | sr-only, `aria-busy` | `Loading handoffs…` | |
| Error | title, `role="alert"` | `Couldn't load the handoffs` | No full stop, as above |
| Error | detail | `{error}` | |
| Error | button | `Try again` | As Activity |
| Empty | `EmptyState` title | `No handoffs yet` | Today |
| Empty | `EmptyState` body | `When the coordinator hands the task to an agent, the brief shows here.` | *Changed*: today's `The coordinator reads the task first.` says what happens next but not what this tab will hold; flows' `…, it shows here.` leaves "it" unclear. Saying "the brief" names what appears |
| Card with no brief | | header and footer only | No filler |

## Phone (390px)

Checked against [activity-mobile.svg](activity-mobile.svg) and the phone half of [handoffs.svg](handoffs.svg), at the sizes in flows.md (mono 12.5px names ≈ 7.5px a character, 12px text ≈ 6.5px, 10px capital badges ≈ 7px).

| String | Fits? | What changes on the phone |
|---|---|---|
| Stuck band: chip, name, status, live duration | yes: `product-designer` + `STOPPED BY YOU` + `1h 12m…` ≈ 300px of 342 | Team, role, turn id and tokens go (design). Names past 120px truncate |
| Unstuck band second line: `Software · reviewer · turn-6` | yes, 28 characters | The design puts `{team} · {role}` and the turn id here; `continued` goes after the turn id and still fits (38) |
| `on {worker}` | not on the band | Hidden on the phone, with the turn id (the accessible name keeps it) |
| Toolbar: `Everything` `Actions` `Messages` `{n} rows` | the control scrolls sideways, as today | none |
| `Show all · 38 lines` | yes, beside the badge | none |
| Code language label | yes, 10 characters max | truncate past 10 |
| Handoffs top line | wraps to two lines for long names | none: wrapping is fine here; the whole line is the button |
| Handoffs card header | `from → to` on one line, time under it (design) | names truncate at 120px |
| Handoffs footer | | **Shortened**: `Led to {turn} · Activity ›`, the design's own cut |
| Error, empty and loading texts | yes, they wrap in the centred `EmptyState` | none |
| Note rows | one line, truncated | none (as today) |

Shortened for the phone: only the Handoffs footer (and the band, by the design's hiding of team, role, turn id, tokens and `on {worker}`). No string has a separate phone wording.

## Questions for the designer

1. **Role repeating the name.** The wireframes show `reviewer  Software · reviewer` and `coordinator  Software · coordinator`. I've written a rule to drop the role when it equals the agent's name. On the demo board almost every agent is named after its role, so most bands would show only the team. Fine with you, or would you rather show the role always for a steady column?
2. **Task-id links' titles.** `task-21` alone doesn't say what the task is. A `title` (tooltip) with the task's title, and the same in the accessible name ("task-21, Fix the chat links"), would help the person catching up decide whether to follow it. The board has the titles in the live task list. Is a tooltip in scope?
3. **The error's next step.** `Try again` covers a blip. When the person's own Agentry server has stopped, trying again won't help. Room for one muted line, "If it keeps failing, check that Agentry is still running."? I've left it out, since it's a new line in the design.
4. **The you band on Handoffs.** Messages from you show `you` as the from name with a `Y` chip; there's no band there. Just checking the card header is meant to say `you`, lowercase, as Activity does.

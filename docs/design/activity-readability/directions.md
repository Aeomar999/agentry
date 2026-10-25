# Readable Activity and Handoffs: three directions

For [the spec](brief.md). Three ways to read a task's Activity and Handoffs tabs, which differ most on how the author stays obvious and on density. The parts that barely differ (Markdown, links, code) are set out once, then each direction says what it does with them. The recommendation is at the end.

Wireframes (light theme, the page's own tokens; regenerate with `cd docs/design/source && python3 -B activity_directions.py ../activity-readability`):

- [Direction A · Author bands](direction-a.svg)
- [Direction B · Conversation](direction-b.svg)
- [Direction C · Ledger](direction-c.svg)

## What the code already gives us

- **Every row in a turn is that turn's agent's.** `activityOf` puts an action, a report, a note under the turn it happened in, and `newestFirst` keeps each turn's band above its rows. The band is already `sticky top-0`. So the author of most rows is already known and already on screen; it just isn't said clearly (a plain name, no chip, no team).
- Rows outside a turn are yours (task created, directed, answered, budget set), or Agentry's own (worktree ready, held in a column, files fetched).
- Messages carry `from` and `to`. Handoffs (`Step`) carry `from`, `to`, `text`, `turn`.
- **A commit view exists.** The Workspace opens a commit of the task's branch with `/tasks/:id/workspace?commit=<hash>` (`WorkspacePage`, `SourceControl`), and `/api/tasks/:id/commits` lists those commits. So a SHA can link there, but only a SHA that is one of the branch's commits; any other hex stays text.
- `Avatar` (letter on an 18% tint of the agent colour) and `linkInto` (web out, task files into the Workspace preview) live inside `CoordinatorChat.tsx`. They need to move to molecules to be shared; the behaviour stays.
- Role: there is no role field. What data gives: `Agent.team` ("" is Software), `Agent.coordinates` (say "coordinator"), `Agent.column` (`review` says "reviews", `ship` says "ships"). Most agents are named for their role (engineer, reviewer), so name plus team is enough; we show the role word only where data gives it.
- Trap: `--color-ink-faint` in the light theme (#9aa0ac on white) is about 2.6:1, under AA for text. New author headers use `ink-muted` (#6b7280, 4.8:1) for team, time and counts. Today's rail durations, the handoff times and the turn ids use ink-faint; we should lift them too while we're there.

## Shared by every direction

**Markdown (question 1).** Briefs, reports, waiting questions, issues and notes with a body render with the existing `Markdown` in compact mode (13px, headings at 14/13px semibold, lists, tables, task lists, fenced code). Long ones are cut by measured height, as the chat's `Long` does, not by characters: at about 10 lines (A, B) or one line (C, compact), on a clean edge with no fade (fades are gradients). Under the cut, a text button "Show all · 38 lines" with `aria-expanded`; open, it reads "Show less". The line count tells you what you're opening. Tables scroll sideways inside their own box at narrow widths.

**Links (question 2).** One `linkInto` for the chat, Activity and Handoffs, extended:

| In the text | Looks like | Goes to |
|---|---|---|
| `https://…`, `mailto:` | underlined, info colour, a small ↗ after it | new tab, `rel="noopener noreferrer"` |
| `[the design](docs/design.md)`, a file of the task | underlined, mono when the label is a path | the task's Workspace preview, new tab (as the chat does) |
| `task-21`, bare or linked | underlined mono | `/tasks/task-21`, same tab (it's the same app) |
| a SHA of the task's branch, 7 to 40 hex | underlined mono | `/tasks/:id/workspace?commit=<hash>`, new tab |
| anything else: `javascript:`, `data:`, `vbscript:`, unknown schemes | plain text, no link | nowhere |

Underline is always on, so a link isn't told by colour alone. `Markdown` autolinks only bare `https://` today, so bare `task-21` and SHAs need a small addition to its inline pass, still building elements, never HTML. Bare file paths in prose are not linked: the spec asks for paths in links only, and guessing paths in prose would link things that aren't files.

**Code (question 3).** A bash action shows its tool in small mono ("Bash"), then the command in a flat box (surface-1, a hairline, 3px corners). Fenced blocks use the same box with the language at the top right. Colour comes from `MonacoColorize`, loaded only when the box first scrolls into view (an IntersectionObserver around a `lazy()` import); until then it's the same text in plain mono, so nothing shifts when colour lands. Long commands wrap at spaces and operators (`pre-wrap`, breaking anywhere only for a word longer than the line), show 3 lines, then "Show all · 9 lines". Fenced blocks show 12 lines, then the same. A command's reason and badge stay to its right on a wide screen and drop under it on a phone.

**Look.** Flat surfaces, hairlines, square corners (3px), no shadows, glows, gradients or coloured left edges. The agent's colour appears only in the chip's letter and tint and in existing dots; the name is always in text beside it. Dark theme: the same tokens; the chip tint rises to about 22% so it reads on surface-0.

## Direction A · Author bands

[Wireframe](direction-a.svg)

**Idea.** Make the turn band, which already sticks, the author header. The feed becomes sections, one per author: a band per turn (that agent), and a band for each run of rows outside a turn (you, or Agentry). Nothing inside a section repeats the author, because the band is always on screen while its rows are. A message inside a section still says who it's from and to, with chips and an arrow.

**Sketch.** Band: chip, **name** in mono semibold, team and role in muted, turn id, status badge, tokens; the time rail stays on the right. Rows under it as today, with Markdown and highlighted code. Handoffs keep their cards; the header gets chips and an arrow ("[C] coordinator → [E] engineer"), the brief's first heading becomes the card's title, and the brief is compact Markdown cut at about 8 lines.

1. Markdown: compact, about 10 lines, then "Show all · 38 lines".
2. Links: as above.
3. Code: as above; the rail keeps time and duration for each command.
4. Author: the sticky band (chip, name, team, role, status) heads every row; messages add from → to.

**Density.** Close to today's: the same rows, plus about 8px per band. On the demo feed at 1280×800 about 14 rows are on screen.

**Against the measures.** Attribution: every visible row is under a band that's on screen (sticky), and rows outside turns have bands too, so 100%, at both widths. Reading, links, speed: the shared parts.

**Trade-offs.** + The smallest change: the band, the rail and the row kinds stay; mostly new rendering inside rows. + Nothing is said twice. + Turn and author are one idea, which is how Agentry works. − A long run of your own rows between turns is one "you" band, fine; but a turn whose agent writes a message to another agent shows both names, and the band's agent must not be mistaken for the message's receiver: the arrow and the "to" chip carry that. − No new way to narrow to one agent (clicking a band already narrows to that turn).

## Direction B · Conversation

[Wireframe](direction-b.svg)

**Idea.** Activity reads like the coordinator chat: a 28px avatar and a byline (name, team, role, time) once per run of one author's entries; entries are text, not rows. Turns become quiet dividers ("turn-4 · engineer started · 05:21"). Notes fold into one muted line under the author. A long entry's byline sticks to the top while you read it. Filter chips by agent sit in the toolbar.

**Sketch.** No time rail: time moves into the byline, running and durations into the dividers. A handoff's byline names both: "coordinator → [E] engineer". Handoffs use the same byline-and-text look, so the chat, Activity and Handoffs read alike.

1. Markdown: compact, about 10 lines, then "Show all".
2. Links: as above.
3. Code: as above, under the author's byline.
4. Author: avatar and byline per run, sticky on long entries; filter chips by agent (name in text and a dot; several can be on).

**Density.** The roomiest: about 30% fewer rows on screen than today (about 9 at 1280×800).

**Against the measures.** Attribution: met while a byline sticks, but between a run's byline and its last entry a short entry low on the screen relies on the byline above it having stuck, and two runs' sticky bylines must hand over cleanly; harder to guarantee 100% than A. Reading, links: shared. Speed: shared.

**Trade-offs.** + One look across chat, Activity and Handoffs. + Filters answer "what did the reviewer do?". − Drops the time rail, which the user can read at a glance today. − Sticky bylines nested inside sticky turns are two layers of sticky, fiddly to get right and to keep accessible. − Fewer rows per screen on a feed of hundreds.

## Direction C · Ledger

[Wireframe](direction-c.svg)

**Idea.** A dense table: an author column (chip and name) on every single row, the content in one line, the time at the right. A Compact / Detailed switch: Compact shows one line per row and opens a row in place with ▸; Detailed opens every row. An agent filter menu (a menu, since teams reach ten agents). Handoffs become a table too (from, to; brief title; when), one row opened at a time.

1. Markdown: Compact shows the first line or heading; opened, the whole Markdown with "Show all" past about 10 lines.
2. Links: as above.
3. Code: a command shows its first line and "9 lines ▸"; opened, it wraps and is coloured.
4. Author: on every row; opened rows add team and role under the name.

**Density.** The densest: about twice today's rows on screen in Compact (about 28).

**Against the measures.** Attribution: 100% by construction, no sticky needed. Reading: Compact cuts Markdown to one line, so no raw syntax shows, but the reading happens only after a click. Links and speed: shared; Compact means almost no Monaco on screen.

**Trade-offs.** + Easiest to scan for "what happened"; the filter and switch suit very long feeds. − The name is repeated on every row: noise, and a 150px column lost at 390px (it folds to a line above each row, so phone rows double). − Briefs and reports, which the user said they want to read, hide behind a click. − The most new UI (a menu, a switch, expanding rows), and the most to test.

## Recommendation: A, Author bands

A answers the spec's measures with the least new surface, because it builds on what Agentry already has: a turn is one agent's, and the turn's band already sticks. It keeps the time rail and today's density, which matter on a feed of hundreds read the morning after, and it says each author once instead of on every row (C) or behind a second layer of sticky (B). Markdown and code render in place, so briefs and reports read without a click.

From the others I'd keep two small things, both optional for a later story: B's filter by agent, as chips in the toolbar, and C's one-line Compact mode, if feeds grow past what bands make easy. Neither is needed to meet the spec.

## Open questions and assumptions

- Rows outside turns that Agentry itself writes (worktree ready, held in a column, files fetched): I assume an "Agentry" band with a neutral chip, not "you". Yes?
- A message from you while a turn runs has no turn, so it opens its own short "you" band between that turn's rows; I assume that's fine rather than folding it into the turn.
- Role words come only from `coordinates` and `column` ("coordinator", "reviews", "ships"); other agents show name and team only.
- SHAs link only when they're among the task branch's commits; the page needs `/api/tasks/:id/commits` on the task page (a small extra fetch). Fine?
- Lifting rail durations and handoff times from ink-faint to ink-muted is a tiny change on the same page; I assume it's in scope as an accessibility fix.
- Phone (390px): the rail shrinks to time only (about 56px), the band drops the team and keeps chip, name and status; next step is to draw it.

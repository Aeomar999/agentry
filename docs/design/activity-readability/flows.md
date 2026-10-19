# Readable Activity and Handoffs: flows for Direction A, Author bands

For [the spec](../../../spec.md) and the chosen [Direction A](direction-a.svg) in [the directions](directions.md). This is the detail the Software team builds from. Strings are placeholders; the UX writer finalises them from [Strings](#strings).

Wireframes (regenerate with `cd docs/design/source && python3 -B activity_bands.py ../activity-readability`):

- [Activity, desktop](activity-desktop.svg): 1280 wide, a long feed with a stuck band, every row kind, every link kind, coordinator, engineer, reviewer, Agentry and you.
- [Activity, phone](activity-mobile.svg): 390 wide.
- [Handoffs](handoffs.svg): desktop and 390.
- [States](states.svg): empty, loading, error, a running turn with no rows yet, Monaco failing, an entry with no text.
- [Activity, dark](activity-dark.svg): the desktop fragment in the dark theme.

## The rules everything below follows

- **An author section** is a run of rows with one author, under one band. A turn is a section (its agent). Rows outside a turn are grouped by who wrote them: **you**, **Agentry** (the loop's own rows: worktree ready, held, files fetched, moved on) or, rarely, another agent (another team's coordinator answering a request). A new author, or a new turn, starts a new section.
- **The band is the author header.** It names the author in text, always: chip, name, then team and role in muted text, then the turn's facts. Nothing inside a section repeats the author; a message still shows `from → to`, because the receiver is someone else.
- **Colour is a hint, never the message.** The agent's colour appears only in the chip (its letter on a tint) and in the rail's dot. Links are underlined. Status is a word in a badge.
- **Flat.** Surfaces and hairlines; corners 3px (4px at most); no shadows, glows, gradients, fades, and no coloured left edge on anything.

## Tokens

Only existing tokens (ui/src/styles/globals.css), plus one alias for links.

| Use | Token | Light | Dark | Contrast |
|---|---|---|---|---|
| Feed background | `surface-0` | #ffffff | #1a1d23 | |
| Band, code box | `surface-1` | #fafafa | #16181d | |
| Inline `code`, neutral chip, badge default | `surface-3` | #e3e5ea | #2b2f38 | |
| Hairlines (band edges, code box, card) | `divider` | #e3e5ea | #2b2f38 | |
| Badge outline, card hover | `divider-2` | #cdd1d8 | #353a44 | |
| Author name, headings | `ink-primary` | #1f2329 | #e4e6eb | 15:1 / 13:1 |
| Body text | `ink-secondary` | #424854 | #b3b7c0 | 9:1 / 8:1 |
| Team, role, turn id, times, durations, counts, "Show all", handoff times | `ink-muted` | #6b7280 | #8b919e | 4.6:1 on surface-1 / 5.3:1 |
| Nothing that must be read | `ink-faint` | #9aa0ac | #5e6471 | 2.6:1: icons and hairline dots only |
| Links | `link` (new alias): light `color-mix(in srgb, var(--color-info) 85%, var(--color-ink-primary))`, dark `var(--color-info)` | ≈#0675ae | #38bdf8 | 5.0:1 / 7.9:1 |
| Agent chip | `agent-<colour>` on an 18% (light) / 22% (dark) tint of itself; in light the letter is `color-mix(agent 75%, ink-primary)` | | | ≥3.8:1 light, ≥4.3:1 dark, every colour |
| Neutral chip (you, Agentry) | letter `ink-secondary` on `surface-3` | | | 7.3:1 / 6.7:1 |
| Running | `info` badge; live duration `success` | | | |
| Focus | the global `*:focus-visible` outline, `brand` | | | see open questions |

Why the two tweaks: `info` (#0284c7) is 4.1:1 on white, under AA for 13px link text, and the light theme's teal, green and orange agent colours are 2.6 to 2.9:1 as a letter on their own tint. Both are fixed by mixing toward `ink-primary`; the dark theme needs neither.

## Flow 1: scanning a long Activity feed

The user opens a task in the morning and scrolls down Activity (newest at the top) to catch up.

1. **Opens the tab.** The feed starts at the top. The first band (the newest section) sits under the toolbar. If a turn is running, its band shows a `running` badge and a live duration on the rail.
2. **Scrolls down.** The band of the section being read stays stuck at the top of the scroller. Its rows scroll under it. Every row on screen is either under the stuck band or under a band that is itself on screen, so the author of every visible row can be read without scrolling.
3. **Reaches the next section.** The next band scrolls up and **pushes** the stuck one out (each section is its own element and its band is sticky inside it, so the old band leaves with its last row; no overlap, no two bands half-drawn). The new band sticks.
4. **Passes an Agentry section** between two turns: a band with a neutral chip (letter A) and the name **Agentry**, no team, no turn facts; its rows are notes (Worktree ready, Held in Review, Fetched 2 shared files).
5. **Passes a you message in the middle of a running turn.** Newest first, the running turn reads as three sections:
   - the turn's later rows, under a band that repeats the turn's header with a muted **continued** after the turn id (the turn's true band, where it started, is further down);
   - a short **you** band (neutral chip, Y, **you**) with the message `you → coordinator` and its text;
   - the turn's band proper with its earlier rows.

   The rail keeps time for all three. If the you message was sent to the turn's own agent, the arrow names it, so it can't be confused with the band.
6. **Clicks a band.** Unchanged from today: the feed narrows to that turn (the "continued" band narrows to the same turn). The toolbar chip shows the turn and an × to go back.
7. **Goes back up.** The existing "Newest" button appears after 200px of scroll.

Where it can go wrong, and what we do:

- A focused link or button scrolls under the stuck band. The scroller sets `scroll-padding-top: 44px` (the band's height), so keyboard focus and `scrollIntoView` (the "row you came to see") never land under it. WCAG 2.4.11.
- A section is a single row shorter than the band: the band still shows, for one row. Fine: that's the honest author.
- An agent the roster no longer has: the chip uses `ink-faint`'s neutral treatment (as `useAgentColor` falls back today), the name is still in text.
- Filters (Everything, Actions, Messages) can empty a section: a section with no rows left is dropped with its band. A turn band is never dropped in Actions, as today.

**The band.** 44px tall (was 36), `surface-1`, hairline top and bottom. Left to right: chip (20px), **name** (mono 12.5, semibold, `ink-primary`), team and role (12, `ink-muted`: "Software · reviewer"), turn id (mono 11, `ink-muted`, was faint), "continued" when it is one, "on <worker>" when remote, status badge (`running`, `ended`, `stopped by you`, `issue`), tokens (mono 11, `ink-muted`), the issue text if any. The rail column (120px, 104px under `md`) carries the start time and the duration (`ink-muted`, was faint; live in `success` with "…"). The band is a button: its accessible name is "turn-4, engineer, Software · reviewer, running. Show only this turn". You and Agentry bands are not buttons (no turn to narrow to); they are headings.

**Role** (spec decision 6): "coordinator" when `Agent.coordinates` is set; otherwise the column's role: `review` → "reviewer", `ship` → "deployer"; otherwise none. Team "" shows as "Software".

**Phone (390).** The band is 44px: chip, name, status badge; team and role move under the name only when the band is not stuck (when stuck it is one line). Turn id and tokens go. The rail is 56px, time only; the duration moves into the band after the badge. The toolbar segmented control scrolls sideways as it does today.

Component map:

| Piece | Status |
|---|---|
| `ActivityFeed` (organism) | changed: renders sections instead of a flat list; `<ol>` of `<li><section aria-labelledby>`, band sticky inside each section |
| `lib/activity.ts` | changed: each item gets `by` (the author, from `who(e)`; Agentry for the loop's own actor); new `sections(rows)` groups `newestFirst` output into author sections and marks continued turns. No change to what is logged |
| `AuthorBand` (new molecule) | the band: `Avatar`, name, team and role, then children (turn facts) |
| `Avatar` (moved) | from `CoordinatorChat.tsx` to `molecules/Avatar.tsx`; adds `size` (20, 28), the light-theme letter mix, and the neutral look for you and Agentry. The chat imports it from there, behaviour unchanged |
| `AgentName`, `useAgentColor`, `Badge`, `Mono`, `SegmentedControl` | reused as they are |
| `roleOf(agent)` (new, lib) | coordinator / reviewer / deployer / none, team "" → Software |

## Flow 2: reading a long brief or report

The user wants to know what the coordinator told the engineer, or what the engineer reported.

1. **Sees the first lines.** Messages (briefs), reports (Finished), waiting questions, issues, and notes with a body render as compact `Markdown`: headings at 14/13px semibold, lists and task lists, tables, quotes, inline code on `surface-3`, fenced code in a code box (flow 4). No raw `**`, `#`, `|---|` or fences.
2. **Long ones are folded** by measured height, not characters: about 10 lines (240px at 24px line height in compact Markdown) for briefs, reports and questions; 8 lines in Handoffs cards; 4 for note bodies. The cut is a clean edge with no fade. Under it, a text button **Show all · 38 lines** (`ink-muted`, 12px, semibold, underlined on hover, focus outline). The count is measured lines, so the user knows what opening it costs.
3. **Opens it.** The whole entry shows in place; the button now reads **Show less** and sits under the last line. Focus stays on the button. Nothing else on the page moves above the entry.
4. **Closes it.** The entry folds back. If the entry's top is above the stuck band, the feed scrolls so the entry's first line sits just under the band (`scroll-margin-top: 44px`) and focus stays on the button, now back under the fold. Without this, closing a 200-line report would leave the user far below where they were.
5. **Short entries** (under the limit, measured after render) get no button.

Keyboard and focus: Tab moves band → links in the entry, in reading order → Show all → the next row's links → the next band. Enter or Space toggles. The button has `aria-expanded` and `aria-controls` (the entry's id). The folded part is clipped with `overflow: hidden` and is `inert` while folded, so Tab never lands on a hidden link. Hit area: the text is 12px, the button's box is 44px tall (8px above, 24px below the text), overlapping the row's padding so the row doesn't grow.

Where it can go wrong:

- A table wider than the column scrolls sideways inside its own box (`overflow-x: auto`), never the page.
- An image in a brief: compact Markdown already shows `[image]`; unchanged.
- Very long single words (hashes, URLs): `overflow-wrap: anywhere`.
- The fold falls inside a code box: the box is cut with the entry; fine, the button is right under it.

Component map: `Markdown` (reused, compact; uses its `link` and `code` hooks, plus one addition in flow 3); `Fold` (new molecule, replaces `Clamp` in ActivityFeed: children, a max height, measures with a `ResizeObserver`, the button, `inert`, the scroll-back); `Clamp` stays for its other users (OpenEpic) until they move; `EntryText` (new, in the organism layer: `Markdown` compact + `linkInto` + `refs` + `CodeBox` for fences, so Activity and Handoffs render text one way).

## Flow 3: following a link

| In the text | Looks like | Goes to | Tab |
|---|---|---|---|
| `https://…`, `http://…`, `mailto:` (bare or `[label](url)`) | `link` colour, underlined, then a 12px ↗ icon | the URL, `rel="noopener noreferrer"` | new |
| `[the design](docs/design.md)`, a path of the task's repo | `link`, underlined (mono when the label is a path), ↗ | the task's Workspace preview, via `linkInto` | new |
| `task-21`, bare, in inline code, or linked, and known to the board | `link`, underlined, mono, no ↗ | `/tasks/task-21` | same |
| a hex word 7 to 40 long that starts one of the task's commits (bare or inline code) | `link`, underlined, mono, ↗ | `/tasks/:id/workspace?commit=<full hash>` | new |
| any other hex word (`9e1d04b`, from dev or another repo) | plain inline `code` or text | nowhere | |
| `task-999` the board doesn't know | plain mono | nowhere | |
| `javascript:`, `data:`, `vbscript:`, `file:`, any scheme not listed above | the label as plain text, no underline | nowhere | |
| `#heading` | as today | in-page | same |

The ↗ means "opens in a new tab", not "external": it's on every new-tab link and its `aria-label` adds "(opens in a new tab)". The underline means "link" in every theme; colour is the second cue, never the only one.

Steps, per kind:

1. **External.** The user clicks; a new tab opens. Agentry stays where it was.
2. **Repo file.** A new tab opens the task's Workspace on that file in preview, as the chat does today (`linkInto`). A path that doesn't exist opens the Workspace's own "not found" state; we don't check paths beforehand (that would mean a request per link).
3. **Task id.** Same tab, that task's page. Back returns to this task's Activity at the same scroll (the router keeps it). Only ids the board knows link (from the live task list); others stay plain, as a mistyped id would otherwise lead to an error page.
4. **Commit.** A new tab opens the Workspace with that commit. The task page fetches `/api/tasks/:id/commits` once (`useResource`, as `WorkspacePage` does) and refetches when the task's head moves; until it arrives, SHAs are plain and become links when it does (no layout shift: same mono, the underline appears).
5. **Unsafe.** Nothing to click. The label reads as text.

Where it can go wrong: a SHA prefix shorter than 7 never links; two commits sharing a 7-char prefix: link the first, the Workspace resolves the full hash we pass. `Markdown` today lets `data:` and `vbscript:` through when no `link` hook rewrites them (its check allows any `\w` start); the new rule is an allow-list of schemes, in `linkInto`, tested.

Component map: `linkInto` (moved from `CoordinatorChat.tsx` to `lib/links.ts`, extended: allow-list of schemes, task ids, SHAs; returns `null` for "not a link"); `Markdown` (one addition: an optional `refs(word)` hook its inline pass calls for bare words and for inline code that is a single word; it returns a link or null; still builds elements, never HTML); `useTaskCommits` (new hook, wraps `useResource`). The chat keeps calling the same `linkInto`, so its links gain the same safety.

## Flow 4: commands and fenced code

The user wants to see what an agent ran.

1. **A Bash action** shows the tool in small mono (`ink-muted`, "Bash"), then the command in a **code box**: `surface-1`, a `divider` hairline, 3px corners, mono 12px, 20px lines, 8px by 10px padding. The badge (`done`, `running`, `denied`…) and the reason sit to its right on desktop, under it on a phone.
2. **Plain first.** The command renders at once as plain mono text in `ink-primary`. Nothing waits for Monaco.
3. **Colour on screen.** When the box comes within 200px of the viewport (one shared `IntersectionObserver`), it asks for Monaco's colouriser (`lazy(() => import("./MonacoColorize"))`, so the chunk loads only when the first code box is near the screen). The colour lands on the same text in the same font and line height: no shift. Once coloured, it stays coloured; scrolling away doesn't undo it. A feed of 500 rows with no code on screen loads no Monaco.
4. **Long commands wrap** at spaces and operators (`white-space: pre-wrap; overflow-wrap: anywhere`, so a word breaks only when it is longer than the line). Three lines show; then **Show all · 9 lines** (the same `Fold`). Nothing scrolls sideways.
5. **Fenced code in an entry** uses the same box, the language in mono 11 `ink-muted` at the top right (a blank language shows nothing), coloured by its language the same lazy way (`bash`, `sh`, `ts`, `go`, `json`, `diff`…, through `languageFor` as `CodeFence` does). Fences show 12 lines, then Show all. Long lines in fences scroll sideways inside the box (code keeps its layout), unlike commands, which wrap.
6. **Copy.** Fenced blocks keep a Copy button (28px icon, 44px hit area) at the top right next to the language, always visible, as `CodeFence` has. Commands don't get one (selecting text works, and a button on every action is noise).
7. **Other tools** (Read, Edit, Write, Grep…) keep today's one line of mono, wrapped; they aren't shell.

Where it can go wrong:

- **Monaco fails to load** (offline, a chunk error): the box stays plain mono, no message. The code reads the same, and an error on every row would be noise. The failure is logged to the console once.
- A language Monaco doesn't know: plain.
- A command with a here-doc of 200 lines: folded at 3, "Show all · 200 lines".
- Reduced motion: nothing animates anyway.

Component map: `CodeBox` (new molecule: the box, the fold, the language label, Copy for fences); `LazyColorize` (new: the shared observer, then `MonacoColorize` under `Suspense` with the plain text as fallback and an error boundary that keeps the plain text); `MonacoColorize`, `languageFor`, `defineTheme` (reused). `CodeFence` stays for documents (WorkspacePreview); `CodeBox` is its compact sibling, and both use `LazyColorize`.

## Flow 5: the Handoffs tab

The user wants the chain of who asked whom to do what.

1. **Opens Handoffs.** The timeline is as today: newest first, a line down the left with a dot per step in the sender's colour, the "is working now · watch the terminal" line on top while a turn runs.
2. **Reads a card.** Header: **from** chip and name → **to** chip and name, both as text, the arrow drawn (lucide `ArrowRight`, `ink-muted`), the time at the right in `ink-muted` mono (was faint). If the brief starts with a heading, the heading becomes the card's title (14px semibold) and is not repeated in the body. The brief renders as compact Markdown (`EntryText`) folded at 8 lines, with **Show all · 38 lines**.
3. **Follows links in the brief** as in flow 3.
4. **Opens the turn.** The card's footer ("Led to turn-4 engineer · 48m · 14 actions · Activity ›") is the button that opens that turn's Activity. The card itself is **no longer a button**: a button can't contain links or another button, so the footer takes over. Footer text is `ink-muted`.
5. **A step with no turn** (said to you, or the turn is gone): no footer, nothing to open.

Phone (390): cards go full width less the 24px timeline gutter; the header wraps after the arrow when the names don't fit (from → on line one, to on line two is wrong; instead the time drops to its own line under the names, so `from → to` stays on one line; names truncate with an ellipsis at 120px each, the full name in the accessible name); the footer wraps, the "Activity ›" button stays at the end and is 44px tall.

`HandoffPath` (the path of agents above the tabs) is unchanged, except that its chevrons are decorative and its `×2` counts move from faint to muted.

Component map: `HandoffTimeline` (changed: card is an `<article>`, footer is the button, body is `EntryText` in a `Fold`, header uses `Avatar` 18px + `AgentName` text); `Avatar`, `EntryText`, `Fold`, `linkInto` (shared with Activity); `HandoffPath` (tiny contrast fix only).

## States

Drawn in [states.svg](states.svg).

| State | Activity | Handoffs |
|---|---|---|
| Loading (task not fetched yet) | the toolbar, then three grey bars the shape of bands with rows (`surface-2` blocks, no shimmer), and "Loading activity…" for screen readers (`aria-busy`) | three card outlines, same idea |
| Error loading the task | an inline message in the feed: "Couldn't load the activity." + the error in muted mono + a Retry button. The toolbar stays | the same, "Couldn't load the handoffs." |
| Empty (a new task) | today's `EmptyState`: "Nothing here yet" | `EmptyState`: "No handoffs yet" / "When the coordinator hands the task to an agent, it shows here." |
| A running turn, no rows yet | its band (running, live duration) and under it today's now-row "engineer is starting…" with the pulsing dot | the "is working now" line, and the card that led to it |
| Monaco fails to load | the box keeps plain mono, nothing else changes | same |
| An entry with no text | a message: the `from → to` line only; a report: "Finished, 41k tokens" only; a waiting question with no text: "Waiting" only. No empty box, no "No text" filler | a card with no brief: header and footer only |
| Commits not loaded yet / failed | SHAs plain, as "any other SHA". No error shown: it's an enhancement | same |

## Accessibility summary

- Author in text on every band; chips are `aria-hidden`.
- Text ≥4.5:1, chips ≥3:1 in both themes (table above). `ink-faint` carries no text that must be read.
- Links underlined; new-tab links say so to screen readers.
- Bands are `<button>`s (turns) or headings (you, Agentry), inside `<section aria-labelledby>` so screen-reader users can jump section by section.
- Show all / Show less: `aria-expanded`, `aria-controls`, folded content `inert`.
- `scroll-padding-top` keeps focus out from under the stuck band.
- Targets: bands 44px tall; Show all and Activity › 44px hit areas; Copy 44px hit area. Inline links in sentences are exempt (WCAG 2.5.8).
- 390px: no horizontal page scroll; tables and fences scroll in their own boxes.

## Decisions I made here

1. Sections push each other (sticky inside each section), not overlap (sibling sticky), so a band never covers half of another.
2. A turn split by a you message gets a "continued" band above the later rows, since newest-first puts them above the you band.
3. You and Agentry chips are neutral (`surface-3`, `ink-secondary` letter), not `ink-faint`: faint fails 3:1.
4. Light theme: chip letters and links mix 15 to 25% toward `ink-primary` to pass contrast. Dark unchanged.
5. ↗ marks new-tab links, all of them (files and commits too), not just external sites.
6. Task ids link only when the board knows them; SHAs only when they start a commit of the task's branch; inline code that is exactly one of those links too.
7. Handoff cards stop being buttons; the footer opens the turn (nested interactive content is invalid and breaks links).
8. Monaco failure is silent; the plain text is the fallback.
9. Commands wrap and fold at 3 lines; fences scroll sideways and fold at 12.
10. Role words: "reviewer" for the `review` column and "deployer" for `ship` (the directions said "reviews" / "ships"; nouns read better after a team name). The writer may change them.

## Open questions and assumptions

- The global focus outline (`brand` #fc6d26, 1px) is 2.9:1 on white, under the 3:1 WCAG 1.4.11 asks of a focus indicator. It's site-wide, so out of scope here; I'd raise it as a follow-up (2px, or `ink-primary` in light).
- "Known to the board" for task ids uses the live task list the page already has; tasks from other boards (another workspace) won't link. Assumed fine.
- The new `link` colour alias is a token addition (light only differs). Assumed acceptable as part of this build; the chat keeps `text-info` until restyled, per the non-goals.
- Agentry's own rows: events carry an actor kind (`internal/events`: Human, Agent, System, Worker). I map System and Worker to "Agentry" (a worker is Agentry's own machine doing Agentry's plumbing); `activity.ts`'s `who()` returns the id or kind today, so `by` needs that mapping.

## Strings

Every visible string, placeholders for the UX writer.

Activity
- Toolbar: "Everything", "Actions", "Messages", "{n} rows" / "1 row", "Newest", "Show every turn" (aria)
- Band: "{agent}", "{team}", "{team} · {role}", "Software" (blank team), roles "coordinator", "reviewer", "deployer"; "{turn}", "continued", "on {worker}"; badges "running", "ended", "stopped by you", "issue"; "{n}k tokens"
- Band accessible name: "{turn}, {agent}, {team · role}, {status}. Show only this turn"
- You band: "you"; Agentry band: "Agentry"
- Rail: "{hh:mm}", "{duration}", "{duration}…", "now"
- Rows: "Bash" (and other tool names, as logged), action badges "done", "running", "denied", "waiting for you"; "Finished", "Finished, {n}k tokens"; "Waiting"; "Something went wrong"; notes as logged today (e.g. "Read its brief", "Worktree ready", "Held in {place}", "Fetched {n} shared files")
- Message line: "{from}", "{to}", "{turn}" (leads to)
- Fold: "Show all · {n} lines", "Show less"
- Code: "{language}", "Copy", "Copied"
- Links: "(opens in a new tab)" (aria)
- Now row: "{agent} is starting…", "{agent} is working"
- States: "Loading activity…", "Couldn't load the activity.", "Retry", "Nothing here yet", "Every message, turn and tool call shows here as it happens."

Handoffs
- Card: "{from}", "{to}", "{hh:mm}", "{title}", "Led to", "From", "{turn}", "{duration}", "{n} actions" / "1 action", "{n}k tokens", "Activity"; footer accessible name "Open {turn}'s activity"
- Top line: "{agent} is working now · watch the terminal"
- States: "Loading handoffs…", "Couldn't load the handoffs.", "Retry", "No handoffs yet", "When the coordinator hands the task to an agent, it shows here."

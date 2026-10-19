# Readable Activity and Handoffs: flows for Direction A, Author bands

For [the spec](../../../spec.md) and the chosen [Direction A](direction-a.svg) in [the directions](directions.md). This is the detail the Software team builds from. The final words are in [the copy deck](copy.md): where this file or a wireframe disagrees with it, copy.md wins. Revised after [the critique](critique.md) (spec decision 9); each fix is listed under [Critique fixes](#critique-fixes).

Wireframes (regenerate with `cd docs/design/source && python3 -B activity_bands.py ../activity-readability`):

- [Activity, desktop](activity-desktop.svg): 1280 wide, a long feed with a stuck band, every row kind, every link kind, coordinator, engineer, reviewer, Agentry and you.
- [Activity, phone](activity-mobile.svg): 390 wide, every band one line, and the Newest button clear of the band.
- [Handoffs](handoffs.svg): desktop and 390, and a card scrolled into an opened brief with its header stuck.
- [States](states.svg): empty, loading, error, a running turn with no rows yet, Monaco failing, an entry with no text, a filter with nothing to show.
- [Activity, dark](activity-dark.svg): the desktop fragment in the dark theme.

## The rules everything below follows

- **An author section** is a run of rows with one author, under one band. A turn is a section (its agent). Rows outside a turn are grouped by who wrote them: **you**, **Agentry** (the loop's own rows: worktree ready, held, files fetched, moved on) or, rarely, another agent (another team's coordinator answering a request). A new author, or a new turn, starts a new section.
- **The band is the author header.** It names the author in text, always: chip, name, then team and role in muted text (the role left out when it is the name, spec decision 8), then the turn's facts. Nothing inside a section repeats the author; a message still shows `from → to`, because the receiver is someone else.
- **Colour is a hint, never the message.** The agent's colour appears only in the chip (its letter on a tint) and in the rail's dot. Links are underlined. Status is a word in a badge.
- **Flat.** Surfaces and hairlines; corners 3px (4px at most); no shadows, glows, gradients, fades, and no coloured left edge on anything.

## Tokens

Only existing tokens (ui/src/styles/globals.css), plus two light-theme aliases: `link` and `success-ink`. Ratios are WCAG contrast, measured with `color-mix` in sRGB.

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
| Links, and inline code that is a link (no code background then) | `link` (new alias): light `color-mix(in srgb, var(--color-info) 85%, var(--color-ink-primary))`, dark `var(--color-info)` | #0675af | #38bdf8 | light 5.03 on surface-0, 4.82 on surface-1; dark 7.88 / 8.29. Never on surface-3 (3.99) |
| Green text: live durations, `Finished`, `{agent} is starting…`, `{agent} is working now` | `success-ink` (new alias): light `color-mix(in srgb, var(--color-success) 70%, var(--color-ink-primary))`, dark `var(--color-success)` | #197d40 | #22c55e | light 5.19 on surface-0, 4.97 on surface-1, 4.59 on surface-2; dark 7.41 / 7.79. Never on surface-3 (4.12) or a badge tint |
| Agent chip | `agent-<colour>` on an 18% (light) / 22% (dark) tint of itself; in light the letter is `color-mix(agent 75%, ink-primary)` | | | ≥3.8:1 light, ≥4.3:1 dark, every colour |
| Neutral chip (you, Agentry) | letter `ink-secondary` on `surface-3` | | | 7.3:1 / 6.7:1 |
| Running | `info` badge (site-wide Badge contrast is a follow-up, spec decision 9); live duration `success-ink`; rail dot and the done icon stay `success` (non-text, 3.3:1) | | | |
| Focus | the global `*:focus-visible` outline, `brand` | | | see open questions |

Why the tweaks: `info` (#0284c7) is 4.1:1 on white and `success` (#16a34a) 3.3:1, under AA for 11 to 13px text, and the light theme's teal, green and orange agent colours are 2.6 to 2.9:1 as a letter on their own tint. All are fixed by mixing toward `ink-primary`; the dark theme needs none. Dots, icons and badges keep `success`: only words use `success-ink`.

## Flow 1: scanning a long Activity feed

The user opens a task in the morning and scrolls down Activity (newest at the top) to catch up.

1. **Opens the tab.** The feed starts at the top. The first band (the newest section) sits under the toolbar. If a turn is running, its band shows a `running` badge and a live duration on the rail.
2. **Scrolls down.** The band of the section being read stays stuck at the top of the scroller. Its rows scroll under it. Every row on screen is either under the stuck band or under a band that is itself on screen, so the author of every visible row can be read without scrolling.
3. **Reaches the next section.** The next band scrolls up and **pushes** the stuck one out (each section is its own element and its band is sticky inside it, so the old band leaves with its last row; no overlap, no two bands half-drawn). The new band sticks.
4. **Passes an Agentry section** between two turns: a band with a neutral chip (letter A) and the name **Agentry**, no team, no turn facts; its rows are notes as `lib/activity.ts` writes them (`Worktree ready on …`, `Held in Review`, `Copied … into the shared files`).
5. **Passes a you message in the middle of a running turn.** Newest first, the running turn reads as three sections:
   - the turn's later rows, under a band that repeats the turn's header with a muted **continued** after the turn id (the turn's true band, where it started, is further down);
   - a short **you** band (neutral chip, Y, **you**) with the message `you → coordinator` and its text;
   - the turn's band proper with its earlier rows.

   The rail keeps time for all three. If the you message was sent to the turn's own agent, the arrow names it, so it can't be confused with the band.
6. **Clicks a band.** Unchanged from today: the feed narrows to that turn (the "continued" band narrows to the same turn). The toolbar chip shows the turn and an × to go back.
7. **Goes back up.** The existing **Newest** button appears after 200px of scroll, now at the **bottom right** of the scroller (16px in from the right and bottom, 32px tall with a 44px hit area), not top centre: there it sat over the stuck band and could hide a long author name on a phone. At the bottom right it never covers a band's author. On desktop it only covers rail times; on a phone (56px rail) it also covers about 40px of the content column's last line, so below `md` the scroller sets `scroll-padding-bottom: 48px` (the button's 32px plus its 16px inset): a link or Copy button that Tab scrolls into view stops above it, never under it. WCAG 2.4.11. Flat: `surface-2`, `divider-2` hairline, 3px corners, no shadow. Drawn in [activity-mobile.svg](activity-mobile.svg), note 7.

Where it can go wrong, and what we do:

- A focused link or button scrolls under the stuck band. The band has one fixed height at every width, stuck or not (44px), so the scroller sets `scroll-padding-top: 44px` (and, on a phone, `scroll-padding-bottom: 48px` for Newest, step 7), and keyboard focus and `scrollIntoView` (the "row you came to see") never land under it. WCAG 2.4.11. Check it in all three engines (see [Build checklist](#build-checklist)).
- A section is a single row shorter than the band: the band still shows, for one row. Fine: that's the honest author.
- An agent the roster no longer has: the chip uses `ink-faint`'s neutral treatment (as `useAgentColor` falls back today), the name is still in text.
- Filters (Everything, Actions, Messages) can empty a section: a section with no rows left is dropped with its band. A turn band is never dropped in Actions, as today.

**The band.** 44px tall (was 36) at every width, stuck or not, one line that never wraps (`height: 44px; white-space: nowrap`; the issue text is the one item that shrinks and truncates). `surface-1`, hairline top and bottom. Left to right: chip (20px), **name** (mono 12.5, semibold, `ink-primary`), team and role (12, `ink-muted`: "Software · deployer"; just "Software" when the role is the agent's name), turn id (mono 11, `ink-muted`, was faint), "continued" when it is one, "on <worker>" when remote, status badge (`running`, `ended`, `stopped by you`, `issue`), tokens (mono 11, `ink-muted`), the issue text if any. The rail column (120px, 104px under `md`) carries the start time and the duration (`ink-muted`, was faint; live in `success-ink` with "…"). **Every band is an `h3`.** For a turn, the `h3` holds the band's button (the whole band is the button, as today); its accessible name is in copy.md ("reviewer, Software team, turn-6, running. Show only this turn"). You and Agentry bands are the `h3` alone (no turn to narrow to). So screen-reader users jump band to band with H, the same for every kind.

**Role** (spec decisions 6 and 8): "coordinator" when `Agent.coordinates` is set; otherwise the column's role: `review` → "reviewer", `ship` → "deployer"; otherwise none. Left out when it equals the agent's name. Team "" shows as "Software".

**Phone (390).** The band is the same 44px, one line, stuck or not: chip, name (truncated past 120px), status badge, duration. Team, role, turn id, `continued`, `on {worker}` and tokens are not shown; they stay in the band's accessible name, and come back from `md` (768px). Nothing about the band depends on whether it is stuck, so no script watches for it and nothing jumps. `scroll-padding-top: 44px` as on desktop. The rail is 56px, time only; the duration is the one in the band. The toolbar segmented control scrolls sideways as it does today.

Component map:

| Piece | Status |
|---|---|
| `ActivityFeed` (organism) | changed: renders sections instead of a flat list; an `<ol>` with one `<li>` per section (no `<section>`, no `region` landmarks: a long feed would have hundreds); the band is an `h3`, sticky inside its `<li>`, and the rows follow in a nested list |
| `lib/activity.ts` | changed: each item gets `by` (the author, from `who(e)`; Agentry for the loop's own actor); new `sections(rows)` groups `newestFirst` output into author sections and marks continued turns. No change to what is logged |
| `AuthorBand` (new molecule) | the band: `Avatar`, name, team and role, then children (turn facts) |
| `Avatar` (moved) | from `CoordinatorChat.tsx` to `molecules/Avatar.tsx`; adds `size` (20, 28), the light-theme letter mix, and the neutral look for you and Agentry. The chat imports it from there, behaviour unchanged |
| `AgentName`, `useAgentColor`, `Badge`, `Mono`, `SegmentedControl` | reused as they are |
| `roleOf(agent)` (new, lib) | coordinator / reviewer / deployer / none, team "" → Software |
| Tab heading (new) | a sr-only `h2` at the top of each tab panel, `Activity` or `Handoffs`, so the bands' `h3`s don't skip a level (the task page has no `h1` or `h2` above the panel today) |

## Flow 2: reading a long brief or report

The user wants to know what the coordinator told the engineer, or what the engineer reported.

1. **Sees the first lines.** Messages (briefs), reports (Finished), waiting questions, issues, and notes with a body render as compact `Markdown`: headings at 14/13px semibold, lists and task lists, tables, quotes, inline code on `surface-3`, fenced code in a code box (flow 4). No raw `**`, `#`, `|---|` or fences.
2. **Long ones are folded** by measured height, not characters: about 10 lines (240px at 24px line height in compact Markdown) for briefs, reports and questions; 8 lines in Handoffs cards; 4 for note bodies. The cut is a clean edge with no fade, snapped up to the bottom of the last whole line above the limit, so no line is cut in half. Under it, a text button **Show all · 38 lines** (`ink-muted`, 12px, semibold, underlined on hover, focus outline). The number is **about** the length: measured height ÷ line height, which for tables, headings and code is an estimate and changes with the width. It tells the user roughly what opening costs; tests must not expect an exact count.
3. **Opens it.** The whole entry shows in place; the button now reads **Show less** and sits under the last line. Focus stays on the button. Nothing else on the page moves above the entry.
4. **Closes it.** The entry folds back. If the entry's top is above the stuck band, the feed scrolls so the entry's first line sits just under the band and focus stays on the button, now back under the fold. No `scroll-margin-top` on the entry: `scrollIntoView` would add it to the scroller's `scroll-padding-top` and land the line a band's height too low. The scroller's padding alone sets the offset, 44px in Activity and 40px in Handoffs (under the card header). Without the scroll-back, closing a 200-line report would leave the user far below where they were.
5. **Short entries** (under the limit, measured after render) get no button.

Keyboard and focus: Tab moves band → the entry's **visible** links, in reading order → Show all → the next row's links → the next band. Enter or Space toggles. The button has `aria-expanded` and `aria-controls` (the entry's id). Hit area: the text is 12px, the button's box is 44px tall (8px above, 24px below the text), overlapping the row's padding so the row doesn't grow.

**How the fold hides what's below the cut** (spec decision 9). One rule for every fold, text or code: **clip, and take the hidden links out of the Tab order.**

1. The entry renders whole, in one box with `max-height: {cut}px; overflow: clip`. `clip`, not `hidden`: a `clip` box can't be scrolled, so focusing something inside it can't shift the visible lines. (Supported in Chromium 90+, Firefox 81+, Safari 16+; where it isn't, fall back to `overflow: hidden` and reset the box's `scrollTop` to 0 on `scroll`.)
2. After it renders, and again when a `ResizeObserver` on the box reports a new width, `Fold` measures: the full height (for "about N lines"), and the cut, snapped up to the bottom of the last whole line box above the limit (a `Range` over the text gives the line boxes; inside a code box the line height is fixed, so it's arithmetic).
3. Every focusable element in the box (`a[href], button, [tabindex]`, so links and the code box's Copy) whose **first line box** isn't wholly above the cut gets `tabindex="-1"`, its old `tabindex` kept in `data-fold-tabindex`. A link that starts above the cut and wraps below it stays focusable: its visible part is what gets the focus outline.
4. Show all removes the clip and restores every saved `tabindex`. Show less clips again and re-runs step 3.
5. The clipped text stays in the accessibility tree, on purpose: a screen-reader user reading line by line hears the whole entry and isn't told less than a sighted reader can get with one click; `aria-expanded="false"` on the button says it's folded. Only sequential focus is kept from things no one can see.

Why this rather than folding at block boundaries: one block is often taller than the fold all by itself (every command is a single code block that folds at 3 lines, and a report can open with a long table), so block folding needs the clip rule as a fallback anyway; with two mechanisms the cut would sit in a different place depending on how the Markdown happens to be split. The clip gives one predictable edge at about 10 lines everywhere.

**A single very long paragraph** (a 60-line brief with no breaks) folds the same way: the cut falls between two of its lines, the links on lines below it leave the Tab order, and Show all brings them back. Nothing special is needed.

Tests: Tab through a folded entry with links above and below the cut reaches only the ones above, then Show all; after Show all it reaches every link; after Show less, again only the visible ones; a link that wraps across the cut stays reachable.

Where it can go wrong:

- A table wider than the column scrolls sideways inside its own box (`overflow-x: auto`), never the page.
- An image in a brief: compact Markdown already shows `[image]`; unchanged.
- Very long single words (hashes, URLs): `overflow-wrap: anywhere`.
- The fold falls inside a code box: the box is cut with the entry; fine, the button is right under it.

Component map: `Markdown` (reused, compact; uses its `link` and `code` hooks, plus one addition in flow 3); `Fold` (new molecule, replaces `Clamp` in ActivityFeed: children, a max height, measures with a `ResizeObserver`, the clip, the `tabindex` save and restore, the button, the scroll-back); `Clamp` stays for its other users (OpenEpic) until they move; `EntryText` (new, in the organism layer: `Markdown` compact + `linkInto` + `refs` + `CodeBox` for fences, so Activity and Handoffs render text one way).

## Flow 3: following a link

| In the text | Looks like | Goes to | Tab |
|---|---|---|---|
| `https://…`, `http://…`, `mailto:` (bare or `[label](url)`) | `link` colour, underlined, then a 12px ↗ icon | the URL, `rel="noopener noreferrer"` | new |
| `[the design](docs/design.md)`, a path of the task's repo | `link`, underlined (mono when the label is a path), ↗ | the task's Workspace preview, via `linkInto` | new |
| `task-21`, bare, in inline code, or linked, and known to the board | `link`, underlined, mono, no ↗; title and accessible name `task-21, {title}` when the board has the title (decision 8) | `/tasks/task-21` | same |
| a hex word 7 to 40 long that starts one of the task's commits (bare or inline code) | `link`, underlined, mono, ↗ | `/tasks/:id/workspace?commit=<full hash>` | new |
| any other hex word (`9e1d04b`, from dev or another repo) | plain inline `code` or text | nowhere | |
| `task-999` the board doesn't know | plain mono | nowhere | |
| `javascript:`, `data:`, `vbscript:`, `file:`, any scheme not listed above | the label as plain text, no underline | nowhere | |
| `#heading` | as today | in-page | same |

**Inline code that becomes a link** (a SHA or task id in backticks) drops its code background: it renders exactly like a mono link, `link` colour, underlined, on the row's own surface (5.03:1 on `surface-0`), as `ActivityFeed.tsx` and `lib/links.ts` already draw refs. On `surface-3` the link colour would be 3.99:1. Inline code that isn't a link keeps `surface-3`.

The ↗ means "opens in a new tab", not "external": it's on every new-tab link and its `aria-label` adds "(opens in a new tab)". The underline means "link" in every theme; colour is the second cue, never the only one.

Steps, per kind:

1. **External.** The user clicks; a new tab opens. Agentry stays where it was.
2. **Repo file.** A new tab opens the task's Workspace on that file in preview, as the chat does today (`linkInto`). A path that doesn't exist opens the Workspace's own "not found" state; we don't check paths beforehand (that would mean a request per link).
3. **Task id.** Same tab, that task's page. Back returns to this task's Activity at the same scroll (the router keeps it). Only ids the board knows link (from the live task list); others stay plain, as a mistyped id would otherwise lead to an error page.
4. **Commit.** A new tab opens the Workspace with that commit. The task page fetches `/api/tasks/:id/commits` once (`useResource`, as `WorkspacePage` does) and refetches when the task's head moves; until it arrives, SHAs are plain and become links when it does (no layout shift: same mono, the underline appears).
5. **Unsafe.** Nothing to click. The label reads as text.

Where it can go wrong: a SHA prefix shorter than 7 never links; two commits sharing a 7-char prefix: link the first, the Workspace resolves the full hash we pass.

**The URL check, exactly.** One function, `safeHref(raw)` in `lib/links.ts` (no Monaco or React imports), used by `linkInto` and by `Markdown`'s own fallback. It returns `null` for "not a link", or a kind with **the href to render**: for an external link the parsed URL's `href` (`new URL(s).href`), for a repo path the cleaned relative path, for an anchor the `#…` string. Every caller, `Markdown`'s fallback included, renders that returned href and never the raw string from the text:

1. Remove every C0 control character and DEL (`\u0000`–`\u001F`, `\u007F`) anywhere in the string, including tab and newline: browsers drop them inside a scheme, so `java\tscript:` is `javascript:` to them.
2. Trim leading and trailing whitespace.
3. If its first two characters are each `/` or `\`, in any mix (`//`, `\\`, `/\`, `\/`; browsers read `\` as `/`): **external**, a protocol-relative URL to another host. It opens in a new tab like any external link; never a repo path.
4. If it starts with a scheme, `^[A-Za-z][A-Za-z0-9+.-]*:`, lowercase the scheme. Allow-list: `http`, `https`, `mailto` → **external**, and confirm with `new URL(s)` that it parses and its `protocol` is still one of `http:`, `https:`, `mailto:`. Any other scheme (`javascript`, `data`, `vbscript`, `file`, `blob`, anything unknown) → `null`.
5. If it starts with `#`: **anchor**, in-page, as today.
6. Anything else is a **repo path**, relative to the repo root: a leading `/` or `./` is dropped, `..` segments are resolved, and a path that climbs above the root → `null`. The app's own routes (`/tasks/…`) are never taken from text: `linkInto` builds them itself from task ids and SHAs.

`null` means the label renders as plain text: no `<a>`, no underline, no hint. The same check runs on bare autolinks, so a `javascript:` in prose never becomes a link.

**The gap in `Markdown.tsx` today.** Without a `link` hook it builds `{ href, external: /^https?:\/\// }` and only refuses hrefs that don't start with `https?://`, `#`, `/`, `.` or a word character, or that start `javascript:` (case-insensitive, untrimmed). So `data:`, `vbscript:`, `file:` and `//evil.example` (as a same-tab link) get through, and `\u0001javascript:` or ` javascript:` slip past the `javascript:` test. It also renders `href={to.href}` with the raw string. The fix: the fallback calls `safeHref` too and renders the href it returns, so `Markdown` is safe on its own, with or without a hook.

Test cases (each through `safeHref`, and through `Markdown` both with and without a `link` hook):

| Input | Result |
|---|---|
| `javascript:alert(1)`, `JaVaScRiPt:alert(1)` | `null` |
| `\u0001javascript:alert(1)`, `' javascript:alert(1)'`, `java\tscript:alert(1)`, `java\nscript:alert(1)` | `null` |
| `data:text/html,<b>x</b>`, `vbscript:msgbox(1)`, `file:///etc/passwd`, `blob:https://x/1` | `null` |
| `//evil.example`, `\\evil.example`, `/\evil.example`, `\/evil.example` | external, new tab, rendered as `https://evil.example/`, never as the raw string |
| `https://example.com`, `HTTPS://EXAMPLE.COM/x`, `mailto:a@example.com` | external, new tab |
| `docs/design.md`, `./docs/design.md`, `/docs/design.md` | repo path `docs/design.md` |
| `../../etc/passwd` | `null` |
| `#strings` | anchor |
| `foo:bar` | `null` (unknown scheme, not a path) |
| `[x](javascript:alert(1))` in Markdown | the text `x`, no `<a>` |
| a bare `javascript:alert(1)` in prose | plain text, not autolinked |

Component map: `safeHref` (new, `lib/links.ts`, the check above); `linkInto` (moved from `CoordinatorChat.tsx` to `lib/links.ts`, extended: calls `safeHref`, then task ids, SHAs; returns `null` for "not a link"); `Markdown` (two changes: an optional `refs(word)` hook its inline pass calls for bare words and for inline code that is a single word, returning a link or null, and drawn without the code background when it links; and its fallback check replaced by `safeHref`. Still builds elements, never HTML); `useTaskCommits` (new hook, wraps `useResource`). The chat keeps calling the same `linkInto`, so its links gain the same safety.

## Flow 4: commands and fenced code

The user wants to see what an agent ran.

1. **A Bash action** shows the tool in small mono (`ink-muted`, "Bash"), then the command in a **code box**: `surface-1`, a `divider` hairline, 3px corners, mono 12px, 20px lines, 8px by 10px padding. The badge (`done`, `running`, `denied`…) and the reason sit to its right on desktop, under it on a phone.
2. **Plain first.** The command renders at once as plain mono text in `ink-primary`. Nothing waits for Monaco.
3. **Colour on screen.** When the box comes within 200px of the viewport (one shared `IntersectionObserver`), it asks for Monaco's colouriser (`lazy(() => import("./MonacoColorize"))`, so the chunk loads only when the first code box is near the screen). The colour lands on the same text in the same font and line height: no shift. Once coloured, it stays coloured; scrolling away doesn't undo it. A feed of 500 rows with no code on screen loads no Monaco.
4. **Long commands wrap** at spaces and operators (`white-space: pre-wrap; overflow-wrap: anywhere`, so a word breaks only when it is longer than the line). Three lines show; then **Show all · 9 lines** (the same `Fold`). Nothing scrolls sideways.
5. **Fenced code in an entry** uses the same box, the language in mono 11 `ink-muted` at the top right (a blank language shows nothing), coloured by its language the same lazy way (`bash`, `sh`, `ts`, `go`, `json`, `diff`…). **The language lookup stays inside the lazy chunk:** `CodeBox` passes the fence's raw language string (lowercased, for the label) and only `MonacoColorize`, behind the `lazy()` import, maps it with `languageFor`, as `CodeFence` does today. `languageFor` lives in `lib/monaco.ts`, which imports `monaco-editor` at the top, so `CodeBox`, `LazyColorize` and `EntryText` must not import it (or anything from `lib/monaco`), or Monaco joins the task page's bundle. Fences show 12 lines, then Show all. Long lines in fences scroll sideways inside the box (code keeps its layout), unlike commands, which wrap.
6. **Copy.** Fenced blocks keep a Copy button (28px icon, 44px hit area) at the top right next to the language, always visible, as `CodeFence` has. Commands don't get one (selecting text works, and a button on every action is noise).
7. **Other tools** (Read, Edit, Write, Grep…) keep today's one line of mono, wrapped; they aren't shell.

Where it can go wrong:

- **Monaco fails to load** (offline, a chunk error): the box stays plain mono, no message. The code reads the same, and an error on every row would be noise. The failure is logged to the console once.
- A language Monaco doesn't know: plain.
- A command with a here-doc of 200 lines: folded at 3, "Show all · 200 lines".
- Reduced motion: nothing animates anyway.
- Monaco's light theme draws numbers in #098658, 4.41:1 on `surface-1`: marginal, inside code. Leave it, or, if the build touches `defineTheme` anyway, add one rule (`{ token: "number", foreground: "07704f" }`, 5.8:1).

Component map: `CodeBox` (new molecule: the box, the fold, the raw language label, Copy for fences; imports nothing from `lib/monaco`); `LazyColorize` (new: the shared observer, then `MonacoColorize` under `Suspense` with the plain text as fallback and an error boundary that keeps the plain text); `MonacoColorize` (reused: the only place `languageFor` and `defineTheme` are called). The lazy test asserts both that a 500-row feed with no code on screen loads no `monaco` module, and that none of `CodeBox`, `LazyColorize`, `EntryText`, `ActivityFeed`, `HandoffTimeline` imports `lib/monaco` (a static import check or an eslint `no-restricted-imports` rule on those files). `CodeFence` stays for documents (WorkspacePreview); `CodeBox` is its compact sibling, and both use `LazyColorize`.

## Flow 5: the Handoffs tab

The user wants the chain of who asked whom to do what.

1. **Opens Handoffs.** The timeline is as today: newest first, a line down the left with a dot per step in the sender's colour, the "is working now · watch the terminal" line on top while a turn runs.
2. **Reads a card.** Header: **from** chip and name → **to** chip and name, both as text, the arrow drawn (lucide `ArrowRight`, `ink-muted`), the time at the right in `ink-muted` mono (was faint). 40px tall, `surface-0`, a `divider` hairline under it. `you` has the neutral chip and a neutral slate (`ink-muted`) dot on the timeline, never a colour. If the brief starts with a heading, the heading becomes the card's title (14px semibold) and is not repeated in the body. The brief renders as compact Markdown (`EntryText`) folded at 8 lines, with **Show all · 38 lines**.
3. **Scrolls through a long opened brief.** The card's header **sticks** to the top of the Handoffs scroller (`position: sticky; top: 0` inside its own `<article>`), so the text on screen always has its author. It works like the Activity bands: each header is sticky inside its own card, so the card's end pushes it out and the next card's header takes the top; two headers never overlap. The header is the same 40px stuck or not. The scroller sets `scroll-padding-top: 40px` so a focused link in the brief never hides under it (WCAG 2.4.11). Drawn in [handoffs.svg](handoffs.svg), notes 8 and 9.
4. **Follows links in the brief** as in flow 3.
5. **Opens the turn.** The card's footer ("Led to turn-4 · engineer · 48m · 14 actions · Activity ›", the facts joined by ` · ` as in copy.md, no coloured dot; while the turn runs, a `running` badge and the action count, no duration, as copy.md says) is the button that opens that turn's Activity. The card itself is **no longer a button**: a button can't contain links or another button, so the footer takes over. Footer text is `ink-muted`.
6. **A step with no turn** (said to you, or the turn is gone): no footer, nothing to open.

Phone (390): cards go full width less the 24px timeline gutter. **The header never wraps**: from → to and the time always share one line, so the sticky header keeps one height. To fit, names truncate with an ellipsis at 96px each on a phone (120px from `md`); the full names are in the card's accessible name (copy.md). The footer keeps the agent, `Led to turn-5 · engineer · Activity ›`, and drops the duration and counts; a running turn keeps its `running` badge too, `Led to turn-6 · reviewer · running · Activity ›`, with the agent truncating at 72px (was 96) to make room; the "Activity ›" button stays at the end and is 44px tall.

`HandoffPath` (the path of agents above the tabs) is unchanged, except that its chevrons are decorative and its `×2` counts move from faint to muted.

Component map: `HandoffTimeline` (changed: card is an `<article>`, its header sticky inside it, footer is the button, body is `EntryText` in a `Fold`, header uses `Avatar` 18px + `AgentName` text); `Avatar`, `EntryText`, `Fold`, `linkInto` (shared with Activity); `HandoffPath` (tiny contrast fix only).

## States

Drawn in [states.svg](states.svg). The words are copy.md's.

| State | Activity | Handoffs |
|---|---|---|
| Loading (task not fetched yet) | the toolbar, then three grey bars the shape of bands with rows (`surface-2` blocks, no shimmer), and "Loading activity…" for screen readers (`aria-busy`) | three card outlines, same idea |
| Error loading the task | an inline message in the feed: title, the error in muted mono, the muted next step, a Try again button. The toolbar stays | the same |
| Empty (a new task) | today's `EmptyState` | `EmptyState` |
| A filter with nothing to show (Actions or Messages) | `EmptyState` saying so and how to get back to Everything | |
| A running turn, no rows yet | its band (running, live duration) and under it today's now-row "engineer is starting…" with the pulsing dot | the "is working now" line, and the card that led to it |
| Monaco fails to load | the box keeps plain mono, nothing else changes | same |
| An entry with no text | a message: the `from → to` line only; a report: "Finished, 41k tokens" only; a waiting question with no text: "Waiting" only. No empty box, no "No text" filler | a card with no brief: header and footer only |
| Commits not loaded yet / failed | SHAs plain, as "any other SHA". No error shown: it's an enhancement | same |

## Accessibility summary

- Author in text on every band; chips are `aria-hidden`.
- Text ≥4.5:1, chips ≥3:1 in both themes (table above). `ink-faint` carries no text that must be read.
- Links underlined; new-tab links say so to screen readers.
- Every band is an `h3` (a turn's holds its button), so screen-reader users jump band to band with H. Sections are plain `<li>`s: no `<section>` or `region` landmarks, which would flood landmark navigation on a long feed. Handoff cards are `<article>`s with an `aria-label` (copy.md).
- Show all / Show less: `aria-expanded`, `aria-controls`; links below the cut leave the Tab order while folded (flow 2).
- `scroll-padding-top` keeps focus out from under the stuck band (44px) and the stuck Handoffs header (40px); both have one fixed height. On a phone, `scroll-padding-bottom: 48px` keeps it out from under Newest.
- Each tab panel starts with a sr-only `h2` (`Activity`, `Handoffs`) above the bands' `h3`s.
- Targets: bands 44px tall; Show all and Activity › 44px hit areas; Copy 44px hit area; Newest 44px; the message row's leads-to `turn-5` button at least 24 × 24 (its padding overlaps the row's, so the row doesn't grow). Inline links in sentences are exempt (WCAG 2.5.8).
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
11. The fold clips and takes hidden links out of the Tab order (flow 2), rather than folding at block boundaries: one mechanism, one predictable edge, and it works for a single block taller than the fold.
12. Bands have one fixed height everywhere, stuck or not; on a phone that means one line, with team, role and turn id only in the accessible name.
13. Bands are `h3`s inside plain `<li>`s; no `<section>` landmarks.
14. Handoff card headers stick inside their cards, like the bands; the phone header never wraps (names truncate at 96px) so it keeps one height.
15. Newest moves to the bottom right of the scroller.

## Open questions and assumptions

- The global focus outline (`brand` #fc6d26, 1px) is 2.9:1 on white, under the 3:1 WCAG 1.4.11 asks of a focus indicator. It's site-wide, so out of scope here; I'd raise it as a follow-up (2px, or `ink-primary` in light).
- "Known to the board" for task ids uses the live task list the page already has; tasks from other boards (another workspace) won't link. Assumed fine.
- The new `link` colour alias is a token addition (light only differs). Assumed acceptable as part of this build; the chat keeps `text-info` until restyled, per the non-goals.
- Agentry's own rows: events carry an actor kind (`internal/events`: Human, Agent, System, Worker). I map System and Worker to "Agentry" (a worker is Agentry's own machine doing Agentry's plumbing); `activity.ts`'s `who()` returns the id or kind today, so `by` needs that mapping.

## Strings

[copy.md](copy.md) is the one list of strings; this file no longer keeps its own. Where a wireframe and copy.md disagree, copy.md wins.

### For the writer

Changes copy.md needs to match these flows (the designer doesn't edit copy.md):

1. **Phone band** (Phone table, "Unstuck band second line"): there is no second line any more. Every band is one line at 390, stuck or not: chip, name, status, duration. Team, role, turn id, `continued` and `on {worker}` are in the accessible name only, below `md`. The "Stuck band" row is now just "Band".
2. **Handoffs footer on phone** (Card footer and Phone tables): keep the agent: `Led to {turn} · {agent} · Activity ›` (`From {turn} · {agent} · Activity ›`). Critique finding 8.
3. **Handoffs card header on phone** (Phone table, and the From / To max): `from → to` and the time always on one line; names truncate at 96px each on a phone (was 120px), full names in the card's `aria-label`.
4. **Section labels** (Band accessible name, You and Agentry bands): there is no `<section aria-labelledby>` now. Each band is an `h3`; a turn's `h3` holds the button, so the heading's name is the button's name, `… Show only this turn` included. The "section label" rows and the label/button split paragraph can go. The you and Agentry `h3` text stays `you` / `Agentry`; another agent's band outside a turn is an `h3` reading `{agent}, {team} team, {role}`.
5. **Fold count** (Fold buttons): say in Notes that `{n}` is about the length (measured height ÷ line height), not an exact count. The visible text doesn't change.
6. **Newest** (Activity toolbar): note that it now sits at the bottom right of the scroller; text and name unchanged.
7. **Linked inline code** (Links table): a task id or commit written in backticks reads exactly like the bare link; no copy change, just a note.

From the re-check (turn 14):

8. **R4, live duration** (Time rail, "Duration, live", Notes): `success` colour → `success-ink` colour. Visible text unchanged.
9. **N3, stale note** (Notes, the shared-files paragraph): drop "the wireframes' `Fetched 2 shared files` is just a sketch"; no wireframe shows it now.
10. **N5, footer separator** (Card footer): no change to the strings; the wireframes now join every fact with ` · ` as copy.md does, with no coloured dot. Say in Notes that the footer has no agent dot.
11. **N7, phone footer while running** (Card footer, Phone table, and the Handoffs footer row of the 390px table): add the badge: `Led to {turn} · {agent} · {status} · Activity ›` → `Led to turn-6 · reviewer · running · Activity ›`; the agent truncates at 72px on a phone (was 96) to make room. Ended turns keep `Led to {turn} · {agent} · Activity ›`. The accessible name already carries `{status}`.
12. **R2, Newest** (Activity toolbar, Notes, with item 6): on a phone the scroller keeps 48px clear at the bottom so Newest never covers focus. No string change.
13. **N6, tab heading** (new rows): a sr-only `h2` per tab panel, `Activity` and `Handoffs`, the tab's own label.

## Build checklist

For the Software team's tests and manual checks, on top of the spec's measures.

- [ ] **scroll-padding-top and -bottom in all three engines.** In Chromium, Firefox and WebKit (Playwright's three browsers), Tab onto a link just under a stuck band in Activity, and just under a stuck card header in Handoffs; it must land fully below the band or header. At 390, Tab forward onto a link at the end of the last visible line and onto a fence's Copy button: each must stop above Newest (`scroll-padding-bottom: 48px`). If one engine lets it hide, add the fallback: a `focusin` handler on the scroller that scrolls the target into view less the band's height.
- [ ] Fold: Tab through a folded entry reaches only the links above the cut; after Show all, all of them; after Show less, the visible ones again. A link wrapping across the cut stays reachable. Focusing a link in a folded entry never shifts its visible lines (`overflow: clip`).
- [ ] Bands keep 44px when they stick, at 390 and 1280 (no layout shift; measure the band's height on both sides of the stick point).
- [ ] Newest never overlaps a band's name at 390, with `product-designer` as the author.
- [ ] Show less scrolls the entry's first line to just under the band (44px Activity, 40px Handoffs), not twice that: no `scroll-margin-top` on entries.
- [ ] Links render the href `safeHref` returns, never the raw text: `/\evil.example` becomes `https://evil.example/` in a new tab.
- [ ] Links: every row of the test table in flow 3, through `safeHref` and through `Markdown` with and without a `link` hook.
- [ ] Lazy: a 500-row feed with no code on screen loads no `monaco` module; `CodeBox`, `LazyColorize`, `EntryText`, `ActivityFeed`, `HandoffTimeline` don't import `lib/monaco`.
- [ ] Contrast, light: `success-ink` text on `surface-0`/`surface-1` (5.19 / 4.97), `link` on the row's surface, linked inline code with no `surface-3` behind it.
- [ ] Monaco light numbers (#098658, 4.41:1 on `surface-1`): leave, or one `defineTheme` rule if touched (flow 4).
- [ ] Handoffs: headers stick and push at 1280 and 390; the phone header stays one line with `product-designer → coordinator`.

## Critique fixes

Each finding of [the critique](critique.md) assigned to the designer, and where it's fixed.

| # | Fix | Where |
|---|---|---|
| 1 | Fold clips with `overflow: clip` and takes links below the cut out of the Tab order; no `inert`. Chosen over block-boundary folding, with the long-paragraph case and tests | Flow 2; desktop note 7 |
| 2 | One fixed band height, 44px, stuck or not, at every width; phone band is one line; `scroll-padding-top: 44px` matches. Desktop already one line; now it never wraps | Flow 1; activity-mobile.svg notes 1 and 4 |
| 3 | Handoff card header sticky inside its article, push not overlap, 40px fixed, `scroll-padding-top: 40px` | Flow 5; handoffs.svg, the stuck frame, notes 8 and 9 |
| 4 | `success-ink` token, light #197d40, ratios listed; dark keeps `success`; used for every green word | Tokens; all five SVGs |
| 5 | Linked inline code drops its code background: mono link, 5.03:1 | Flow 3; desktop and handoffs SVGs (`3f2a9c1`) |
| 7 | Role dropped when equal to the name; `Couldn't load the …` with no full stop; `Try again`; the muted next-step line; empty-state bodies; the filter-empty state; Strings point to copy.md; `Fetched 2 shared files` gone | All SVGs; states.svg 1, 3, 7, 9, 10; Strings |
| 8 | Phone footer keeps the agent; desktop running footer shows the badge and count, no duration; phone header: always one line, names truncate at 96px | Flow 5; handoffs.svg; For the writer 2 and 3 |
| 9 | Plain `<li>` sections, every band an `h3` (a turn's holds the button); no `<section>` landmarks | Flow 1; Accessibility |
| 10 | Newest at the bottom right of the scroller, clear of every author | Flow 1; activity-mobile.svg note 7 |
| 11 | The URL check, step by step; the `Markdown.tsx` gap; test table | Flow 3 |
| 12 | `CodeBox` passes the raw language; only `MonacoColorize` calls `languageFor`; import test | Flow 4 |
| 14 | Leads-to button hit box at least 24 × 24 | Accessibility; desktop note 10 |
| 16 | scroll-padding-top checked in Chromium, Firefox, WebKit, with a fallback | Build checklist |
| 17 | Table cells plain | Desktop and phone SVGs |
| 18 | The Finished report is drawn folded at 240px, cut snapped to a whole line | activity-desktop.svg |
| 19 | Note text no longer cut off | activity-desktop.svg, Also |
| 20 | `javascript:` example moved to a numbered note | activity-desktop.svg note 11 |
| 21 | `you` has a neutral slate dot on Handoffs | handoffs.svg |
| 22 | "about N lines" | Flow 2 |
| 23 | Monaco #098658 noted, with an optional rule | Flow 4; Build checklist |

Not done here: 6 and 15 (writer), 13 (light Badge contrast, a follow-up with the focus outline, spec decision 9).

### Re-check fixes (turn 14)

| # | Fix | Where |
|---|---|---|
| R1 | `safeHref` returns the href to render (parsed `href` for external, cleaned path for a repo path); callers, `Markdown`'s fallback included, never render the raw string. Any two leading `/` or `\` in any mix are external. `/\evil.example` and `\/evil.example` in the test table | Flow 3; Build checklist |
| R2 | `scroll-padding-bottom: 48px` on the scroller below `md`; note 7 corrected | Flow 1 steps 7 and "Where it can go wrong"; Accessibility; Build checklist; activity-mobile.svg note 7 |
| R3 | No `scroll-margin-top`; the scroller's `scroll-padding-top` (44 Activity, 40 Handoffs) sets the scroll-back offset | Flow 2 step 4; Build checklist |
| R4 | Live duration `success-ink` | Flow 1, The band; For the writer 8 |
| N1 | Phone brief drawn folded at about 10 lines | activity-mobile.svg |
| N2 | Loading text drawn as a dashed sr-only label, marked "sr-only, not visible" | states.svg 2 and 8 |
| N4 | Table drawn in its own box with a thin scrollbar under it | activity-mobile.svg note 6 |
| N5 | Footer facts joined by ` · `, no coloured dot | Flow 5; handoffs.svg |
| N6 | sr-only `h2` per tab | Flow 1 component map; Accessibility |
| N7 | Phone footer keeps `running`; agent truncates at 72px | Flow 5; handoffs.svg; For the writer 11 |
| N3 | copy.md's stale note: writer | For the writer 9 |

# Critique: readable Activity and Handoffs (Direction A, Author bands)

Reviewed against [the spec](../../../spec.md) (decisions 1 to 8), [flows.md](flows.md), [copy.md](copy.md) and the five wireframes ([desktop](activity-desktop.svg), [phone](activity-mobile.svg), [Handoffs](handoffs.svg), [states](states.svg), [dark](activity-dark.svg)), rendered to PNG and read at full size. I also read today's `ActivityFeed.tsx`, `HandoffTimeline.tsx`, `Markdown.tsx`, `MonacoColorize.tsx`, `lib/monaco.ts`, `CoordinatorChat.tsx` (`linkInto`, `Avatar`) and `globals.css`, and recomputed every contrast ratio from the tokens (WCAG relative luminance, `color-mix` in sRGB).

Severities: **blocker** (can't ship), **major** (fails a spec measure or WCAG AA, or can't be built as written), **minor** (should fix in this build), **nit** (polish). Owner: **designer** (flows.md, SVGs) or **writer** (copy.md).

## Summary

The direction is right and the detail is unusually thorough. Making the turn band the author header gets attribution from structure rather than decoration, keeps today's density and time rail, and needs no accent stripe. The link rules, the lazy Monaco path, the states table and the copy deck are close to buildable as they are. No blockers.

Five majors, all small to fix: the fold's `inert` can't be built as described; the phone band changes height when it sticks; Handoffs loses the author of a long opened brief; success-green text fails 4.5:1 in light; and linked inline code fails 4.5:1 in light.

## What works well

- **Attribution through structure.** Every row sits under a band that names its author in text; sections push each other instead of overlapping, so a band is never half-drawn over another. The `continued` band for a turn split by a `you` message is a careful answer to newest-first ordering.
- **Colour is never the message.** Name always in text, chips `aria-hidden`, links underlined, status a word. The chip and link tweaks for light theme are the right, minimal fix, and the numbers hold (below).
- **Link rules.** The table in flow 3 is complete and conservative: task ids only when the board knows them, SHAs only when they start one of the task's commits, an allow-list of schemes in `linkInto`, unsafe links read as plain text. The designer found the real gap in `Markdown.tsx` (its fallback check lets `data:` and `vbscript:` through).
- **Lazy highlighting done right.** Plain text first, colour on the same glyphs, one shared observer, silent fallback. No layout shift, no Monaco on a feed with no code on screen.
- **Handoff cards become articles** with the footer as the button: fixes today's invalid nesting (links inside a button) and is explained in the design.
- **Fold behaviour.** Measured lines in the button, `aria-expanded`/`aria-controls`, scroll-back on close so a 200-line report doesn't strand the reader. Clean edge, no fade.
- **States** are all there: empty, loading (static, `aria-busy`), error with a way out, a turn with no rows, Monaco failing, empty entries, commits not loaded.
- **Look rules** hold in every wireframe: flat surfaces and hairlines, no shadows, glows or gradients, no left accent stripes or coloured left borders, corners 2 to 4px (the 4.5 to 6.5 radii in the SVGs are the round dots).
- **Scope** is disciplined: no filters by agent, no compact/detailed toggle, no search, nothing logged differently. The commit fetch (decision 4) and `link` alias are accepted additions.
- **Copy** is plain, consistent with the board's own words (`you`, lowercase agents, `turn`, `action`), and its accessible names are better than the placeholders (agent first, "team" spelled out, `commit 3f2a9c1`, the sr-only "to" for arrows).

## Contrast, checked

flows.md's numbers are correct where stated:

| Pair | flows.md | Measured |
|---|---|---|
| `ink-muted` on `surface-1`, light | 4.6:1 | 4.63:1 (4.83 on `surface-0`) |
| `ink-muted` on `surface-0`, dark | 5.3:1 | 5.34:1 (5.62 on `surface-1`) |
| `link` light (`info` 85% + `ink-primary` = #0675af) on `surface-0` | 5.0:1 | 5.03:1 (4.82 on `surface-1`) |
| `link` dark (#38bdf8) on `surface-0` | 7.9:1 | 7.88:1 |
| Neutral chip, `ink-secondary` on `surface-3` | 7.3 / 6.7:1 | 7.29 / 6.67:1 |
| Agent chip letter, light (75% mix) on its 18% tint | ≥3.8:1 | 3.78 (green, on `surface-1`) to 5.79; passes the spec's 3:1 for chips |
| Agent chip letter, dark, on its 22% tint | ≥4.3:1 | 4.27 (violet, on `surface-0`) to 6.41; passes 3:1 |
| Rail dots, light | | 3.16 (green) and up: pass 3:1 |
| `ink-faint` | 2.6:1, no text | 2.52 to 2.63 light; correctly kept off text |

Not in the table, and failing (findings 4, 5 and 13): light `success` text 3.30:1, light `link` on `surface-3` 3.99:1, light `info` and `success` badges 3.60 and 2.87:1, focus outline `brand` 2.86:1 (known follow-up).

## Findings

### Major

**1. The fold's `inert` can't be built as written.** Designer. *flows.md, flow 2, "Keyboard and focus"; `Fold`.*
The fold clips one Markdown tree by height (`overflow: hidden`) and says "the folded part is `inert`". There is no folded *element* to make inert: `inert` on the body would also disable the links in the visible lines; without it, Tab lands on links below the cut. Worse, focusing an element inside an `overflow: hidden` box scrolls that box, so the visible lines jump and the entry looks broken.
*Why:* WCAG 2.4.3 Focus Order and 2.4.7 Focus Visible: keyboard focus lands on something the user can't see; heuristic "visible system status".
*Fix:* specify the mechanism. Suggested: clip with `overflow: clip` (can't be scrolled by focus), and after measuring, mark every focusable element whose top is below the cut with `tabindex="-1"` (restore on open); give the clipped remainder `aria-hidden` only if it's a separate block. Or fold at top-level block boundaries (the blocks past the cut render in a wrapper that is `hidden` or `inert`), falling back to the clip rule only when a single block is taller than the fold. Either way, add a test: Tab through a folded entry never reaches a hidden link.

**2. On a phone, the band changes height when it sticks.** Designer. *flows.md, flow 1, "Phone (390)"; activity-mobile.svg, note 4.*
The unstuck band has a second line (team · role · turn id, about 60px); the stuck band is one line (44px). CSS can't tell a sticky element it is stuck (scroll-state queries aren't in every browser), so this needs a script, and a sticky element that changes height while sticking shifts the rows under it, which can unstick it again: the band flickers between the two heights at the boundary. `scroll-padding-top: 44px` is also wrong for the 60px band.
*Why:* heuristic "consistency"; WCAG 2.4.11 (the padding no longer matches the band); a jittering header undermines the attribution measure on phones.
*Fix:* one fixed band height per breakpoint, never depending on stuck state. Simplest: the phone band is always one line, 44px (chip, name, status, duration); team and role stay in the band's accessible name and come back at `md`. Or always two lines, with `scroll-padding-top` set to that height under `md`.

**3. Handoffs loses the author of a long opened brief.** Designer. *flows.md, flow 5; handoffs.svg.*
Activity keeps the author in view with a sticky band; a Handoffs card doesn't. Open a 38-line brief at 1280×800 (or any brief at 390px) and scroll: the `from → to` header leaves the screen and the visible text has no author.
*Why:* fails the spec's attribution measure ("the author of every visible entry can be named without scrolling") on the Handoffs tab, and the task's own ask for a sticky author header while scrolling a long entry.
*Fix:* make the card header `position: sticky; top: 0` inside its `<article>`, on `surface-0` with a hairline under it, the same push rule as the bands (sticky inside its own card, so cards push each other). Give the Handoffs scroller `scroll-padding-top` equal to the header height. Draw one stuck header in handoffs.svg.

**4. Green status text fails 4.5:1 in the light theme.** Designer. *flows.md tokens ("Running: … live duration `success`"); activity-desktop.svg rail `18m…` and `Finished, 41k tokens`; states.svg 4 (`engineer is starting…`); handoffs.svg top line (`reviewer is working now · watch the terminal`); the green `pass` cells.*
Light `success` (#16a34a) is 3.30:1 on `surface-0` and 3.16:1 on `surface-1`; these are 11 to 13px text. Dark (7.4:1) is fine.
*Why:* WCAG 1.4.3, and the spec's "contrast 4.5:1 for text". Decision 5 already brought the rail's contrast into scope; the live duration is in the rail.
*Fix:* add a light-only alias in the pattern of `link`: `success-ink` = `color-mix(in srgb, var(--color-success) 70%, var(--color-ink-primary))` ≈ #197d40, 5.19:1 on `surface-0`, 4.97:1 on `surface-1`; dark stays `success`. Use it for every green word in these two tabs; dots and icons can keep `success`. List it in the tokens table. (In the wireframe, the `pass` cells shouldn't be green at all: compact Markdown doesn't colour table cells. See 17.)

**5. Linked inline code fails 4.5:1 in the light theme.** Designer. *flows.md, flow 3 ("inline code that is exactly one of those links too"); activity-desktop.svg `3f2a9c1`; handoffs.svg `3f2a9c1`.*
A SHA or task id in backticks becomes a link, drawn in `link` on the inline-code `surface-3` background: 3.99:1.
*Why:* WCAG 1.4.3.
*Fix:* when inline code becomes a link, drop its `surface-3` background: mono, `link` colour, underline, on the row's surface (5.0:1), which is how `ActivityFeed.tsx` and `lib/links.ts` are already drawn. Say so in flow 3 and redraw the two SHAs.

### Minor

**6. Copy.md doesn't yet carry decision 8.** Writer. *copy.md, States and Links tables, "Questions for the designer".*
Decision 8 accepted the muted error line and the task-id title, but copy.md still says the line is "left out" and the task-id accessible name is `task-21` alone. Add: the error's second line `If it keeps failing, check that Agentry is still running.` (`ink-muted`, under the error detail, both tabs); the task link's `title` and accessible name `task-21, {title}` when the board has the title, `task-21` when it doesn't; mark the four questions resolved.

**7. The wireframes predate decision 8 and the copy deck.** Designer. Copy wins in every case (it's newer and accepted):
- *activity-desktop.svg, activity-dark.svg, handoffs.svg:* `reviewer  Software · reviewer` and `coordinator  Software · coordinator` should read `reviewer  Software` and `coordinator  Software`.
- *states.svg 3 and 9:* `Couldn't load the activity.` → no full stop; `Retry` → `Try again`; add the muted line from finding 6.
- *states.svg 1:* body `…turn and tool call…` → `…turn and action…`.
- *states.svg 7:* body → `When the coordinator hands the task to an agent, the brief shows here.`
- *states.svg:* add the filter-empty state (`No actions yet` / `No messages yet`, `Choose Everything to see every row.`) that copy added.
- *flows.md Strings:* the band accessible name and `Copy`/`Retry` placeholders are superseded by copy.md; point to it rather than keep two versions. Flow 1 step 4's `Fetched 2 shared files` doesn't exist in the code (copy.md, Notes).

**8. Handoffs footer: copy and wireframe disagree.** Writer and designer.
- *Phone:* copy says `Led to {turn} · Activity ›`; handoffs.svg (note 7) keeps the agent: `Led to turn-5 ● engineer  Activity ›`. Keep the agent (it says whose turn opens, and fits); writer to update copy.md's Phone table.
- *Desktop:* handoffs.svg shows `running · 18m · 6 actions` as plain text with a duration while running; copy says a `running` badge and duration only once ended. Designer to redraw per copy.
- *Phone header:* flows says the time drops to its own line under the names; handoffs.svg keeps it on the same line. Pick one rule: "time on the same line when it fits, under the names when it doesn't" and say so in flows.

**9. Too many landmarks; turn bands aren't headings.** Designer. *flows.md, flow 1 component map and Accessibility summary.*
A `<section aria-labelledby>` is a `region` landmark; a 500-row feed would have hundreds, which floods landmark navigation. And turn bands are buttons while `you` and Agentry bands are headings, so jumping by heading skips every turn.
*Why:* WCAG 1.3.1 and 2.4.1; heuristic "consistency".
*Fix:* `<li>` per section (no `section` role, or `role="group"` with the label), and every band an `h3`; for turns the `h3` contains the button. Screen-reader users then jump band to band with H, in both kinds.

**10. The Newest button can cover the author on a phone.** Designer. *ActivityFeed.tsx today: `absolute top-3 left-1/2`; not drawn in the wireframes.*
It sits over the middle of the stuck band. At 390px a long name (`product-designer`, ~120px from x=48) runs under it, so the band's author can be hidden.
*Why:* the attribution measure; WCAG 2.4.11 for focus on the band.
*Fix:* move it to the bottom right of the scroller (or right-align it inside the band area clear of the name), and draw it in activity-mobile.svg.

**11. Specify the URL check, not just "an allow-list".** Designer (flow 3), checked by the build's tests.
Browsers strip leading control characters and spaces and tabs or newlines inside the scheme, so a regex on the raw string can miss `\u0001javascript:` or `JaVaScRiPt:`. Today's `linkInto` turns any non-http href into a Workspace link (safe, since it's encoded into the query) but the new code returns `null` for "not a link", so the check matters.
*Fix:* say: parse with `new URL(href, location.origin)` and allow only `protocol` in {`http:`, `https:`, `mailto:`}, or a same-origin path the app built itself; protocol-relative `//host` counts as external, not a repo path. Apply the same rule in `Markdown.tsx`'s own fallback (it lets `data:`, `vbscript:` and `//host` through when no hook is given). Test vectors: `javascript:`, `JaVaScRiPt:`, `\u0001javascript:`, `data:text/html,…`, `vbscript:`, `file:`, `//evil.example`, `[x](javascript:alert(1))`, and a bare `javascript:` in text.

**12. Keep the language lookup inside the lazy chunk.** Designer. *flows.md, flow 4 step 5 ("through `languageFor` as `CodeFence` does") and the `CodeBox` map.*
`languageFor` lives in `lib/monaco.ts`, which imports `monaco-editor` at the top. If `CodeBox` imports it to pick a language or label, Monaco joins the task page's bundle and the spec's speed measure fails.
*Fix:* say that `CodeBox` passes the fence's raw language string and only `MonacoColorize` (inside the `lazy` import) maps it, as `CodeFence` does today; the build's lazy test should assert no `monaco` module loads on a 500-row feed without code on screen.

**13. Badges in the light theme are under AA.** Designer, as a follow-up. *`Badge` atom: `RUNNING` (`info` on `info-soft`) 3.60:1, `success` 2.87:1.*
These ship today and are site-wide, like the focus outline, so I don't count them against this design. Raise them with the focus-outline follow-up (`success-ink` from finding 4 and `link` would fix the two light ones).

**14. Small leads-to button.** Designer. *Message row: the `turn-5` after `coordinator → engineer`.*
It's a standalone button, not a link in a sentence, about 16px tall.
*Why:* WCAG 2.5.8 Target Size (24×24 or spacing).
*Fix:* give it a 24px-tall hit box (padding overlapping the row), as done for Show all.

**15. Footer accessible name doesn't start with what's shown.** Writer. *copy.md, Card footer: "Open the activity of turn-6, reviewer".*
The visible text starts `Led to turn-6 reviewer … Activity`. A speech user says "click Led to" or "click turn 6".
*Why:* WCAG 2.5.3 Label in Name (best practice: the name starts with the visible label).
*Fix:* `Led to turn-6, reviewer, running. Open its activity` (`From turn-5, …` for the other case).

**16. Check `scroll-padding-top` in all three browsers.** Designer, for the build's checklist. *flows.md, flow 1, "Where it can go wrong".*
Sequential focus navigation honouring `scroll-padding` has differed between engines. Add to the test notes: Tab onto a link just under a stuck band in Chromium, Firefox and WebKit; if one lets it hide, a `focusin` handler on the scroller that scrolls by the band height is the fallback.

### Nits

**17.** *activity-desktop.svg, activity-mobile.svg:* the table's `pass` cells are drawn green; compact Markdown doesn't colour cells, and green-only meaning plus 3.3:1 would be two problems. Draw them plain. Designer.

**18.** *activity-desktop.svg:* the Finished report is drawn about 280px tall before `Show all · 38 lines`, past the stated 240px (~10 lines). Draw it to the rule so the build matches. Designer.

**19.** *activity-desktop.svg, Also note 10:* the note text is cut off ("Live: green with …"). Designer.

**20.** *activity-desktop.svg, last row:* `(was a javascript: link)` reads like shipping UI; move it to a numbered note. Designer.

**21.** *handoffs.svg:* the `you` dot on the timeline is a warm orange, close to `tech-lead`'s; everywhere else `you` is neutral. Use the neutral (slate) dot. Designer.

**22.** *flows.md, flow 2:* "lines" in `Show all · 38 lines` is measured height ÷ line height for Markdown with tables and code; it's an estimate and changes with width. Fine; say "about" in the design note so no one tests it as exact. Designer.

**23.** Monaco's light theme draws numbers in #098658, 4.41:1 on `surface-1`. Marginal and inside code; leave it, or add one `rules` entry in `defineTheme` if the build touches it. Designer.

## Known, not counted

- The global focus outline (`brand` #fc6d26, 1px) is 2.86:1 on white, under 1.4.11's 3:1. Site-wide; the separate follow-up stands. The footer button in handoffs.svg shows it.
- Light badges (finding 13).

## Scope

Nothing beyond the spec: no agent filter, compact/detailed toggle, search, threading or logging change. The commit-link fetch (decision 4), the `link` alias and moving `Avatar` and `linkInto` to shared places are within the accepted decisions. The filter-empty wording copy added is a string change in an existing component, not new design. The `success-ink` alias in finding 4 is the same kind of token fix as `link`.

## Blockers and majors

Blockers: none.

Majors:
1. Fold: specify how hidden content is kept out of the Tab order without `inert` on the visible lines (designer).
2. Phone band: one fixed height per breakpoint, no layout change when stuck (designer).
3. Handoffs: sticky card header so a long opened brief keeps its author on screen (designer).
4. Light `success` text 3.3:1: add a `success-ink` alias, ≈ #197d40 (designer).
5. Linked inline code on `surface-3` 3.99:1: drop the code background when linked (designer).

Minors 6 (writer) and 7 (designer) are quick and should go in the same pass so copy and wireframes agree before the build.

Verdict (turn 8): changes requested. Superseded by the re-check below.

## Re-check (turn 14)

Checked the designer's fixes (c76b5cf: flows.md and the five SVGs, regenerated from `docs/design/source/activity_bands.py`) and the writer's (4c0e066: copy.md) against spec decision 9. I rendered all five wireframes to PNG and read them at full size, recomputed the new ratios from `globals.css`, and read `Markdown.tsx`'s current link fallback to test the URL check.

### Earlier findings

| # | Finding | Status | Checked |
|---|---|---|---|
| 1 | Fold `inert` | **Fixed** | Flow 2: `overflow: clip` (fallback `hidden` + reset `scrollTop`), focusables whose first line box isn't above the cut get `tabindex="-1"` with the old value saved, restored on Show all. A link wrapping across the cut stays reachable. Text stays in the accessibility tree on purpose, with `aria-expanded` saying it's folded: sound. Tests listed. Block folding rejected with a good reason |
| 2 | Phone band height when stuck | **Fixed** | 44px, one line, at every width, stuck or not; no script. Team, role, turn id, `continued`, `on {worker}` live in the accessible name below `md`. `scroll-padding-top: 44px` matches. activity-mobile.svg notes 1 and 4 |
| 3 | Handoffs lose the author | **Fixed** | Card header sticky inside its `<article>`, 40px fixed, push not overlap, `scroll-padding-top: 40px`; phone header never wraps (names truncate at 96px). The stuck frame in handoffs.svg shows it |
| 4 | Light `success` text | **Fixed** | `success-ink` = `color-mix(success 70%, ink-primary)` = #197d40, recomputed: 5.19 on `surface-0`, 4.97 on `surface-1`, 4.59 on `surface-2`; dark `success` 7.41 / 7.79. Every green word in the light SVGs is #197d40 (live durations, `Finished, 41k tokens`, `engineer is starting…`, the Handoffs top line); #16a34a remains only on dots and the done icon (3.30 / 3.16, non-text, passes 3:1) |
| 5 | Linked inline code | **Fixed** | Flow 3 and copy say it drops the code background; `3f2a9c1` is drawn as a mono link with no `surface-3` (5.03:1) on desktop, Handoffs and dark, while `9e1d04b` keeps it |
| 6 | Copy lacks decision 8 | **Fixed** | Error next-step line in both tabs, task-id tooltip and accessible name `task-21, {title}`, the four questions marked resolved |
| 7 | Wireframes predate the copy | **Fixed** | Role dropped when it is the name (`reviewer  Software`, `coordinator  Software`); `Couldn't load the …` without a full stop, `Try again`, the muted line; empty bodies; filter-empty state 10; Strings point to copy.md; `Fetched 2 shared files` gone from every SVG |
| 8 | Handoffs footer | **Fixed** | Phone footer keeps the agent (copy and SVG agree); desktop running footer shows the badge and count, no duration; phone header rule stated. See new nit N5 on the separator |
| 9 | Landmarks, band headings | **Fixed** | Plain `<li>` sections, every band an `h3`, a turn's `h3` holds its button; copy.md's band names follow. See new nit N6 on the level above |
| 10 | Newest over the band | **Fixed** for the author: bottom right, drawn at 390. See new minor R2 for focus |
| 11 | URL check | **Fixed**, with one gap: see new minor R1 |
| 12 | Language lookup outside the lazy chunk | **Fixed** | `CodeBox` passes the raw string; only `MonacoColorize` calls `languageFor`; an import test or `no-restricted-imports` rule on the five files |
| 14 | Leads-to target | **Fixed** | 24 × 24 hit box, desktop note 10 |
| 15 | Footer name vs visible text | **Fixed** | `Led to {turn}, {agent}[, {status}]. Open its activity`, same at every width |
| 16 | scroll-padding in three engines | **Fixed** | In the Build checklist with the `focusin` fallback |
| 17 | Green table cells | **Fixed** | Plain |
| 18 | Report drawn past the fold | **Fixed** | About 230px, cut inside the code box on a whole line |
| 19 | Note cut off | **Fixed** | |
| 20 | `javascript:` note in the UI | **Fixed** | Note 11 |
| 21 | Orange `you` dot | **Fixed** | Slate |
| 22 | "about N lines" | **Fixed** | Flow 2 and copy.md Notes; tests mustn't expect an exact count |
| 23 | Monaco #098658 | **Fixed** | Noted, optional `defineTheme` rule (5.8:1) |
| 13 | Light Badge contrast | Not counted | Follow-up with the focus outline (decision 9). `RUNNING` is still `info` on `info-soft` in the SVGs, as expected |

### New findings

None is a blocker or major.

**R1. Minor: the URL check can still let a same-tab off-site link through the `Markdown` fallback.** Designer. *flows.md, flow 3, "The URL check, exactly", steps 3 and 6, and the test table.*
Step 3 treats only `//` and `\\` as protocol-relative. Browsers read any mix of the two, so `/\evil.example` and `\/evil.example` also go to `evil.example`. Step 6 then calls them repo paths, which is safe through `linkInto` (it builds the Workspace URL and encodes the path) but not through `Markdown`'s own fallback: today it renders `href={to.href}` with the raw string (`Markdown.tsx` line 295 to 300), and flows doesn't say the fallback renders the normalised value. Result: a same-tab link off the app, with no `noopener`.
*Why:* the spec's safety constraint ("external links new tab, `rel=noopener noreferrer`") and the purpose of the check.
*Fix:* step 3: "starts with two characters each `/` or `\`" is external. Say that `safeHref` returns the href to render (the parsed URL's `href` for external, the cleaned relative path for a repo path) and that callers, the fallback included, render that and never the raw string. Add `/\evil.example` and `\/evil.example` (external, new tab) to the test table.

**R2. Minor: on a phone the Newest button can sit over a focused control.** Designer. *flows.md, flow 1 step 7; activity-mobile.svg note 7.*
At 390 the rail is 56px, and Newest is about 96px wide, 16px in from the right: it covers about 40px of the content column at the bottom of the scroller (x 322 to 375 in the wireframe). Tabbing forward scrolls each new target to exactly that bottom edge, so a link at the end of a line or a fence's Copy button (top right of its box) lands mostly under the button. Not entirely hidden, so 2.4.11 is met by a few pixels, but it reads as focus vanishing. Note 7's "it can only cover rail times" holds on desktop (120px rail), not on a phone.
*Fix:* give the scroller `scroll-padding-bottom` of the button's height plus its inset (48px; 56px is fine) below `md`, or on every width, and correct note 7. Add it to the Build checklist next to `scroll-padding-top`.

**R3. Minor: the fold's scroll-back offsets twice.** Designer. *flows.md, flow 2 step 4.*
Closing a long entry scrolls with `scroll-margin-top: 44px` on the entry, but the scroller already has `scroll-padding-top: 44px`; `scrollIntoView` adds both, so the first line lands 88px down, a band's height below where the design says. On Handoffs the band is 40px, not 44.
*Fix:* drop `scroll-margin-top`; the scroller's `scroll-padding-top` (44 in Activity, 40 in Handoffs) puts the entry just under the band or header in both tabs.

**R4. Minor: the live duration is still `success` in two places.** Designer and writer. *flows.md, flow 1, "The band" ("live in `success` with '…'"); copy.md, Time rail, "Duration, live" ("`success` colour").*
The tokens table, desktop note 12 and every SVG say `success-ink`; these two lines predate it. A builder reading either will ship 3.3:1 text.
*Fix:* `success-ink` in both.

**N1. Nit:** *activity-mobile.svg, right frame:* the coordinator's brief is cut after one line (`Read spec.md first. Then:`) before `Show all · 38 lines`; briefs fold at about 10 lines. Draw it to the rule or say it's cropped. Designer.

**N2. Nit:** *states.svg 2 and 8:* `Loading activity…` and `Loading handoffs…` are drawn as visible text (2 overlaps a grey bar), but flows and copy say sr-only. Mark them as sr-only in the frame's note or drop them from the drawing. Designer.

**N3. Nit:** *copy.md, Notes:* "the wireframes' `Fetched 2 shared files` is just a sketch" is stale now the wireframes don't show it. Writer.

**N4. Nit:** *activity-mobile.svg, right frame, note 6:* says a thin scrollbar is shown on the wide table; none is drawn, and the `Where` column is simply cut at the frame. Draw the scrollbar or the box edge. Designer.

**N5. Nit:** *Handoffs footer:* copy.md joins the facts with ` · ` (`Led to turn-4 · engineer · 48m …`); handoffs.svg and flow 5 draw the agent's coloured dot in place of the first separator (`Led to turn-5 ● engineer 48m · …`). Either is fine; pick one so the build doesn't guess. Copy wins by the stated rule, so the designer redraws, or the writer adopts the dot (decorative, `aria-hidden`). Designer and writer.

**N6. Nit:** *Heading levels:* bands are `h3`, but the task page has no `h1` or `h2` above the tab panel today (`TaskPage.tsx` has no heading; only `TaskBoard` has an `h2`). A sr-only `h2` naming the tab (`Activity`, `Handoffs`) keeps the outline from skipping a level. Add it to the component map. Designer.

**N7. Nit:** *Phone Handoffs footer:* the running badge is dropped with the counts, so on a phone a card doesn't show that its turn is still running (the top line does, for the current turn). The badge fits (`RUNNING` ≈ 56px; truncate the agent harder if needed). Optional. Designer and writer.

### Also checked, no change needed

- **Consistency:** copy.md, flows.md and the SVGs now agree on the band contents, role dropping, the h3 structure and accessible names, the footer names, the states' words and the phone cuts, apart from R4, N3 and N5.
- **Focus under sticky things (2.4.11):** one fixed height for bands (44) and card headers (40), padding to match, three-engine check with a fallback. Newest at the bottom right never covers an author (R2 is about focus only).
- **390px:** band ≈ 300 of 342px, card header ≈ 285 of 334, footer ≈ 317 of 334; tables and fences scroll in their own boxes; commands wrap. No page scroll sideways.
- **Light and dark:** dark wireframe on the real dark tokens (surface-0 #1a1d23, surface-1 #16181d, muted #8b919e 5.3:1, link #38bdf8 7.9:1, success #22c55e 7.4:1).
- **Look rules:** no shadows, glows or gradients in any SVG (the one match for "shadow" is note 6's text saying there are none); no left accent stripes or coloured left borders (the Handoffs timeline line is today's, neutral, not an edge on a card); corners 2 to 4px, the larger radii are round dots and the error icon's circle; Newest is flat.
- **Link safety:** apart from R1, the steps strip C0 controls and DEL, allow-list `http`, `https`, `mailto` by parsed `protocol`, refuse every other scheme and paths climbing above the root, and run on autolinks too. The test table covers every vector I asked for.
- **Scope:** no filters, compact/detailed toggle, search or logging change. The new pieces (`success-ink`, Newest's position, the filter-empty copy, `Copy code`) are small fixes inside the existing components, not new features.

### Blockers and majors

None. Majors 1 to 5 are fixed. R1 to R4 are minors to fold into the build; R1 and R3 are a sentence each in flows.md and R4 is two words, so they can be fixed by the designer and writer in a quick pass or taken by Software as written here.

Verdict: approved

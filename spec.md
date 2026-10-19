# Spec: readable Activity and Handoffs

## Who and when
The person who runs Agentry (one user, a developer) opens a task's **Activity** or **Handoffs** tab to see what the agents did: usually after the fact (in the morning, after a night of turns), sometimes while a turn runs. Feeds run to hundreds of rows across many agents (coordinator, engineer, reviewer, tester, design agents, other teams' coordinators) and the user.

## The job
Catch up on a task: find out who handed what to whom, what each agent was told, what it reported, and what it ran, then jump to the file, task or commit being talked about.

## Problem
Briefs, reports and commands in Activity and Handoffs are hard to read and hard to attribute, so catching up on a task means decoding raw text and guessing who wrote each entry.

## How we will know it worked
- Attribution: on any screen of a scrolled feed, the author of every visible entry can be named without scrolling (target: 100% of entries, checked on the demo feed at 1280×800 and 390px wide).
- Reading: no raw Markdown syntax (`**`, `#`, `|---|`, ``` fences) visible in any entry of the demo feed.
- Links: every URL, repo file path in a link, and task id in an entry is a working link.
- Speed: opening Activity on a 500-row feed loads no Monaco code until a code block or command is on screen; scroll stays smooth.

## Constraints
- Web UI in ui/ (React, Tailwind, atomic components). Reuse `molecules/Markdown.tsx` (compact mode, `code` and `link` hooks), `MonacoColorize.tsx` (lazy), `AgentName`, `useAgentColor`, `Clamp`, `SegmentedControl`, `Badge`. CoordinatorChat already has an `Avatar` (letter on a tint of the agent colour) and `linkInto` for links: reuse, don't reinvent.
- Data we have: agent name, colour, team (Agent.team); the coordinator of each team; "you" for the user. Anything else (a role label) must come from existing data or be left out.
- Look: flat and clean, as the existing pages. No shadows, glows or gradients; **no left accent stripes**; square corners (≤4px radius). Light and dark themes via existing tokens.
- Accessibility: WCAG 2.2 AA. Author is never shown by colour alone (name always in text); contrast 4.5:1 for text, 3:1 for chips; keyboard reachable "Show all" and links; focus visible.
- Safety: no `javascript:` or other unsafe links; external links new tab, `rel="noopener noreferrer"`.
- Performance: Monaco loads lazily, only for code on screen.
- Narrow widths (≥390px) must still work.

## Non-goals
- Changing what is logged or the event model.
- A new commit view (link SHAs only if a commit view already exists).
- Search inside the feed, threading, reactions, editing entries.
- Restyling other pages, the chat, or the Workspace.

## Decisions
(Filled in as they are made.)

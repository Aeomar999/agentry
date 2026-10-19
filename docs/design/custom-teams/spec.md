# Spec: build and edit a custom team on the Agents page

Story task-177 of epic task-174. Design only; task-178 builds it.

## Who and when
The one person who runs Agentry (a developer) is on the Agents page (`ui/src/pages/AgentsPage.tsx`, route `/agents`). Today they can only add a built-in team template (Software, Writing, Research, Design, Visionaries; Networking coming) from the **Add team** list, or make one agent at a time on `/agents/new` (`organisms/AgentForm.tsx`) by typing a free-text team name. They want a team of their own: their own agents, prompts and coordinator, made in one sitting and changed later, and they want to keep a good team as a template to add again for another repo or after removing it. Mostly at a desktop; sometimes on a phone (≥390px).

## The job
Put together a working team (a coordinator plus the agents it hands work to) without hand-making each agent and without knowing the rules a team must follow, then keep it in shape and reuse it.

## Problem
A team can only be made from a built-in template or assembled agent by agent with no check that the result is a valid team, so a custom team is slow to make and easy to get wrong.

## How we will know it worked
- Time to a working team: from the Agents page, a user can make a three-agent team copied from a template and changed (one agent renamed, one prompt edited, one agent added) in under 3 minutes in a walk-through on the demo server, with no trip to `/agents/new`.
- Validity: no way to save a team with zero or two coordinators, duplicate agent names, a team name in use, or a profile policy.toml lacks without either fixing it or accepting the missing-profile offer. Every error names the field and how to fix it, next to that field.
- Reuse: a saved user template shows in Add team next to the built-ins and adds the same team again in one action.
- Nothing lost: a server error on save keeps everything typed.

## Constraints
- Web UI in `ui/` (React, Tailwind, atomic components). Reuse the existing fields and rules of `organisms/AgentForm.tsx` (name, description, colour radiogroup, "Its work counts as" column, Coordinates, engine, model, policy profile), the Add team list and the missing-profile offer in `AgentsPage.tsx` (`missingProfilesOf`), `RemoveDialog`, `ColumnBadge`, `AgentName`, `useAgentColor`, `Badge`, `SegmentedControl`.
- Data: an agent is `agents.Agent` (name, description, prompt, engine, model, profile, coordinates, team, workers, color, column, template). Existing routes: GET /api/teams/templates, POST /api/teams, POST/PUT/DELETE /api/agents/{name}, POST /api/agents/{name}/rename. A team is the set of agents sharing `team`; there is no team record today. User templates are new (events in the log, preferred by the epic); the design says what the UI needs, not the storage.
- Columns: Builds (In progress), Reviews (Review), Ships (Shipping).
- One page plus a dialog or a side panel. No new navigation, no new routes beyond what `/agents` already has (a query parameter for an open panel is fine).
- Look: flat and clean like the existing pages. No shadows, glows or gradients; no left accent stripes; square corners (≤4px radius). Light and dark via existing tokens; light ink-faint fails AA for text, use ink-muted.
- Accessibility: WCAG 2.2 AA. Errors in text, tied to their field (`aria-describedby`), summarised at the top on a failed save with links to each field; coordinator choice is a real radio group (exactly one); focus moves sensibly when agents are added or removed; keyboard reachable everywhere; targets ≥24px.
- Phone width 390px must work: no horizontal scroll, long prompts readable.

## Non-goals
- How coordinators route work to custom teams and the default coordinator prompt (epic item 5; we only show where the prompt default appears).
- Export/import of a template as a file.
- Machines (workers) and knowledge editing beyond linking to the existing agent page.
- Changing built-in templates, or editing a team made from a built-in template beyond what the agent page already allows (decide in design whether such a team is editable as a custom team).
- Storage format of user templates.

## Decisions
See "Decisions for the engineer" in [the design note](../custom-teams.md): a modal side panel, agents as inline accordion rows, the team saved in one checked request.

Decisions made at the end of the design, and why:
- **Approved after two rounds of critique** ([critique.md](critique.md), round 2 at 44c3ea92). Both majors are fixed: the phone footer keeps one row, and nothing sticks below 500px of height (WCAG 1.4.10, 2.4.11). The panel follows the URL: Back, loading, a failed load, and a team that isn't there.
- **Fields are read-only while saving.** It is simpler to build than letting someone edit during a save, and nothing typed is lost either way.
- **The save carries the team's version.** If the team changed in another tab, the server refuses with a 409. Otherwise the save would silently overwrite those changes.
- **The `*-ink` token switch for `Banner` titles and danger `Button` text is part of task-178.** Without it the error summary title is 3.2:1.
- **The round 2 minors and nits go into the build brief (task-178), not another design round.** None of them changes a layout:
  - r1: the "changed elsewhere" 409 names the agents that changed, and the copy says saving stays refused until the user reloads.
  - r2: under 640px, the one-row footer leaves out "{n} things to fix, listed at the top."
  - r3: copy.md gets the `Add this team, {template}` / `Add another, {template}` names, drops the "saved with later edits" toast, and lists the settled questions as answered.
  - r4: the placeholder contrast is 4.63:1, which still passes.
- **Left for tech-lead:** whether open tasks move to a new coordinator (decision 4), and the app-wide primary-button contrast (2.86:1), which is its own task.

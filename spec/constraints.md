# Constraints

How Agentry is built and must stay built. Each constraint says what holds and how anyone checks it. Everything here comes from README.md (Work on it, How it works, The policy, Features' settings and secrets), docs/decisions.md, the package comments under `internal/`, the Makefile and `.gitlab-ci.yml`. Nothing here is planned: it is how dev is built today.

## 1. Readability and separation of concerns

The code is easy to read, with each concern in its own package: one folder under `internal/` does one job, and folders only depend on the ones below them (docs/decisions.md, 2026-10-16; README.md, Work on it). Each file does one job, and names say what it does without a comment (docs/decisions.md, 2026-10-16). The leaves, `events`, `extensions` and `httpapi`, import no core package that could import them back.

The packages under `internal/`, by folder as in docs/design/repo-organization.md section 2, each described in its own package comment:

`internal/platform`, the outside world, with no Agentry concept:

- `client`: calls a running server's API, for the commands that run beside it: the desk's `agentry task`, `agentry mcp` and the status line.
- `engines`: everything Agentry needs from an agent CLI, so the rest of Agentry never knows which engine it is running.
- `exactgob`: gob that reads a value back exactly as it was written, nil and empty told apart, for the snapshot.
- `hookcall`: the contract between `agentry hook` and the server, imported by both sides so they cannot disagree.
- `hostaddrs`: the addresses other machines reach this one on: what serve prints as links and what its certificate names.
- `launch`: what a program that opens Agentry for you does: find the server running from a data folder, or start `agentry serve`, and build the signed-in link to its page, for `agentry open` and the desktop app.
- `pinned`: how Agentry's own clients reach a server served with Agentry's own CA: they trust that CA and nothing else.
- `procs`: what Agentry does with processes that differs between Unix and Windows.
- `schema`: builds the JSON Schemas of MCP tools' arguments and of what Claude Code asks you.
- `sessions`: each task's agent in a session of its own, kept by a small host process, so it runs on when Agentry restarts.
- `terminal`: a task's agent session live in the browser, fanned out to every viewer, keystrokes back only from the one in control.
- `tlsca`: Agentry's own certificate authority, name constrained, its key kept in the data folder.
- `workspace`: reads a task's worktree for its Workspace page, the way an editor's git view does; it only reads, never outside the worktree.
- `worktrees`: each task's own git worktree, branch and port, so agents on different tasks never work on each other's code.

`internal/core`, what the log holds, the log, the read model, check 3, and how every handler answers:

- `events`: what the log holds: the event types, the catalogue of what each one must carry, and a JSON Schema of each.
- `store`: the append-only event log, in SQLite; Propose is the way in for every event but the archive, erase and redact markers.
- `current`: the state of every stream in memory, as of the last event the log committed; built from the log at every start, or from a snapshot that is only a cache.
- `rules`: check 3 of every proposal: given what happened so far, may this happen now? Run by the store inside Propose, under its lock.
- `httpapi`: how every handler of the HTTP API answers, the core's and an extension's alike; a leaf that imports only events, extensions and the tasks domain.

`internal/domain`, one folder per spec area, its root the model and its subfolders its routes, tools and background work:

- `agents`: who can be handed work: each agent's name, prompt, engine and the machines it may run on; the defaults are only what a new install starts with.
- `tasks`: a task's state from its events.
- `waitlist`: the core's part of what waits for you, each approval, question and stop as one entry; it names no extension.
- `workers`: the machines, besides the one Agentry runs on, that agents can run on: who has joined, how they prove it, and who is online.
- `workerlink`: connects a worker to the server: the worker dials out and keeps one websocket open.
- `workerservice`: keeps `agentry worker run` running in the background on a worker machine, started at login.
- `connections`: the machines whose Claude Code has Agentry's tools, connected from the Integrations page: who they are, and how they prove it.
- `policy`: decides one tool call: allow it, deny it, or ask the user; the rules are in policy.toml, not here.
- `hooks`: the server side of `agentry hook`: each hook call becomes events, and a gate call is decided, or waits for the user.
- `settings`: your settings from the settings stream; one you never changed has its default.
- `settingsfile`: the settings file, features.toml beside policy.toml: each feature's settings and secrets, watched as the policy is and kept out of the log.
- `sharedfiles`: a task's shared files, kept once on the server by hash and filled into `.agentry/files` before a turn.
- `webpush`: what Web Push needs: Agentry's key pair and the browsers subscribed, kept in a file beside the log, never in it.

`internal/extensions`, the extension framework and each extension:

- `extensions`: what an extension, such as ideas, adds to Agentry; a leaf the core reads as a list, never naming one.
- `ideas`: an idea's state from its events, as tasks does for a task.
- `schedules`: a schedule's state from its events; a schedule makes a task at a set time.

`internal/runtime`, what runs:

- `coordinator`: the loop: every task goes to its coordinator, its decision starts the next agent, and every report or issue goes back to it.
- `turns`: starts and stops agent turns: one agent working once in the task's worktree, from a brief.
- `watchdog`: catches agents that go quiet, on a clock; every problem it finds is an issue, which goes back to the coordinator.
- `archive`: moves done tasks out of the live log, at start and then hourly, and gives the space back.
- `machines`: where agents work: the machine Agentry runs on, or a worker; everything done to a worktree, session or terminal goes through a Machine.
- `desk`: runs Claude, the session you talk to in the dock; it never gets the UI token.

`internal/web` and `internal/mcp`, serving the user:

- `web`: the HTTP side of `agentry serve`: the UI, the API it uses, and the endpoint the hooks post to.
- `mcp`: Agentry as a Model Context Protocol server, so Claude Code in any repo has Agentry's tools; an approval is asked of you through Claude Code, never of the model.

`internal/testkit`, what tests share, never imported by code:

- `storetest`: opens stores for tests the way serve does, and grows synthetic logs from a copy of a real one for benchmarks.
- `deeptest`: checks copies of state by reflection, finding memory two values share.

Outside `internal/`, under `tools/`, the spec's own check:

- `speccheck`: `spec/check`, which checks every scenario under `spec/` against the rules in spec/README.md.

Checked by: the reviewer on every change, who asks of each new file and function whether it has one job and sits in the package of its concern, and whether the dependency points downward; the `internal/` list above matching the last element of each path `go list ./internal/...` prints, leaving out a `<name>test` package beside its `<name>` package (45 packages); `go test ./tools/importrules`, with `TestEveryImportPointsDown`, `TestEveryPackageHasALayer` and `TestTestkitIsOnlyForTests` in `rules_test.go`, which fail when an import points up the layers of docs/design/repo-organization.md section 3 (so `extensions` and `httpapi` never import `store`, `current` or `rules`), when a package has no layer, and when code that isn't a test imports `storetest` or `deeptest`.

## 2. The append-only event log is the only state

Every step is an event in one append-only log, and Agentry's pieces never call each other to say what happened: they write events to the log, and the others read the log (README.md, How it fits together). The log is SQLite in the data folder, append-only by triggers, so `UPDATE` and `DELETE` fail in the database itself and do not depend on the code remembering; the only deletes are of a task's rows that its archive or erase marker covers, and the only updates are of rows a redaction names (`internal/core/store`). One writer proposes every event through Propose, which runs the checks against the model and appends the event or a rejection in its place. The read model, `internal/core/current`, is the in-memory state of every stream, built from the log at every start and changed only by the store as each event commits, so it can never disagree with what happened; a snapshot is only a cache, thrown away whenever it might disagree with the log (docs/design/read-model.md). Readers follow the log by position, each with its own cursor (docs/design/proofs.md, 05 event-store).

Checked by: `go test ./internal/core/store`, with `TestEventsCannotBeChangedOrDeleted`, `TestProposeAppendsOrRecordsTheRejection`, `TestRacingProposalsAcceptExactlyOne` and `TestReopenKeepsTheLog` in `store_test.go`; `TestNoEventIsReadBeforeTheModelHasIt` and `TestAFailedWriteNeverReachesTheModel` in `applied_test.go`; `reader_test.go`, `pool_test.go` and `resume_test.go`; and `go test ./internal/core/current ./internal/core/events`. The store's test files: `applied_test.go`, `archive_test.go`, `catalogue_test.go`, `erase_test.go`, `history_test.go`, `pool_test.go`, `read_test.go`, `reader_test.go`, `redact_archived_test.go`, `redact_counted_test.go`, `redact_knowledge_test.go`, `redact_property_test.go`, `redact_rows_test.go`, `redact_test.go`, `refold_test.go`, `resume_test.go`, `store_test.go`, `unarchive_test.go`, `vacuum_test.go`.

## 3. Every agent action goes through the gate

Before an agent runs a command, edits a file or fetches a page, its engine calls `agentry hook`, which posts the action to the server; the policy judges it with the working agent's profile before it runs, and the first rule that matches decides allow, ask or deny (README.md, What happens to one action and The policy). Anything short of a clear yes blocks the action, the server being down or slow included: the hook has its own deadline and exits 2 on any failure, so the gate fails closed. A denied action never runs, and a denial stays a denial. An ask waits for the user up to 30 minutes; with no answer the agent is told no and the action stays in Needs you, and a late approval lets that exact command through once, in the asking agent's next turn, if the policy still asks rather than denies. The policy judges only shell code: quoted text and heredoc bodies are data. Engines run in never-ask mode, so Agentry's hook is the only gate (docs/decisions.md, 2026-10-15).

Checked by: `go test ./internal/domain/policy`, with `TestTheDeveloperProfile`, `TestTheReadOnlyProfile`, `TestTheJudgingProfilesWriteNothingFromTheShell` and `TestAgentrysOwnFilesAreAlwaysOffLimits` in `default_test.go` and the shell reading in `shell_test.go`; `go test ./internal/domain/policy/hooks`, with `TestTheGateFollowsThePolicy`, `TestAnAskedActionWaitsForTheUser`, `TestAnAskThatTimesOutIsHeldForTheUser`, `TestALateApprovalLetsTheSameCommandThroughOnce`, `TestAPassDoesntOverrideADenial`, `TestOnlyTheWorkingAgentPassesTheGate`, `TestTheGateUsesTheWorkingAgentsProfile` and `TestTheHTTPEndpointNeedsTheHookToken` in `handler_test.go`; and `go test ./cmd/agentry ./internal/platform/engines`, where an unreachable, slow or broken server exits 2 and hooks written fail closed.

## 4. No secrets in the log or the repo

Credentials are kept as hashes: a worker's join code and a connected machine's install code work once and only their hash is kept, and the credential each machine then keeps is in the log only as its hash (`internal/domain/workers`, `internal/domain/connections`). A sign-in code works once, for 5 minutes, and wrong codes are limited. The tokens, `ui-token`, `hook-token`, `desk-token` and `helper-token` in the data folder, the log, the policy, the settings file and the CA's folder and key are protected in code: no agent may touch them, whatever the policy says (README.md, The policy, Built in; `cmd/agentry/policy.go`). A feature's settings and secrets live in `features.toml` beside the policy, and never reach the log, the state's snapshot, the page or the API; a broken version is refused and the server's output names the file and the line, never what the line holds (README.md, Features' settings and secrets; `internal/domain/settings/settingsfile`). Web Push's keys and subscriptions are in `push.json` beside the log, never in it (`internal/domain/notifications/webpush`). A secret that does reach the log is taken out with Redact a text, which leaves no byte of it on disk.

The repo holds no `.env`: `git ls-files` lists none today. `.gitignore` does not name `.env`; a task's worktree gets its repo's untracked `.env` files copied in by `internal/platform/worktrees`, which is how they stay out of git. Local databases (`*.db`, `*.db-wal`, `*.db-shm`) and `bin/` are ignored.

Checked by: `go test ./internal/domain/workers ./internal/domain/connections`, with `TestAMachineJoinsOnceWithItsCode`; `go test ./internal/web`, with `TestAWorkerJoinsAndTakesWork` in `workers_test.go` and `TestASigninCodeSignsInOnce`, `TestASigninCodeExpires` and `TestFailedSigninsAreLimited` in `signin_test.go`; `TestAgentrysOwnFilesAreAlwaysOffLimits` in `internal/domain/policy/default_test.go`; `go test ./internal/domain/settings/settingsfile ./internal/extensions`, with `TestABrokenFileKeepsTheLastGoodVersionAndIsNotQuoted`, `TestAnExtensionReadsOnlyItsOwnTable` and `TestSettingsReachOnlyTheirOwnExtensionsTable`; `TestARedactedSecretIsInNoByteOnDisk` in `internal/testkit/storetest/redact_test.go` and `TestRedactingASecretAcrossTheLogLeavesNoTraceOnDisk` in `internal/domain/settings/settingsapi/redact_log_api_test.go`; and the reviewer, who reads every commit for a token, a key or a `.env` before it is merged, since no check does.

## 5. The UI from one token file, with no shadows, glows or accent stripes

The whole look of the UI is in one token file, `ui/src/styles/globals.css`. In its words: components only spend these variables, so a light theme, or a new palette, is an override block here and never a change to a component; clean and sharp: dark by default in four surface tones, 1px hairlines between panels, nearly square corners, and no shadows, glows or accent stripes anywhere; what is selected is a flat, lighter surface; orange is for the logo, the one main button on a screen and the focus line. Components go `atoms` (spend the design tokens), `molecules`, `organisms` (own state) and one template, `AdminShell`, that every page renders in (README.md, Work on it). The user asked for this: no shadows, no rounded accent stripes on the active item, a 1px focus line, nearly square corners (docs/decisions.md, 2026-10-16).

Current state, from the reviewer's grep run on 2026-10-25 over `ui/src` for `box-shadow`, `text-shadow`, `drop-shadow`, `shadow-`, `border-left` and `border-l-`: no shadow of any kind in any component or stylesheet. A second grep for the word shadow finds only Monaco's `widget.shadow`, set to transparent in `ui/src/lib/monaco.ts` ("no shadows anywhere"). Two left borders, both hairlines in the divider colour, not accent stripes: `border-l border-divider-2` on the side sheet's edge in `ui/src/components/molecules/SideSheet.tsx`, and `border-l-2 border-divider` on a question's quoted context in `ui/src/components/organisms/QuestionsPanel.tsx`.

Checked by: the reviewer, on every UI change, running from the repo root

```sh
grep -rnE 'box-shadow|text-shadow|drop-shadow|shadow-|border-left|border-l-' ui/src --include='*.tsx' --include='*.ts' --include='*.css' --exclude='*.test.tsx' --exclude='*.test.ts'
```

and expecting only the two divider-coloured left borders above (the test files are left out: a test may name a forbidden class to assert it is absent); and reading that a changed component uses token classes and variables, never a colour, radius or font of its own.

## 6. WCAG AA contrast for text and controls

Text reads at 4.5:1 and controls and chips at 3:1 on every surface, in both themes. The token file says how: links and words in a state's colour are made darker in light so they read at 4.5:1 on any surface, under a badge's soft fill too; the keyboard focus line shows at 3:1 on every surface; an agent's avatar letter reads at 3:1 on its tint (`ui/src/styles/globals.css`). The Activity work measured its token pairs and recorded them (docs/design/activity-readability/flows.md: text at least 4.5:1, chips at least 3:1 in both themes; `ink-faint` carries no text that must be read).

Checked by: a reviewer with a contrast checker on the token file's colour pairs, in both themes, for every new pair or changed value. No automated check exists today; the site-wide Badge contrast and focus outline are noted as follow-ups in docs/design/activity-readability.md.

## 7. Plain prose with no em-dashes

README.md, the docs, the package comments and `spec/` are written in plain sentences, in Agentry's own words, with no marketing language and no em-dash (spec/README.md, rule 6; the house style of README.md and docs/decisions.md, which hold none). Current state on 2026-10-25: under `docs/`, one file holds em-dashes, `docs/design/custom-teams/critique.md`; the rest hold none.

Checked by: `spec/check`, which fails on an em-dash anywhere under `spec/`; the reviewer elsewhere, who may grep README.md and docs for the character U+2014 and expect nothing.

## 8. go vet, go test and the UI's checks are green

`go test ./...` passes at the repo root (README.md, Work on it). The Makefile's `test` target builds the page with `make ui` when `internal/web/static/dist/index.html` is missing, then runs `spec/check` and `go vet ./... && go test ./...`, then `cd ui && npm run lint`, which is `tsc --noEmit && prettier --check src` (the UI's type and format checks; README.md, Work on it). CI's check stage runs for every merge request and every commit on `dev`: its job `ui`, on `node:24-bookworm-slim`, runs `npm ci`, `npm run lint` and `npm run build` in `ui`, and its job `check`, on `golang:1.27-bookworm`, takes that page and runs `spec/check`, `go vet ./...` and `go test ./...`, so the UI's tests never skip there (`.gitlab-ci.yml`). The build and deploy stages run only after check passes.

Checked by: `make test` on the branch before it is merged, and the pipeline on `dev`; `make spec` for the spec folder, which story 7 of the spec epic wires into `test` and CI.

## 9. A changed shipped profile is copied to the shipped folder

When a change to `internal/domain/policy/default.toml` changes a profile, it is copied to the next `internal/domain/policy/shipped/NN-vX.Y.Z.toml`, so existing installs can be offered the update with its diff (README.md, The policy, Maintainers and Upgrades).

Checked by: `go test ./internal/domain/policy`, with `TestEveryShippedProfileIsKept` and `TestTheShippedBlocksHoldTheirProfiles`, which fail until the copy is made.

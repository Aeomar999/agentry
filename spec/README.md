# The Agentry spec

This folder is the specification of Agentry as it is today: what it does, for whom, and how well. It is the document AI agents build from and audit against, Agentry's own agents included. The application follows this document. It describes behaviour that exists on dev now, nothing planned, so a scenario that is here is a promise the code already keeps.

## How to read it

Each feature file is a list of scenarios in Given, When, Then form. A scenario names one behaviour: the Given states the conditions or the load, the When is what happens, and every Then is something a person or a program can check, with a number or a yes/no outcome. Read the functional files to learn what Agentry does for each kind of user, and the non-functional files to learn how it holds up: how it fails, what it refuses, what it costs and how it is run.

The glossary gives each noun Agentry uses its one meaning, so the scenarios can use them without explaining them. The constraints say how Agentry is built and must stay built, and how each constraint is checked.

## The folder

| File | What it is about |
| --- | --- |
| `spec/README.md` | this file: the layout, the rules and how to run the check |
| `spec/glossary.md` | every noun Agentry uses, one line each |
| `spec/constraints.md` | how Agentry is built and must stay built, with how each constraint is checked |
| `spec/NOT-YET.md` | behaviour left out of this baseline because it is not built on dev |
| `spec/functional/user.feature` | the user: the person at the page |
| `spec/functional/coordinator.feature` | the coordinator: the agent that reads each report and decides the next step |
| `spec/functional/agent.feature` | the agents that do the work: engineer, reviewer, tester, deployer and the other teams' agents |
| `spec/functional/claude-code.feature` | Agentry used from Claude Code, through its MCP tools |
| `spec/functional/worker.feature` | a machine joined as a worker, where agents run |
| `spec/non-functional/reliability.feature` | what happens when a turn, a process or a machine fails |
| `spec/non-functional/security.feature` | sign-in, credentials, the gate and what no agent may do |
| `spec/non-functional/operations.feature` | running Agentry: the policy file, the settings, the log and its upkeep |
| `spec/non-functional/performance.feature` | measured numbers: how long things take |
| `spec/non-functional/limits.feature` | budgets, turn limits, sizes and caps |
| `spec/non-functional/usability.feature` | the page: keys, panels and what the user sees |
| `spec/non-functional/compatibility.feature` | the systems, engines and browsers Agentry runs on and with |
| `spec/non-functional/observability.feature` | what Agentry shows about itself: tokens, turns, events, the status line |

## IDs

Every scenario is tagged with an ID: the prefix of its file, a dash and a number of two or more digits. Each file has its own prefix, fixed here so that stories written side by side never clash:

| File | ID |
| --- | --- |
| `user.feature` | `@USER-nn` |
| `coordinator.feature` | `@COORD-nn` |
| `agent.feature` | `@AGENT-nn` |
| `claude-code.feature` | `@CC-nn` |
| `worker.feature` | `@WORKER-nn` |
| `reliability.feature` | `@REL-nn` |
| `security.feature` | `@SEC-nn` |
| `operations.feature` | `@OPS-nn` |
| `performance.feature` | `@PERF-nn` |
| `limits.feature` | `@LIM-nn` |
| `usability.feature` | `@USE-nn` |
| `compatibility.feature` | `@COMPAT-nn` |
| `observability.feature` | `@OBS-nn` |

An ID is never renumbered or reused once it is published. A new scenario takes the next number in its file. A scenario that is removed leaves a gap; nothing moves up to fill it. A feature file with a name outside this table is refused by the check.

## The rules for every scenario

Every scenario, with no exceptions:

1. Is tagged with a unique ID, as above.
2. Is tagged with exactly one of `@test`, `@benchmark` and `@manual`:
   - `@test`: an automated test exists for it, named in a comment line directly under the scenario, such as `# internal/policy/default_test.go`.
   - `@benchmark`: a measured number, with where and when it was measured.
   - `@manual`: a person checks it, and the steps they take are the Given, When and Then.
3. Has a Given that states the conditions or the load.
4. Has a number or a yes/no outcome word in every Then, and in every And or But that continues a Then. The outcome words, matched as whole words in any case, are: not, no, never, nothing, cannot, yes, exactly, every, each, once, only, same, empty, refused, denied, allowed, asked, shown, hidden, kept. Any contraction (doesn't, isn't, can't, won't) is one word and counts for nothing: write does not, is not, cannot, will not.
5. Covers what cannot happen too: a refusal, a denial or a cap is a scenario of its own.
6. Is written in plain sentences, in Agentry's own words, as the README and the page use them. No marketing language, no em-dash, and never the words quickly, properly or appropriately.
7. Names where it comes from in a comment line, such as `# README.md, Use it` or `# PLAN.md T-013`, and for `@test` the test file.
8. Names no host, machine name, address or email address: say the machine, the user.

Scenario Outline and Examples tables are not used; write one scenario per case, so every Then is a plain sentence the checker can read.

## Running the check

```sh
spec/check        # from any folder
make spec         # the same, from the repo root
```

`spec/check` runs `go run ./internal/speccheck` from the repo root; it needs Go and nothing else, as the CI image has. An optional argument names another folder to check, relative to the repo root.

It walks every `.feature` file under `spec/` and checks each scenario against rules 1, 2, 4 and 6: the ID tag and its prefix, exactly one check tag, no ID used twice anywhere in the folder, a number or an outcome word in every Then, and none of the three words. It refuses a Scenario Outline and does not count it. It also fails on an em-dash anywhere under `spec/`, in any file, this one included. For rule 7 it checks the citations of each `@test` scenario, as below; the rest of rule 7, and rules 3, 5 and 8, are the reviewer's.

## Citations

The check reads every comment line in a scenario's body, from the `Scenario:` line to the next scenario's tags, and splits it into words, with commas, semicolons, colons, full stops, brackets and quotes cut off their ends, and a line number such as `:42` or `:3-5` cut off the end of a path. A comment line between two scenarios belongs to the one above it.

- A path is a word that starts with one of the repo's top folders and a slash (`cmd/`, `deploy/`, `docs/`, `internal/`, `packaging/`, `spec/`, `tools/`, `ui/`), or is the bare name of a root file (`.dockerignore`, `.gitignore`, `.gitlab-ci.yml`, `Dockerfile`, `LICENSE`, `Makefile`, `NOTICE`, `PLAN.md`, `README.md`, `docker-stack.dev.yml`, `go.mod`, `go.sum`, `spec.md`). It is read from the repo root, the folder above `spec/`. A test fails when the repo root has a folder or a file these lists do not name, so they cannot fall behind it. A short name such as `systemd.go` or `organisms/TopBar.tsx` is not a path and is not checked: write the whole path.
- A word with `*`, `?`, `[`, `{` or `<` in it, such as `ui/src/**/*.tsx`, is a glob or a placeholder. It is skipped, not matched.
- A Go test is a word that starts with `Test` and a capital letter, a digit or an underscore. It belongs to the `_test.go` file named right after it, as in `TestA in internal/x/x_test.go`, or else to the last `_test.go` file cited above it in the same scenario, on the same line or an earlier one. So `internal/x/x_test.go TestA and TestB`, `...; the count: TestC in the same file` and a `TestD` on a later comment line all belong to `internal/x/x_test.go`. A path to a file that is not a Go test, such as `internal/x/x.go`, does not change which file a test belongs to.
- A path under `ui/` is checked for the file only; a test named in it is not looked up.

In a `@test` scenario, the check fails on a path that does not exist (`cited path internal/x/gone_test.go does not exist`), on a Go test that has no `func TestName(` line at the start of a line in its file (`TestA is not defined in internal/x/x_test.go`), and on a Go test with no `_test.go` file before it. A `@manual` or `@benchmark` scenario may cite what it likes. A passing run prints how many paths and Go tests were checked.

Line numbers go stale as dev moves, so a new scenario cites a function or a test by name instead. The check fails on `line N` or `lines N`, in any case, in a comment line of any scenario numbered above the last ID each file had when this rule came in: USER 133, COORD 60, AGENT 56, CC 56, WORKER 41, REL 54, SEC 68, OPS 42, PERF 16, LIM 35, USE 29, COMPAT 27 and OBS 34. These numbers are fixed, in `lastIDBeforeLineRule` in `internal/speccheck/rules.go`, and the scenarios at or below them keep their line numbers. A step is not a citation, so a Then such as "line 1 of the report is the verdict" passes.

A passing run prints the scenarios per file and the totals per check tag, and exits 0:

```
spec/functional/agent.feature: 1 scenario
spec/functional/claude-code.feature: 1 scenario
...
13 files, 13 scenarios: 10 @test, 1 @benchmark, 2 @manual
cited by @test: 12 paths, 9 Go tests
```

A failing run prints the same counts, then each problem as `file:line: message`, and exits 1:

```
spec/functional/user.feature:8: Then has no number and no outcome word; the words are not, no, never, ...
spec/README.md:3: em-dash; write a comma, a colon or a full stop
spec/check: 2 problems
exit status 1
```

The last line is `go run` reporting the exit code.

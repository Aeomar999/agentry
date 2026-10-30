# internal

Agentry's packages, in layers. A package imports only what sits below it.
The full rules are in
[docs/design/repo-organization.md, section 3](../docs/design/repo-organization.md#3-the-dependency-rules);
this page is the short version.

## The layers, from the bottom up

| Layer | Folder | What it holds |
|---|---|---|
| 1 | `platform/*` | The outside world: OS, git, engines, TLS, encodings. No Agentry concept. |
| 2 | `core/events` | What the log holds: the events. |
| 3 | `domain/<area>` | One folder per spec area. Its root package is the model. |
| 4 | `extensions` | The extension framework itself. |
| 5 | `core/httpapi` | How every handler answers. A leaf: it never imports `store`, `current` or `rules`. |
| 5 | `core/store`, `core/current`, `core/rules` | The log (`store`), the read model (`current`) and check 3 (`rules`). |
| 6 | `runtime/*` | What runs: the coordinator loop, turns, the watchdog, machines, the desk. |
| 7 | `domain/<area>/<sub>`, `extensions/<name>` | Surfaces: a domain's routes, tools and background work (such as its `*api` package), and each extension. |
| 8 | `web`, `mcp` | `agentry serve`'s HTTP side and `agentry mcp`. Never each other. |
| 9 | `cmd/*`, `tools/*` | The binary and the programs that build or check the repo. They may import everything. |
| – | `testkit/*` | What tests share across packages. Only tests may import them. |

## The rule

Imports point only down. A few same-layer imports are allowed:

- `platform` packages may import each other.
- Domain models may import each other.
- `store`, `current` and `rules` may import each other.
- `runtime` packages may import each other.
- A surface may import another surface only in its own folder: a domain's
  subpackages, not another domain's, and never another extension.

Tests are exempt: a test may import whatever proves its claim.

## Core sits above the domain models

`core` is not one layer. `core/events` is below the domain models; the rest
of `core` is above them. So `core/store` importing a domain model is right,
not a layer pointing the wrong way.

Why: a model knows only what its events mean, so it needs `core/events` and
nothing else of `core`. It never reads the log or the read model. The log
and the read model must know every model to build state, so they sit above
the models.

## The enforcer

[tools/importrules](../tools/importrules) checks every import against these
layers when `make test` runs. It reads each package's layer off its path. A
package at a path that has no layer fails the test, so a new package must sit
where the layout puts it.

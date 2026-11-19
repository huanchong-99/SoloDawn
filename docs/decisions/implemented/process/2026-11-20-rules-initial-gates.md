# Agent Note: Initial code rules for the SoloDawn rebuild

Status: implemented

## Problem

### Summary

1. Language and roots: mixed Go and TypeScript, `language = "go"` with `extensions = [".go", ".ts"]`; source `cmd`, `internal`, `web/src`; tests `**/*_test.go`, `**/testdata/**`, `internal/host/fakecli`, `web/src/**/*.test.ts`, `web/src/lib/mock.ts`, `web/features`, `web/e2e`, `features`; excluded `**/generated/**`, `**/*.d.ts`, `**/node_modules/**`, `web/dist/**`.
2. Thresholds: package defaults. CRAP 6 (ui 8); duplication 3 percent, blocks of 6 lines; mutation survivors 0.
3. Tiers: generated (`**/generated/**`, exempt); ui (`web/src/routes`, `web/src/components`, CRAP 8, no mutation); core (`internal/proto`, `internal/identity`, `internal/store`, `internal/home`, CRAP 6, mutation); standard (`cmd`, `internal`, `web/src`, CRAP 6, mutation).
4. Modules: 10 modules (cmd, node, proto, identity, store, peer, rendezvous, home, host, web), features under `features/`; 10 features missing, none of them debt yet because no module has code.
5. Dependency rules: each Go package's allowed internal imports, written as the layer order `cmd` > `internal/node` > `internal/host` > `internal/home` > `internal/rendezvous` > `internal/peer` > `internal/store` > `internal/identity` > `internal/proto` plus 15 forbidden downward edges; cycles forbidden. The web client's imports are not checked by the built-in engine under these rules.
6. Protected paths and directives: template defaults without `.github/workflows/**`; Go and TypeScript directive lists merged, plus `svelte-ignore`.
7. Expected debt from `audit.py` (dry run, 2026-11-20, writes suppressed): 0 functions over CRAP, 0 duplicate blocks, 0 dependency violations, 0 modules without a feature, 0 failing fast commands; 0 source files exist.

The repository holds the rebuild plan (`planning/`) and no code yet. Code will arrive as pull requests into `rebuild`: the Go backend from a cloud implementer, the Svelte web client from local implementers. Every pull request is checked out locally and must pass `gate.py --stage merge` before it is merged. The gates therefore need one rules file that covers both languages, works before either half exists, and uses as few tools as possible.

The package supports one language, one coverage report and one mutation engine per rules file. A mixed Go and TypeScript repository has to fold two toolchains into those single slots.

## Decision

The rules in `.project-standard/gates.toml` are the project's code rules. They were drafted by the adopting agent from `planning/architecture.md` section 8, `planning/steps/step-1-backend.md` and `planning/steps/step-1-frontend.md`, under the coordinator's instruction to use the fewest and lightest tools.

Approval: approved on 2026-11-20 by the coordinator (the main session), acting under the owner's written authorization that delegates to the coordinator every decision that needs the owner's assent. The coordinator reviewed the `gates.toml` draft and this proposal before approving; the owner did not approve it in person. The rules were approved as drafted, without changes. `Code rules` in the project record reads `gates.toml` from the same date.

### `[project]`

- `language = "go"`: the setting only selects the built-in dependency engine and default comment prefixes. The Go side has the declared layering (eight internal packages), so the built-in engine checks Go.
- `extensions = [".go", ".ts"]`: both languages are measured. `.svelte` is left out because lizard cannot parse it (checked: lizard 1.24.1 reports no functions for a `.svelte` file). Logic therefore belongs in `.ts` and `.svelte.ts` modules, where CRAP and mutation reach it; this matches the plan's rule that the web client renders rows and holds no group logic.
- `source = ["cmd", "internal", "web/src"]`: the production roots of architecture section 8 and the frontend structure. Spike code (M0) placed outside these roots is not measured, which suits throwaway code.
- `tests`: Go tests and golden files; the fake CLIs `internal/host/fakecli/claude` and `internal/host/fakecli/codex` (main packages that stand in for the real CLIs in the host tests, B-10); web unit tests; the mock transport (`web/src/lib/mock.ts`, development-only, fed by golden files); the web step files; the end-to-end suite; the acceptance features. Test paths carry no CRAP, coverage or mutation obligation. The fake CLIs still belong to the `host` module by path, so the host README and feature cover them.
- `exclude`: generated code, type declarations, dependencies and build output.
- The built web client is written to a git-ignored directory under `internal/node`, from which the binary embeds it. The scripts list files with `git ls-files --cached --others --exclude-standard`, so a git-ignored directory is invisible to every gate without a further rule.

### `[thresholds]` and `[[tier]]`

Thresholds are the package defaults. Tiers: `core` is the trust, persistence and decision code (wire structs, envelopes and device logs, SQLite, the home's command handling and task state machine); `standard` is everything else in the source roots with the same limits; `ui` is the web client's routes and components (only their `.ts` files are measured), CRAP 8 and no mutation, as the TypeScript template does; `generated` is exempt.

### `[[module]]`

One module per directory of architecture section 8, plus `node` (`internal/node`), the assembly layer confirmed by the planning lead: it wires the other packages into the `node` and `rendezvous` subcommands, while `cmd` only parses arguments. Each module has `features/<name>.feature` and a README beside the code (`cmd/solodawn/README.md`, `internal/<name>/README.md`, `web/README.md`). A declared module owes its feature from the session that writes its first code.

### `[stages]`

Every command is a no-op while its half of the repository does not exist (no `go.mod`, no `web/package.json`), so a session that only changes documents can finish before B-00 and F-00 land.

| Order | Command | Category | Tool |
|---|---|---|---|
| 1 | `gofmt -l cmd internal features`, empty output required | format | gofmt (ships with Go) |
| 2 | `go vet ./...` | lint, types | go vet (ships with Go) |
| 3 | `go test -count=1 -covermode=count -coverprofile=... ./...` | unit tests, coverage | go test (ships with Go) |
| 4 | `prettier --check .` in `web/` | format | prettier with prettier-plugin-svelte |
| 5 | `svelte-check --fail-on-warnings` in `web/` | types, lint | svelte-check (Svelte compiler warnings including accessibility, and TypeScript) |
| 6 | `vitest run` with v8 coverage, LCOV, in `web/` | unit tests, coverage | vitest with @vitest/coverage-v8 |
| 7 | merge into `.project-standard/state/coverage.lcov` | coverage report | awk and sed |

The scripts read one coverage report. Command 7 converts the Go cover profile to LCOV (one `SF` block per profile line, every line of the block with its count) and appends the web LCOV with its paths prefixed by `web/`. Checked on a synthetic profile: the merged file parses to the same per-line counts as the package's own Go profile reader.

Nightly: full gremlins and full Stryker runs; they never block.

### `[mutation]`

`engine = "command"`, because one rules file has one engine and the gremlins engine would hand TypeScript directories to gremlins. The command takes the task branch's merge-base with `rebuild` (every pull request targets it), runs `gremlins unleash --diff <base> --exclude-files "internal/host/fakecli/" --threshold-efficacy 100 --threshold-mcover 100 .` when Go files outside tests, test data and the fake CLIs changed (gremlins exits 10 or 11 below a threshold; NOT COVERED counts against the mutant-coverage threshold), and runs `stryker run --mutate <changed files>` in `web/` for added or modified `.ts` files outside tests, routes, components and the mock. `web/stryker.config.json` (created with F-00) sets the vitest runner and `thresholds.break = 100`. Zero survivors is the default limit.

### `[dup]`

Built-in line-window detector, any language, no dependency.

### `[deps]`

Built-in Go package graph. The planning lead confirmed which internal packages each package may import:

| Package | May import |
|---|---|
| `internal/proto` | nothing internal |
| `internal/identity` | proto |
| `internal/store` | proto |
| `internal/peer` | proto, identity, store |
| `internal/rendezvous` | proto, identity |
| `internal/home` | proto, identity, store |
| `internal/host` | proto, identity, store |
| `internal/node` | all of the above |
| `cmd` | node |

The engine has no allow-list. The table is expressed as the layer order, top to bottom, `cmd`, `internal/node`, `internal/host`, `internal/home`, `internal/rendezvous`, `internal/peer`, `internal/store`, `internal/identity`, `internal/proto` (an earlier entry may import later ones, never the reverse), plus the downward edges the table does not allow:

- `internal/host` to `internal/home`, `internal/rendezvous` and `internal/peer`: a host reaches homes and peers only through what `node` hands it; a group homed on the same node is wired in `node` (architecture section 2.1).
- `internal/home` to `internal/rendezvous` and `internal/peer`: the home's command handling is pure, tested without a network.
- `internal/rendezvous` to `internal/peer` and `internal/store`: the rendezvous is a stateless server and holds no database (architecture section 2.2, C-7).
- `internal/store` to `internal/identity`: store depends on proto only.
- `cmd` to every internal package except `internal/node`: `cmd` only parses arguments.

The rule for `store -> identity` and the seven `cmd` edges are not in the planning lead's list of forbidden edges; they follow from the allow-list. Checked by enumerating every ordered pair of the nine packages and of their subpackages against the table: the layers and forbidden edges allow exactly the listed imports and reject every other one. A subpackage counts as its parent (`internal/host/driver` is `internal/host`). Imports inside one package's subtree are allowed. Go itself rejects import cycles; `no_cycles` stays on as the package default.

### `[acceptance]`

Go module features run with godog under `go test -tags acceptance ./features/...` (the build tag keeps them out of the fast stage); the web feature runs under vitest with `@amiceli/vitest-cucumber`. Neither is a separate runner program: godog is a Go test dependency, vitest-cucumber a vitest plug-in.

### `[tamper]`

Template defaults minus `.github/workflows/**`: B-00 writes the CI workflow, and a protected path fails the tamper gate on the pull request that creates it; the local merge gate, not CI, is the enforcement point. Directives: the Go and TypeScript template lists, plus `svelte-ignore`, because `svelte-check --fail-on-warnings` is the web lint.

### Dependencies the gates need

| Where | Dependency | Added by |
|---|---|---|
| machine | Go toolchain | owner's installation |
| machine | gremlins | owner's installation |
| `go.mod`, test only | `github.com/cucumber/godog` | first Go acceptance feature |
| `web/package.json`, dev | prettier, prettier-plugin-svelte, svelte-check, vitest, @vitest/coverage-v8, @stryker-mutator/core, @stryker-mutator/vitest-runner, @amiceli/vitest-cucumber | F-00 |
| `web/.gitignore` | `node_modules/`, `dist/`, `.stryker-tmp/`, `reports/` | F-00 |

The hooks treat every untracked, unignored file as a change, so the tools' output directories must be ignored before the first web run.

### Session records

The session records under `docs/tasks/` stay on the developer's machine and are never committed; the structure checks skip them through a format exception in the project record. Rule proposals and other decision records under `docs/decisions/` are committed as the standard intends.

## Alternatives considered

**`language = "typescript"`.** It would check the web client's imports instead of the Go packages. The Go side carries the declared layering and the trust boundaries, so Go was preferred.

**The `gremlins` or `stryker` engine.** Either built-in engine covers one language only; the gremlins engine would also pass TypeScript directories to gremlins and fail.

**A `[deps] command` that runs both graphs.** It would need a script outside the package or a long inline program that calls the package's internals; left out for least code. dependency-cruiser for the web client is a further tool.

**ESLint with typescript-eslint and eslint-plugin-svelte.** A fuller linter, at the cost of four to five packages and a configuration. svelte-check with `--fail-on-warnings` plus strict TypeScript compiler options covers type errors, unused code and Svelte's accessibility warnings with a tool the project needs anyway.

**Biome in place of prettier.** One binary for format and lint, but partial Svelte template support; prettier with its Svelte plug-in is the established formatter for Svelte.

**staticcheck or golangci-lint.** More Go checks than `go vet`, as an extra installation. Can be added by a later proposal if `go vet` proves too weak.

**cucumber-js for all features.** One runner, but Go module scenarios would have to drive Go code from JavaScript; godog runs inside `go test`.

**Package-level mutation from the session diff file.** Reading `.project-standard/state/session.diff` would include uncommitted files, but needs more shell and mutates whole packages; the merge-base form is shorter and matches the pull-request flow.

## Consequences

The gates cover both halves of the repository with one rules file, before either half exists, using the Go toolchain, gremlins and a handful of web dev dependencies. The Stop hook enforces them from the approval date. What this costs, and what stays open:

- Unverified until code exists: the SF paths that vitest writes into LCOV (relative to `web/` is assumed; absolute paths pass through unchanged); whether gremlins exits non-zero when a Go diff yields no mutants; whether `go test ./...` silently skips the tag-only `features` package; how long gremlins runs on the host package with its fake CLIs. Each is checked on the first pull request that brings the code.
- Mutation measures committed changes only; uncommitted work is not mutated until it is committed on the task branch.
- The command engine ignores `[mutation] ignore`; an equivalent mutant needs a tool-level exclusion approved as a directive exception (`Stryker disable`) or a gremlins file exclusion.
- Logic written inside `.svelte` files escapes CRAP, mutation and duplication; review and the ui tier's thin-component rule carry it.
- The web client's import graph is unchecked.
- The package's directive scan reads `.ts`, `.json` and similar files but not `.go` or `.svelte` (`.project-standard/scripts/tamper.py`, directive extensions), so the Go directives and `svelte-ignore` are listed but not enforced by the tamper gate.
- A new internal package that is not in the layer list is not constrained by any dependency rule until a proposal adds it; the engine cannot deny by default.
- The binary embeds the built web client from a git-ignored directory under `internal/node`. If that directory is empty when `go vet` or `go test` runs, the embed pattern fails to compile; the build order or a tracked placeholder must handle this when B-17 lands. The directory's ignore rule must sit in a tracked ignore file.
- `--exclude-files` of gremlins is a regular expression on the file path; its exact path form is checked when the first fake CLI lands.
- The checks that can only run once code exists are still owed: after B-00 and F-00 merge, the fast stage must write `coverage.lcov` with entries for both halves, `gate.py --stage merge` must pass on those pull requests, and a deliberately surviving mutant in each language must fail the harden stage.

## Verification

Observed on 2026-11-20, before approval, on a tree with no source files: the dry run of `python3 .project-standard/scripts/audit.py` (writes suppressed) reported zero debt (0 source files, 0 items); `python3 .project-standard/scripts/check.py` reported zero violations; with no `go.mod` and no `web/package.json`, every fast command, the acceptance command and the mutation command exited 0. The post-approval audit and structure check are run when `Code rules` is set to `gates.toml`. Nothing about the gates' behaviour on real Go or TypeScript code is verified yet; see the first item under Consequences.

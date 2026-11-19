# Agent Note: Go mutation verdict read from the gremlins output

Status: implemented

## Problem

The Go half of the mutation gate never failed. The harden command and the nightly command ran `gremlins unleash ... --threshold-efficacy 100 --threshold-mcover 100` and took the exit code as the verdict, as `2026-11-20-rules-initial-gates.md` intended. Reviewing pull request #52 showed otherwise: with gremlins v0.6.0 (installed with `go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0`; it reports its version as `dev`), a run with 0.00 percent test efficacy exited 0. Two mutants of `cmd/solodawn/main.go` that the tests do kill were also reported as LIVED, or as TIMED OUT, instead of KILLED.

Three causes, read in the gremlins v0.6.0 source (tag `v0.6.0` of github.com/go-gremlins/gremlins) and in viper v1.21.0, the configuration library it is built with:

1. **The threshold flags are dropped.** Both flags are declared as `float64` (`cmd/unleash.go:217-218`) and bound to viper. For a flag set on the command line, viper's `find` converts int, bool and slice flags and returns every other type as the flag's string (`viper.go:1176-1219`, `return flag.ValueString()`). gremlins reads the value with `configuration.Get[float64]` and falls back to `Get[int]` (`internal/report/report.go:233-242`); `Get` is a plain type assertion that yields 0 on a mismatch (`internal/configuration/configuration.go:186`). The string `"100"` is neither, so both thresholds read as 0, and `assess` checks a threshold only when it is above 0 (`report.go:237`, `report.go:244`). An environment variable (`GREMLINS_UNLEASH_THRESHOLD_EFFICACY`) also reaches viper as a string. Only a YAML configuration file yields a number.
2. **A threshold of 100 cannot pass.** `assess` fails when the value is at or below the threshold (`tEfficacy <= et`, `report.go:237`; the same for coverage at `report.go:244`). With a configuration file setting `efficacy: 100`, a run with 100.00 percent efficacy exits 10 (`ERROR: below efficacy-threshold`), observed on 2026-11-20. Efficacy is also 0 when no mutant is killed or lived (`report.go:93`), so a run whose mutants are all TIMED OUT or NOT VIABLE would fail a lower threshold as well.
3. **The default mode tests the wrong package and uses a cached baseline.**
   - Without `-i` (integration mode), each mutant runs `go test <pkg>` (`internal/engine/executor.go:235`), where `pkg` is found by walking up from the file's directory to a directory whose name ends with the Go package name (`internal/engine/engine.go:160-180`). For `cmd/solodawn/main.go`, package `main`, no directory matches, so `pkg` becomes the module root (`engine.go:172`), and the tests run are the root package's `imports_test.go`, which the mutant cannot fail: LIVED.
   - The per-mutant timeout is the coverage run's duration times the coefficient, default 3 (`executor.go:101`). The coverage run is a plain `go test -cover ./...` (`internal/coverage/coverage.go:153`), which Go's test cache answers in milliseconds on a second run (86 ms observed), so a mutant whose package must be rebuilt hits the deadline: TIMED OUT (`executor.go:210`).

## Decision

Approval: approved on 2026-11-20 by the coordinator (the main session), acting under the owner's written authorization that delegates to the coordinator every decision that needs the owner's assent. The owner did not approve it in person. The fix was drafted and verified by a subagent at the coordinator's request, with the instruction to prefer gremlins' own correct usage and to fall back to a short shell check of the output, without a script file.

gremlins v0.6.0 has no correct built-in form for "zero surviving mutants": the command-line thresholds are ignored (cause 1) and a configuration-file threshold of 100 fails a perfect run (cause 2). The Go part of the harden command and of the nightly command therefore judges the output:

```sh
o=$(GOFLAGS=-count=1 gremlins unleash -i --timeout-coefficient 10 [--diff "$b"] --exclude-files "internal/host/fakecli/" . 2>&1); s=$?
printf "%s\n" "$o"
[ $s -eq 0 ] && ! printf "%s\n" "$o" | grep -Eq "^ *(LIVED|NOT COVERED) "
```

- The gate fails when gremlins itself exits non-zero (not a module, coverage run failed) or when any mutant line reads LIVED or NOT COVERED. KILLED, TIMED OUT, NOT VIABLE and SKIPPED pass, as gremlins counts them. gremlins prints colour only to a terminal, and the captured output is not one, so the status words are plain text.
- `-i` runs `go test ./...` from the module root for every mutant, so a mutant in any package, `package main` included, is tested by the whole suite.
- `GOFLAGS=-count=1` makes the coverage run execute the tests instead of reading the test cache, so the timeout baseline is a real run; `--timeout-coefficient 10` (an int flag, which viper does convert) leaves room for rebuilding the mutated package and its dependents.
- The `--threshold-*` flags are removed: they have no effect today, and if a later gremlins release honoured them, a value of 100 would fail every perfect run.
- The `[mutation]` comment in `gates.toml` states the new verdict. The web half (Stryker, `thresholds.break = 100`) is unchanged.

### `gates.toml` change

Before:

```toml
nightly = [
  '[ ! -f go.mod ] || gremlins unleash --exclude-files "internal/host/fakecli/" --threshold-efficacy 100 --threshold-mcover 100 .',
  ...
]
command = 'b=$(git merge-base HEAD rebuild) && { git diff --quiet "$b" HEAD -- "*.go" ":!*_test.go" ":!**/testdata/**" ":!internal/host/fakecli/**" || gremlins unleash --diff "$b" --exclude-files "internal/host/fakecli/" --threshold-efficacy 100 --threshold-mcover 100 .; } && f=$(...web part unchanged...)'
```

After:

```toml
nightly = [
  '[ ! -f go.mod ] || { o=$(GOFLAGS=-count=1 gremlins unleash -i --timeout-coefficient 10 --exclude-files "internal/host/fakecli/" . 2>&1); s=$?; printf "%s\n" "$o"; [ $s -eq 0 ] && ! printf "%s\n" "$o" | grep -Eq "^ *(LIVED|NOT COVERED) "; }',
  ...
]
command = 'b=$(git merge-base HEAD rebuild) && { git diff --quiet "$b" HEAD -- "*.go" ":!*_test.go" ":!**/testdata/**" ":!internal/host/fakecli/**" || { o=$(GOFLAGS=-count=1 gremlins unleash -i --timeout-coefficient 10 --diff "$b" --exclude-files "internal/host/fakecli/" . 2>&1); s=$?; printf "%s\n" "$o"; [ $s -eq 0 ] && ! printf "%s\n" "$o" | grep -Eq "^ *(LIVED|NOT COVERED) "; }; } && f=$(...web part unchanged...)'
```

## Alternatives considered

**A `.gremlins.yaml` with the thresholds.** The only input path where gremlins reads a number, but 100 fails a perfect run (cause 2), so the file would need 99.99 and would still fail a run whose mutants are all TIMED OUT or NOT VIABLE. It also adds a tracked configuration file for a verdict that one grep gives.

**The JSON report (`--output`) parsed by a tool.** Exact counts, but needs `jq` or an inline Python program and a temporary file; the text output already carries one line per mutant.

**Non-integration mode with a larger timeout.** Faster, but still tests the module root for every `package main` outside a directory named `main` (cause 3), so `cmd/solodawn` mutants live regardless of the tests.

**Counting TIMED OUT as a failure.** Stricter against a timeout that is not a real hang, but a mutant that turns a loop infinite (for example `i++` to `i--`) times out by design and could then never pass; the command engine has no `ignore` list to approve it. The real coverage baseline and the coefficient of 10 address false timeouts instead.

**A different gremlins version.** Not tried: changing the machine's installed tool needs the owner, and the flag parsing and the `<=` comparison are both in the v0.6.0 tag.

## Consequences

- The mutation gate now fails on a surviving or uncovered Go mutant, and a branch whose Go diff yields no mutants still passes.
- Every mutant runs the whole Go test suite, without the test cache. On the scaffold (two mutants) a harden run takes under two seconds after the build cache is warm; the cost grows with mutants times suite time, which matters most for the nightly full run and for large diffs. The `[mutation] timeout` of 1800 seconds still bounds the harden run.
- A mutant that hangs is given ten times the coverage run's duration before it counts as TIMED OUT; a slow machine or a loaded one may still produce a false TIMED OUT, which passes.
- The verdict depends on gremlins' text format (`LIVED`, `NOT COVERED` at the start of a mutant line). A gremlins upgrade must recheck it with the three cases under Verification.
- Acceptance features (build tag `acceptance`) do not run per mutant; the unit tests must kill the mutants on their own.
- The fact in `2026-11-20-rules-initial-gates.md` that gremlins exits 10 or 11 below the thresholds no longer holds for the command line; that record keeps the original choice, and this record owns the Go verdict from this date.

## Verification

Observed on 2026-11-20 in a separate worktree of `origin/rebuild` at `7d47f09e5` (pull request #52 merged: `cmd/solodawn`, `internal/node`), with gremlins v0.6.0 and Go 1.27.4. Each command was read from `gates.toml` with the package's own parser and run through the package's command runner (`.project-standard/scripts/common.py`, `run`), as the command engine runs it.

- Before the change, the old harden command's gremlins call on the same tree: `Killed: 0, Lived: 2`, `Test efficacy: 0.00%`, exit 0.
- Mutants killed: harden command, base `5ab399db5` (the merge-base with the local `rebuild` branch), Go diff `cmd/solodawn/main.go` and `internal/node/node.go`: `Killed: 2, Lived: 0`, exit 0, pass. Nightly command on the same tree: `Killed: 2`, pass.
- Mutant surviving: the `t.Errorf` assertion in `cmd/solodawn/main_test.go` commented out, uncommitted: `LIVED CONDITIONALS_NEGATION at cmd/solodawn/main.go:16:31`, `Killed: 1, Lived: 1`, exit 1, fail; nightly also fails. Restored with `git checkout`.
- Mutant not covered: an untested function added to `internal/node/node.go` in a temporary commit: `NOT COVERED CONDITIONALS_NEGATION at internal/node/node.go:6:38`, exit 1, fail. Commit discarded.
- No Go mutants: a temporary commit changing only `issues.md` and a comment in a test file, base `origin/rebuild`: the Go part is skipped, exit 0, pass. A temporary commit changing only a comment in `internal/node/node.go`: gremlins reports both mutants as SKIPPED (outside the diff), `Killed: 0, Lived: 0`, exit 0, pass. Commits discarded.
- With a `.gremlins.yaml` setting `unleash.threshold.efficacy: 100`, a run with 100.00 percent efficacy exits 10 (cause 2).

Not verified: the gate's behaviour with the Stryker half and the Go half in one branch, and run times on a larger code base.

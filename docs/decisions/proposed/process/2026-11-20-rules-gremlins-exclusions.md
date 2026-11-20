# Agent Note: Exclude foreign-platform and test files from the gremlins run

Status: proposed

## Problem

The Go part of the mutation gate (`[mutation] command` for the harden stage, the first `[stages] nightly` command) runs `gremlins unleash` from the module root, as set by `2026-11-20-rules-go-mutation.md` in `docs/decisions/implemented/process/`. Two kinds of file reach gremlins that the gate cannot judge on the machine it runs on.

**Files built for another platform.** gremlins v0.6.0 does not read build constraints. Its engine walks every `.go` file under the module root that is not a `_test.go` file (`internal/engine/engine.go:104-108` at tag `v0.6.0` of github.com/go-gremlins/gremlins) and gives a mutant the status NOT COVERED when the coverage profile has no block for its position (`engine.go:188-197`, `internal/coverage/profile.go:27-31`). The profile comes from `go test -cover` on the host (`internal/coverage/coverage.go:145-153`), which compiles only the files built for the host's GOOS and GOARCH. Every mutant in a Windows-only file is therefore NOT COVERED on macOS or Linux, whatever its tests do, and the gate fails. The first case is pull request #107 (branch `devin/N-0-local-cli`), which adds `internal/node/socket_windows.go`: the harden command reports `NOT COVERED CONDITIONALS_NEGATION at internal/node/socket_windows.go:24:9` and exits 1 (line 22 in an earlier revision of the branch). HS-19's `pty_windows.go` will be the next. The same applies to any file whose file-name suffix or `//go:build` line excludes the host, for example a `_linux.go` file on the Mac.

**Test paths.** `gates.toml` classifies `features`, `**/testdata/**` and `internal/host/fakecli` as tests (`[project] tests`), which carry no mutation obligation (`2026-11-20-rules-initial-gates.md` in `docs/decisions/implemented/process/`). The gremlins command excludes only `internal/host/fakecli/`. Non-test Go files under `features/`, such as the network simulation helpers in `features/netsim/` (pull request #101, build tag `netsim`, Linux only), are walked and mutated; the nightly run on `rebuild` reports them NOT COVERED (see Verification). `testdata` holds no Go file today, but gremlins would walk one as well.

## Proposal

### Summary
1. Language and roots: unchanged.
2. Thresholds: unchanged; zero surviving or uncovered mutants.
3. Tiers: unchanged.
4. Modules: unchanged.
5. Dependency rules: unchanged.
6. Protected paths and directives: unchanged. Only the Go part of `[stages] nightly` and `[mutation] command` changes, plus the `[mutation]` comment.
7. Expected debt from `audit.py`: unchanged; the change removes false NOT COVERED failures and creates no debt item.

Origin: the coordinator (the main session) asked for this proposal on 2026-11-20 after pull request #107 hit the failure, with the direction to exclude every Go file not built for the host and the whole `features/` tree, to prefer a rule derived from `go list` over a hand-kept list, and to state the coverage gap honestly. This record was drafted and verified by a subagent. It is not approved; approval is the owner's, or the coordinator's under the owner's written authorization, as for the earlier rules records.

### The rule

The Go part of both commands computes the exclusions before it runs gremlins:

1. `go list -e ./...` on the host lists every file the host build compiles (`GoFiles` and `CgoFiles`, as paths relative to the module root).
2. The same listing with `CGO_ENABLED=0` for each shipped platform, `linux`, `darwin` and `windows` on `amd64` and `arm64` (the cross-compile matrix of `.github/workflows/backend.yml` and the platforms of `planning/architecture.md`), lists every file some shipped build compiles.
3. A file in the second set and not in the first is built for another platform only. Each such file is excluded by an anchored regular expression (`^(internal/node/socket_windows[.]go)$`), and the command prints the list (`gremlins: not built for this host, not mutated here: ...`, or `none`).
4. The test roots are excluded by one more expression, `^(features/|internal/host/fakecli/|(.*/)?testdata/)`, so the gremlins run matches `[project] tests` for Go. `_test.go` files are skipped by gremlins itself.
5. The pre-check that skips gremlins when no Go source changed also ignores `features/**`.

What the rule does not exclude, on purpose: a file behind a custom build tag (`//go:build debug`, `//go:build ignore`) outside the test roots is built by no shipped platform without that tag, so it stays in the walk and still fails as NOT COVERED. Production code cannot leave the mutation gate by moving behind a tag; it needs a rules proposal.

### How `--exclude-files` behaves in gremlins v0.6.0

Read in the source and confirmed by the runs under Verification:

- The flag (`-E`) is a string slice (`cmd/unleash.go:216`). It can be given more than once; viper converts slice flags given on the command line, unlike the float thresholds of the earlier record. Each value is a Go regular expression compiled with `regexp.Compile` (`internal/exclusion/rules.go:22-31`); an invalid one makes gremlins exit with an error, which fails the gate.
- A file is excluded when any expression matches its path, unanchored (`MatchString`, `rules.go:37-48`). The path is the one `fs.WalkDir` reports under the module root: slash-separated, relative, no `./` prefix (`engine.go:72`, `engine.go:104`). The proposed expressions are anchored with `^` so that a directory name cannot match deeper in another path; dots are written `[.]`.
- pflag splits a slice value at commas, so a value must not contain one. A Go file name with a comma would split the expression into invalid halves and fail the gate rather than exclude the wrong files.

### Coverage gap

Until a mutation run exists on a Windows machine, the lines of Windows-only files have no mutation coverage. They are still compiled by the cross-compile job in CI and tested by their own `_windows_test.go` files on Windows, where such tests run; neither checks that those tests kill mutants. The Windows run is a separate, later piece of work: the same command run on Windows (it needs a POSIX shell, for example Git Bash) would exclude the macOS- and Linux-only files and mutate the Windows ones. On a Linux host, `darwin`-only files are excluded in the same way; on the Mac, `linux`-only ones. Today no such file exists outside `features/` on `rebuild`; on branch `devin/N-0-local-cli` the only excluded file is `internal/node/socket_windows.go`.

### `gates.toml` change

The full diff, for the coordinator to apply to `.project-standard/gates.toml` after approval (the file is git-excluded and protected, so it is not part of this branch's diff):

```diff
@@ -126,7 +126,7 @@
 ]
 coverage = ".project-standard/state/coverage.lcov"
 nightly = [
-  '[ ! -f go.mod ] || { o=$(GOFLAGS=-count=1 gremlins unleash -i --timeout-coefficient 10 --exclude-files "internal/host/fakecli/" . 2>&1); s=$?; printf "%s\n" "$o"; [ $s -eq 0 ] && ! printf "%s\n" "$o" | grep -Eq "^ *(LIVED|NOT COVERED) "; }',
+  '[ ! -f go.mod ] || { t="{{\$d := slice .Dir (len .Module.Dir)}}{{range .GoFiles}}{{\$d}}/{{.}}{{println}}{{end}}{{range .CgoFiles}}{{\$d}}/{{.}}{{println}}{{end}}" && h=$(echo /; go list -e -f "$t" ./...) && a=$(for p in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do CGO_ENABLED=0 GOOS=${p%/*} GOARCH=${p#*/} go list -e -f "$t" ./... || exit 1; done) && x=$(printf "%s\n" "$a" | sort -u | grep -vxF "$h" | sed "s#^/##;s#[.]#[.]#g" | paste -sd "|" -) && echo "gremlins: not built for this host, not mutated here: ${x:-none}" && set -- --exclude-files "^(features/|internal/host/fakecli/|(.*/)?testdata/)" && { [ -z "$x" ] || set -- "$@" --exclude-files "^($x)\$"; } && { o=$(GOFLAGS=-count=1 gremlins unleash -i --timeout-coefficient 10 "$@" . 2>&1); s=$?; printf "%s\n" "$o"; [ $s -eq 0 ] && ! printf "%s\n" "$o" | grep -Eq "^ *(LIVED|NOT COVERED) "; }; }',
   '[ ! -f web/package.json ] || (cd web && npx stryker run)',
 ]
 
@@ -136,11 +136,14 @@
 # --threshold-* given on the command line, so the command fails on gremlins' own error exit or on any LIVED or NOT COVERED
 # line; -i runs the whole suite per mutant and GOFLAGS=-count=1 keeps the timeout baseline real (see
 # docs/decisions/implemented/process/2026-11-20-rules-go-mutation.md). web/stryker.config.json sets thresholds.break = 100.
-# UI paths and test files are not mutated.
+# UI paths and test files are not mutated. gremlins ignores build constraints, so the Go part excludes every file that
+# go list builds for a shipped platform (linux, darwin, windows on amd64 and arm64) but not for this host, and the Go
+# test roots (features/, internal/host/fakecli/, testdata/); those files are not mutated in this run (see
+# docs/decisions/implemented/process/2026-11-20-rules-gremlins-exclusions.md).
 engine = "command"
 test_command = ""
 timeout = 1800
-command = 'b=$(git merge-base HEAD rebuild) && { git diff --quiet "$b" HEAD -- "*.go" ":!*_test.go" ":!**/testdata/**" ":!internal/host/fakecli/**" || { o=$(GOFLAGS=-count=1 gremlins unleash -i --timeout-coefficient 10 --diff "$b" --exclude-files "internal/host/fakecli/" . 2>&1); s=$?; printf "%s\n" "$o"; [ $s -eq 0 ] && ! printf "%s\n" "$o" | grep -Eq "^ *(LIVED|NOT COVERED) "; }; } && f=$(git diff --name-only --diff-filter=AM "$b" HEAD -- "web/src/*.ts" | grep -v -e "\.test\.ts$" -e "^web/src/routes/" -e "^web/src/components/" -e "^web/src/lib/mock\.ts$" | sed "s#^web/##" | paste -sd, -) && { [ -z "$f" ] || (cd web && npx stryker run --mutate "$f"); }'
+command = 'b=$(git merge-base HEAD rebuild) && { git diff --quiet "$b" HEAD -- "*.go" ":!*_test.go" ":!**/testdata/**" ":!internal/host/fakecli/**" ":!features/**" || { t="{{\$d := slice .Dir (len .Module.Dir)}}{{range .GoFiles}}{{\$d}}/{{.}}{{println}}{{end}}{{range .CgoFiles}}{{\$d}}/{{.}}{{println}}{{end}}" && h=$(echo /; go list -e -f "$t" ./...) && a=$(for p in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do CGO_ENABLED=0 GOOS=${p%/*} GOARCH=${p#*/} go list -e -f "$t" ./... || exit 1; done) && x=$(printf "%s\n" "$a" | sort -u | grep -vxF "$h" | sed "s#^/##;s#[.]#[.]#g" | paste -sd "|" -) && echo "gremlins: not built for this host, not mutated here: ${x:-none}" && set -- --exclude-files "^(features/|internal/host/fakecli/|(.*/)?testdata/)" && { [ -z "$x" ] || set -- "$@" --exclude-files "^($x)\$"; } && { o=$(GOFLAGS=-count=1 gremlins unleash -i --timeout-coefficient 10 --diff "$b" "$@" . 2>&1); s=$?; printf "%s\n" "$o"; [ $s -eq 0 ] && ! printf "%s\n" "$o" | grep -Eq "^ *(LIVED|NOT COVERED) "; }; }; } && f=$(git diff --name-only --diff-filter=AM "$b" HEAD -- "web/src/*.ts" | grep -v -e "\.test\.ts$" -e "^web/src/routes/" -e "^web/src/components/" -e "^web/src/lib/mock\.ts$" | sed "s#^web/##" | paste -sd, -) && { [ -z "$f" ] || (cd web && npx stryker run --mutate "$f"); }'
 ignore = []
 
 [dup]
```

Notes on the shell, which `common.run` hands to `/bin/sh`:

- The template prints each file as `<directory relative to the module root>/<file>`, so the root package's files start with `/` like the others; `sed` drops the leading slash. `h` starts with the line `/`, which matches no file, so that `grep -vxF` never receives an empty pattern list (an empty pattern would match every line and exclude nothing silently).
- A failing `go list` stops the chain, and the gate fails rather than run gremlins without the exclusions.
- `set --` holds the `--exclude-files` arguments as the positional parameters; the commands use no other positional parameter. The construction is POSIX and was run under macOS `/bin/sh` and under `dash`.
- The scripts under `.project-standard/scripts/` need no change: the command engine runs the string and takes its exit code (`mutation.py`, `engine == "command"`).

### `PROJECT.md` change

On approval, the `Rules approved on` row gains, after the go-mutation entry: `; changed 2026-11-20 (gremlins exclusions), by <approver>; record: docs/decisions/implemented/process/2026-11-20-rules-gremlins-exclusions.md`.

## Alternatives considered

**A hand-kept list or a file-name pattern** (`--exclude-files "_windows[.]go$"`). Short, but it misses a `//go:build windows` line in a file without the suffix, `_linux.go` files on the Mac, architecture suffixes and `unix` constraints, and needs an edit through a rules proposal for each new kind. `go list` applies Go's own constraint rules.

**The host's `IgnoredGoFiles` from one `go list` call.** One call instead of seven (the seven take about 2 seconds on branch `devin/N-0-local-cli`), but `IgnoredGoFiles` also holds files behind custom tags and `//go:build ignore`. Excluding them would let production code leave the mutation gate by adding a tag; today such code fails as NOT COVERED, and the proposed rule keeps that.

**Running gremlins with `--tags` or another `GOOS`.** `--tags` reaches only `go test` (`coverage.go:147`); the walk still ignores constraints. Tests built for `GOOS=windows` do not run on macOS or Linux, so a Windows coverage profile cannot be produced on the Mac.

**Approving the mutants as equivalent.** The command engine has no `[mutation] ignore` list (noted in the go-mutation record), and the mutants are not equivalent, only untested here; it would also be a per-line list to keep by hand.

**A tracked helper script for the exclusions.** It would remove the duplicated shell from the two commands, but a script outside `.project-standard/` is not protected and could change without a rules proposal; protecting it is a larger change. The rule stays inline in the protected `gates.toml`, as the go-mutation record chose.

**A Windows mutation run now.** It is the answer to the coverage gap, and it is left as separate, later work by the coordinator's direction; this proposal states the gap instead.

## Acceptance criteria

After the coordinator applies the diff:

- On a branch containing `internal/node/socket_windows.go` (for example `devin/N-0-local-cli`), the harden command prints `gremlins: not built for this host, not mutated here: internal/node/socket_windows[.]go`, reports no mutant in that file, and exits 0 when the other mutants are killed or timed out.
- The nightly command on `rebuild` reports no mutant under `features/`.
- A non-test Go file behind a custom build tag outside the test roots still fails as NOT COVERED.
- `python3 .project-standard/scripts/gate.py --stage harden` passes on this branch (no Go change).
- On approval, the record moves to `docs/decisions/implemented/process/` with `Status: implemented` in the same branch before the merge, and `PROJECT.md` records the change.

## Risks

- **No mutation coverage for foreign-platform files until the Windows run exists.** `internal/node/socket_windows.go` (#107) and HS-19's `pty_windows.go` will carry untested-by-mutation lines; the gate prints which files it skipped, so the gap is visible in every run's output.
- **The platform list is fixed in the rule.** A new shipped platform needs a rules proposal. A file built only for a platform outside the list stays in the walk and fails as NOT COVERED, so the rule fails closed.
- **Host-dependent result.** A Linux gate (for example a review VM) excludes `darwin`-only files that the Mac mutates, and the reverse; the two runs can differ on such files.
- **Cost.** Seven `go list` calls, about 2 seconds in total on the current tree, are added to every harden and nightly run that has Go changes.
- **Narrowed match for the fake CLIs.** `internal/host/fakecli/` was matched anywhere in a path; it is now anchored at the module root, the only place it exists.
- **Output format.** The verdict still depends on gremlins' text output, as in the go-mutation record; a gremlins upgrade must recheck `--exclude-files` (regular expressions, slice flag, comma splitting) together with the three verdict cases.

## Verification

Observed on 2026-11-20 with gremlins v0.6.0 (reports its version as `dev`) and Go 1.27.4 on macOS arm64. Each command was read from a copy of `gates.toml` (current, and with the diff above applied) with the package's own parser and run through the package's command runner (`.project-standard/scripts/common.py`, `run`), as the command engine runs it. For the harden command, `git merge-base HEAD rebuild` was pointed at `origin/rebuild`, because the local `rebuild` branch is behind.

- Branch `devin/N-0-local-cli` at `7c9778d63`, harden command. Current rule: `NOT COVERED CONDITIONALS_NEGATION at internal/node/socket_windows.go:24:9`, `Killed: 25, Lived: 0, Not covered: 1`, exit 1, fail. Proposed rule: prints `internal/node/socket_windows[.]go` as not mutated, `Killed: 23, Lived: 0, Not covered: 0, Timed out: 2`, exit 0, pass. The set of runnable mutants differs only by the `socket_windows.go` mutant; the two TIMED OUT mutants (`internal/node/cmd.go:33:9`, `:41:9`) were KILLED in the first run, a timing variance that passes under both rules.
- Same branch, proposed rule, an untested function appended to `internal/node/dir.go` (uncommitted, then restored): `NOT COVERED` twice at `internal/node/dir.go:41:43`, exit 1, fail. The verdict for host files is unchanged.
- `rebuild` at `db85910f7`, nightly command. Current rule: 10 `NOT COVERED` mutants, all in `features/netsim/netsim_linux.go`, `Killed: 185`, exit 1, fail. Proposed rule: `Killed: 185, Not covered: 0`, no `features/` mutant, nothing excluded as foreign (`none`), exit 0, pass, 139 seconds.
- A scratch module covering the edge cases, nightly command. Current rule: NOT COVERED in a root-package `_windows.go` file, a package of only `_windows.go` files, a `_amd64.go` file, a `//go:build linux` file, a `_windows.go` file that does not parse, a non-tagged file under `features/`, a file under `testdata/`, and a `//go:build debug` file. Proposed rule: only the `//go:build debug` file remains NOT COVERED, exit 1. A `//go:build cgo` file built on the host is not excluded.
- The exclusion computation alone gives the same arguments under macOS `/bin/sh` and `dash`, and takes about 2 seconds on `devin/N-0-local-cli`.
- On this branch, the proposed harden command exits 0 (no Go change); `python3 .project-standard/scripts/check.py` reports 0 violations; `python3 .project-standard/scripts/tamper.py` reports 0 findings.

Not verified: a run on Linux (only the Mac was used; `dash` ran the exclusion part, not gremlins); a Windows mutation run, which does not exist; the full merge stage on this branch, whose web commands fail in this worktree because `web/node_modules` is not installed.

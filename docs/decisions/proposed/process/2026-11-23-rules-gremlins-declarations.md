# Agent Note: Mutation gate: declarations, case lines and equivalent mutants

Status: proposed

## Problem

The first CI mutation run (pull request #358, comment 7101958800) reported 22 NOT COVERED mutants in files the Linux runner builds. 21 of them are not untested code:

- **Package-level declarations (8).** gremlins v0.6.0 takes coverage from `go test -cover`, and Go records no coverage block for a `const` or `var` declaration, so every mutant there is NOT COVERED. Examples: `const joinAttachTimeout = 15 * time.Second` (`internal/host/join.go:15`), `var ptyStopTimeout = 10 * time.Second` (`internal/host/pty.go:19`).
- **`case` expression lines (13).** The expression of a `case` clause lies outside every coverage block (a block starts at the colon), so its mutants are NOT COVERED although the function is tested. Examples: `internal/node/export.go:33`, `internal/host/codex.go:647`.

Separately, two LIVED mutants are equivalent: no test can tell the mutant from the original.

- `internal/host/limits.go:311`, `lp.seq++` to `lp.seq--`: the value only has to differ between watches, and counting down gives distinct values too.
- `internal/node/cmd.go:103`, `time.Sleep(25 * time.Millisecond)` with `*` turned into `/`: the poll interval of a dial retry loop bounded by a deadline; no test can tell 25 ms from 0.

The 22nd NOT COVERED mutant (`internal/node/schedule.go:22:97`) is not dropped by this proposal; it is a fix unit's job.

## Proposal

1. Language, thresholds, tiers, modules, dependency rules, protected paths: unchanged. Zero surviving or uncovered mutants stays the rule.
2. A new protected script, `.project-standard/scripts/mutants.py`, reads gremlins' output and prints the LIVED and NOT COVERED lines that stay. It exits 1 if any stays. It drops:
   - a **NOT COVERED** mutant on a package-level `const` or `var` line, unless that line contains `func` (a function literal body is code and stays);
   - a **NOT COVERED** mutant on a `case ...:` or `default:` line;
   - any mutant listed in `EQUIVALENT`, matched by file, mutator and the stripped text of its line (line numbers drift; a changed line stops matching and fails the gate again). Each entry carries a one-line reason. The list is part of the protected script, so a new entry needs a rules proposal.
3. In both Go mutation commands (`[stages] nightly`, first entry, and `[mutation] command`), the verdict `! printf "%s\n" "$o" | grep -Eq "^ *(LIVED|NOT COVERED) "` becomes `printf "%s\n" "$o" | python3 .project-standard/scripts/mutants.py`. The substring is the same with or without #114, so the two proposals apply in either order.

Files the owner applies (both are under protected `.project-standard/`, git-excluded, so not part of this branch's diff):

- `.project-standard/scripts/mutants.py` (new): the text below.
- `.project-standard/gates.toml`: the replacement in the two commands.

### `gates.toml` change

In both commands:

```diff
-[ $s -eq 0 ] && ! printf "%s\n" "$o" | grep -Eq "^ *(LIVED|NOT COVERED) "; }
+[ $s -eq 0 ] && printf "%s\n" "$o" | python3 .project-standard/scripts/mutants.py; }
```

Add to the `[mutation]` comment: `# NOT COVERED mutants on package-level const/var lines and case lines, and the listed equivalent mutants, are dropped by .project-standard/scripts/mutants.py (see docs/decisions/implemented/process/2026-11-23-rules-gremlins-declarations.md).`

### `.project-standard/scripts/mutants.py`

```python
"""Verdict filter for gremlins output (stdin): prints each LIVED or NOT COVERED line that stays, exits 1 if any.

Dropped: a NOT COVERED mutant on a package-level const or var line (gremlins gets no coverage block for declarations)
or on a case or default expression line (the expression lies outside every coverage block); and a mutant listed in
EQUIVALENT, matched by file, mutator and the stripped text of its line, so the entry survives line renumbering.
"""
import re
import sys

EQUIVALENT = {
    ("internal/host/limits.go", "INCREMENT_DECREMENT", "lp.seq++"):
        "gen only has to differ between watches; counting down gives distinct values too",
    ("internal/node/cmd.go", "ARITHMETIC_BASE", "time.Sleep(25 * time.Millisecond)"):
        "poll interval of a dial retry loop bounded by a deadline; no test can tell 25 ms from 0",
}
LINE = re.compile(r"^\s*(LIVED|NOT COVERED)\s+(\w+)\s+at\s+(\S+?):(\d+):\d+")
CASE = re.compile(r"^\s*(case\b.*|default):\s*(//.*)?$")
TOP = re.compile(r"^(const|var)\b")


def depth(text):
    text = re.sub(r'"(\\.|[^"\\])*"|`[^`]*`|\'(\\.|[^\'\\])*\'|//.*', "", text)
    return sum(text.count(o) - text.count(c) for o, c in ("()", "{}", "[]"))


def declaration_lines(src):
    out, open_ = set(), 0
    for n, text in enumerate(src, 1):
        if open_ == 0 and not TOP.match(text):
            continue
        out.add(n)
        open_ = max(open_ + depth(text), 0)
    return {n for n in out if "func" not in src[n - 1]}


def main():
    cache, left = {}, []
    for raw in sys.stdin:
        m = LINE.match(raw)
        if not m:
            continue
        kind, mutator, path, num = m[1], m[2], m[3], int(m[4])
        if path not in cache:
            try:
                src = open(path, encoding="utf-8").read().split("\n")
            except OSError:
                src = []
            cache[path] = (src, declaration_lines(src))
        src, decls = cache[path]
        text = src[num - 1].strip() if 0 < num <= len(src) else ""
        if (path, mutator, text) in EQUIVALENT:
            continue
        if kind == "NOT COVERED" and (num in decls or CASE.match(text)):
            continue
        left.append(raw.rstrip())
    print("\n".join(left))
    return 1 if left else 0


sys.exit(main())
```

## Alternatives considered

- **Inline shell or awk in `gates.toml`.** The command engine's TOML reader takes single-line strings without single quotes; the classification would be unreadable.
- **Go AST tool.** Exact, but a new build target for a filter; the text rule is checked by the verification below and fails closed (a line it misreads stays reported).
- **Changing code to dodge the tool** (turning constants into functions). It adds code for a tool defect; the owner's rule is least code.

## Acceptance criteria

After the owner applies both files:

- On `rebuild` at `f5d63a914`, fed the findings of #358: 21 of 22 NOT COVERED lines and the two equivalent LIVED lines are dropped; 28 LIVED and `internal/node/schedule.go:22:97` stay; exit 1.
- A NOT COVERED mutant on an ordinary statement still fails the gate.
- `python3 .project-standard/scripts/gate.py --stage harden` passes on this branch (no Go change).
- On approval, the record moves to `docs/decisions/implemented/process/` with `Status: implemented` in the same branch before the merge, and `PROJECT.md` records the change.

## Risks

- **Case conditions go unmeasured when a case is never reached.** A `case` expression whose clause no test reaches is not reported through its expression line; the clause body lines still are.
- **Text matching.** A `var x = func() {` line is kept, but a multi-line declaration is judged by bracket depth, with strings and comments stripped by regular expression; a raw string holding unbalanced brackets could misjudge a declaration's end. Misjudging only keeps or drops lines in that declaration.
- **Equivalence is a judgement.** The two entries were read from the code, not proved.
- **Output format.** The script reads `LIVED|NOT COVERED <MUTATOR> at <file>:<line>:<col>`; a gremlins upgrade that changes it makes the script drop everything it cannot match, so the verdict would pass with no lines read. gremlins' own exit status is still checked before it, and a format change must be rechecked at upgrade, as #114 notes for the other commands.

## Verification

Observed on 2026-11-23 on macOS arm64 with `python3 -I`. The script was run on the 52 Go lines of the #358 comment, with the worktree at `f5d63a914` (`rebuild` before T3-14 moved line numbers): 23 dropped (21 NOT COVERED, 2 equivalent), 29 printed, exit 1. gremlins itself was not re-run; the proposed gate was not run on a Go change.

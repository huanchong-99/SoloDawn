# Agent Note: Rules: the dup gate ignores Go package clauses and import declarations

Status: implemented

## Problem

The built-in duplicate detector hashes every window of 6 consecutive significant lines (non-blank, non-comment, at least 4 non-space characters). In Go, the first lines of a file are the package clause and the import list. Files of one package import largely the same packages, so those lines repeat from file to file, and a window of 6 lines inside the import list matches a window in another file. On the files changed since 9a4e116ea the gate reports 29 blocks, and 27 of them consist of nothing but the package clause and import specs, for example `internal/home/cmd_task.go:1-9` against `internal/home/cmd_confirm_task.go:1`, `internal/host/limits.go:1-11` against `internal/host/takeover.go:1`, `internal/node/socket_unix.go:3-10` against `internal/node/socket_windows.go:1`. They block every merge that touches such a file, and no refactor can remove them: an import list is dictated by what the file uses (`goimports` writes it), and it is a list of names, not logic. The other 2 blocks are a real duplicate (`web/src/lib/identity.ts:253-258` and `web/src/lib/owner.ts:23-28`, an IndexedDB open-transaction-close wrapper) and are not affected.

This is a rule problem, recorded as a rule request as `RULES.md` asks: no waiver, no lowered threshold.

### Summary

1. Language and roots: unchanged (`go` with `.go` and `.ts` files; source `cmd`, `internal`, `web/src`; tests and excluded globs unchanged).
2. Thresholds: unchanged (CRAP 6, ui 8; duplication 3 percent, blocks of 6 lines; mutation survivors 0).
3. Tiers: unchanged.
4. Modules: unchanged.
5. Dependency rules: unchanged.
6. Protected paths and directives: unchanged. The change is to a package script, `.project-standard/scripts/dup.py`, which is protected, so the owner applies it with `apply-dup-imports.sh`; the script also updates that script's line in `manifest.json` and appends the record to `PROJECT.md`. `gates.toml` is not touched.
7. Expected debt: on the files changed between 9a4e116ea and `origin/rebuild` 47ae57c1e the dup gate reports 29 blocks before and 2 after (the 27 import-only blocks are gone; the 2 left are the real duplicate). Whole project (`dup.py --all`): 189 of 26497 significant lines duplicated (0.71 percent) before, 12 of 25239 (0.05 percent) after. CRAP, dependency and module debt unchanged.

## Decision

Approval: approved by the owner and applied to `.project-standard/` on 2026-11-27 by the owner, who ran `apply-dup-imports.sh` himself (backup folder `.claude/rules-apply/backup-20261127-195719-dup-imports`); the tamper check passed after applying.

Proposed by: Claude (a Sonnet-model subagent), drafted on 2026-11-27 at the coordinator's request after the Stop hook kept blocking the coordinator session on these findings, and corrected after an independent review by an Opus-model subagent. The owner approves or rejects; nothing is applied before that. Not `user-directed` and not `user-approved`: the choice is the agent's own proposal.

One change, in the built-in engine of the dup gate (`[dup] engine = "builtin"`; the setting in `gates.toml` stays as it is): in a file ending in `.go`, the package clause and the import declarations of the file header are not significant lines, so they cannot take part in a duplicate window. Everything else is measured as before, including `const`/`var` blocks, other languages and the block size.

- Only the file header is scanned. Go requires imports before every other declaration, so the scan ends at the first line that is none of: blank, a comment line (`//`, `/*` or `*` prefix), the package clause (`package name`), or an import declaration. From that line on, nothing is skipped.
- An import declaration is `import (` at the end of a line, or a single-line `import "path"`, `import alias "path"`, `import . "path"`, `import _ "path"`. Inside an `import (` block only import specs (`"path"`, `alias "path"`), blank lines and `//` comments are skipped, up to the `)` at column 0. Any other line ends the block and is counted as code.
- Line numbers of reported blocks are unchanged (they are the file's own line numbers). The percentage in `dup.py --all` is computed over the remaining significant lines.

Diff of `.project-standard/scripts/dup.py` (the whole change; Python 3.9 compatible like the rest of the package):

```diff
--- a/.project-standard/scripts/dup.py
+++ b/.project-standard/scripts/dup.py
@@ -18,11 +18,37 @@



-def significant_lines(text: str, prefixes) -> list:
-    """[(lineno, normalized)] for lines that carry code: not blank, not a comment, at least four non-space characters."""
+GO_PACKAGE = re.compile(r'package\s+\w+\s*(?://.*)?$')
+GO_IMPORT = re.compile(r'import\s*(?:\(\s*$|(?:[\w.]+\s+)?"[^"]*"\s*(?://.*)?$)')
+GO_SPEC = re.compile(r'\s*(?:[\w.]+\s+)?"[^"]*"\s*(?://.*)?$')
+
+
+def go_header_lines(text: str) -> set:
+    """Line numbers of a Go file's package clause and import declarations, which come before any other declaration: they name
+    packages, they are not logic, so files may share them. The first line that is none of these ends the header."""
+    skip, block = set(), False
+    for i, raw in enumerate(text.splitlines(), 1):
+        s = raw.strip()
+        if block:
+            if raw.startswith(")") or not s or s.startswith("//") or GO_SPEC.match(raw):
+                skip.add(i)
+                block = not raw.startswith(")")
+                continue
+            return skip
+        if not s or s.startswith(("//", "/*", "*")):
+            continue
+        if not (GO_PACKAGE.match(raw) or GO_IMPORT.match(raw)):
+            return skip
+        skip.add(i)
+        block = raw.rstrip().endswith("(")
+    return skip
+
+
+def significant_lines(text: str, prefixes, skip=()) -> list:
+    """[(lineno, normalized)] for lines that carry code: not blank, not a comment, not in `skip`, at least four non-space characters."""
     out = []
     for i, raw in enumerate(text.splitlines(), 1):
-        if raw.lstrip().startswith(tuple(prefixes)):
+        if i in skip or raw.lstrip().startswith(tuple(prefixes)):
             continue
         norm = " ".join(raw.split())
         if len(norm.replace(" ", "")) < 4:
@@ -31,9 +57,14 @@
     return out


+def file_lines(path: Path, prefixes) -> list:
+    text = c.read_text(path)
+    return significant_lines(text, prefixes, go_header_lines(text) if path.suffix == ".go" else ())
+
+
 def find_blocks(root: Path, rels: list, prefixes, min_lines: int) -> dict:
     """Duplicate blocks per file: {rel: [(start, end, partner_rel, partner_start)]}; also the total of significant lines."""
-    per_file = {rel: significant_lines(c.read_text(root / rel), prefixes) for rel in rels}
+    per_file = {rel: file_lines(root / rel, prefixes) for rel in rels}
     windows: dict = {}
     for rel, lines in per_file.items():
         for i in range(0, len(lines) - min_lines + 1):
```

`manifest.json`: the sha256 of `scripts/dup.py` changes from `cc6217400923...` to `7c1ffec381d1...` (one line). `PROJECT.md`: one entry appended to the `Rules approved on` row. `apply-dup-imports.sh` makes these three edits and nothing else, refuses when its targets are not what it expects, and is safe to run twice.

## Alternatives considered

- **Keep the rule and fix the files.** Impossible for import lists; the same files would be reported again after any edit.
- **Exempt the files or raise `duplication_min_lines`.** A waiver per file does not scale (27 now, more with every new file), and a window large enough to skip the longest import-only block (9 lines) would stop catching real 6 to 9 line duplicates, which is the reason the limit is 6.
- **Switch to `jscpd`** (`[dup] engine = "jscpd"`). An external tool (Node) with a different matching model; it would change every result, not only the import findings.
- **Skip every block opened by `import (` anywhere in the file** (the first version of this proposal). Rejected after the independent review: Go allows `import (` at column 0 inside a `/* */` comment or a raw string, and everything after it up to the next `)` at column 0 was hidden; function bodies end with a closing brace, not a parenthesis, so three identical 7-line functions disappeared from the gate in the reviewer's test. The header-only scan has no such case.
- **Skip every line that starts with a quote inside any parenthesised block.** It would also hide the contents of `const (` / `var (` string tables.
- **Ignore the import declarations but keep the package clause.** The clause carries no logic either, and files of one package all begin with the same line; dropping it keeps the first window of every file made of real code.

## Consequences

- Only header lines can be skipped, and only lines that are the package clause, an import declaration, a blank line or a comment line inside the header. Text that merely looks like an import (in a comment or a raw string) can hide at most that one line, never the code after it: the first line that is not a header line ends the scan.
- A file whose header is preceded by a multi-line `/* */` comment with text lines that carry no `*` prefix is not recognised as having a header; its import lines are counted as before (reported, not hidden). The rule fails towards reporting.
- An import block opened with a trailing comment (`import ( // x`) is not recognised as a block, so its lines count as before.
- The whole-project duplication percentage drops (0.71 to 0.05), so the 3 percent limit has more headroom; the rule for a session's own findings (any block touching its changed lines) is unchanged.
- This rule alone does not unblock a session whose changed lines touch the 2 real TypeScript blocks; those keep the rule until refactored.
- The change is to a package script, so the next package upgrade (a new `manifest.json`) replaces it; the rule change must then be re-applied or sent upstream.

## Verification

- `python3 -I test_dup_imports.py <scripts dir>` (17 tests; embedded in `apply-dup-imports.sh`, which runs them after applying): against the original package `FAILED (failures=8, skipped=3)`; against the first version of the proposal `FAILED (failures=5)`, namely the three attack cases below and two header-boundary tests; against the applied version `Ran 17 tests ... OK`.
  - Attack cases that stay caught: an `import (` line inside a block comment, inside a raw string, and after a real import block, each followed by an identical 7-line function in two files.
  - Also: a real 6-line logic duplicate is still found, its block starts at the logic, five logic lines stay below the limit, a `const (` block is still logic, TypeScript import blocks are unchanged, single-line, aliased, blank and dot imports and comments inside an import block are skipped, nothing after the first non-header line is skipped.
- The dup gate on the range 9a4e116ea to 47ae57c1e: 29 blocks before, 2 after (`web/src/lib/identity.ts:253-258`, `web/src/lib/owner.ts:23-28`). The whole-project run shows 12 of 25239 significant lines in both the applied version and the first; on every real file the two skip the same lines.
- `tamper.py` reports no `manifest:` finding after the script ran.

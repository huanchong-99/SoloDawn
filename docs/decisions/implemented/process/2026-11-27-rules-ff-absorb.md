# Agent Note: Rules: a fast-forward of HEAD to the remote mainline is an outside change

Status: implemented

## Problem

The Stop hook and the gates measure "the session's changes" as the files whose content differs from the snapshot taken at the start of the task (`hook.baseline`), and their changed lines as `git diff <base_commit>`. `hook.absorb_outside` moves files that changed while a session was away into the base, but it runs only at `SessionStart` with source `resume`. A session that runs in the main checkout while the checkout is fast-forwarded (`git pull --ff-only`, `git merge --ff-only origin/rebuild`) therefore counts every file of the pulled commits as its own change, and every changed line of them as its own changed line. The coordinator session's base is 9a4e116ea; the main checkout now sits at 47ae57c1e, 397 commits later, and the session "changed" 1273 files. Every one of those commits was written in a unit session that ran its own gates and was merged on `rebuild` after CI; the Stop gates re-run on code nobody in this session wrote and fail on its findings (29 dup blocks, of which 27 are also a separate rule problem, see the dup proposal).

### Summary

1. Language and roots: unchanged.
2. Thresholds: unchanged.
3. Tiers: unchanged.
4. Modules: unchanged.
5. Dependency rules: unchanged.
6. Protected paths and directives: unchanged. The change is to a package script, `.project-standard/scripts/changes.py`, which is protected, so the owner applies it with `apply-ff-absorb.sh`; the script also updates that script's line in `manifest.json` and appends the record to `PROJECT.md`. `gates.toml` and `hook.py` are not touched.
7. Expected effect, measured on a clone fast-forwarded from 9a4e116ea to `origin/rebuild` 47ae57c1e with the coordinator session's state: the Stop hook's `session_changes` returns 1273 files before and 0 after; the dup gate returns 29 blocks before and 0 after. No debt is created or removed; the gates stop measuring other sessions' merged work.

## Decision

Approval: approved by the owner and applied to `.project-standard/` on 2026-11-27 by the owner, who ran `apply-ff-absorb.sh` himself (backup folder `.claude/rules-apply/backup-20261127-195708-ff-absorb`); the tamper check passed after applying.

Proposed by: Claude (a Sonnet-model subagent), drafted on 2026-11-27 at the coordinator's request after the Stop hook kept blocking the coordinator session on work it had only pulled, and tightened after an independent review by an Opus-model subagent, who found five ways for a session's own commit to be absorbed in the first version. The owner approves or rejects; nothing is applied before that. Not `user-directed` and not `user-approved`: the choice is the agent's own proposal.

One function in `changes.py`, two module constants and one call at the top of `changes.changed_files` (the single place from which the Stop hook, `gate.py`, `dup.py`, `tamper.py`, `deps.py`, `evidence.py` and the changed-function lookup read the session's files; `hook.py` needs no change, and a manual `gate.py --stage merge` agrees with the Stop hook).

`absorb_fast_forward(root, state)` moves the task base to HEAD, and puts the files of the commits in between into the snapshot as they are on disk, when all of the following hold:

1. the state has a snapshot and a `base_commit`, the repository is a git repository, and HEAD is not the base;
2. the upstream of the checked-out branch is exactly `MAINLINE = "refs/remotes/origin/rebuild"`, and HEAD is an ancestor of (or equal to) that ref. Any other upstream (a feature branch, a local branch, none) absorbs nothing. `origin/HEAD` is not used: the remote's default branch is `main`, not `rebuild`;
3. every move of HEAD since the oldest time HEAD was at `base_commit` is a pull or merge that fast-forwarded. The reflog of HEAD is read from its oldest entry that points at the base onward, and each entry that moved HEAD must have a whole subject matching `(?:pull|merge)\b[^:]*: Fast-forward` (`merge origin/rebuild: Fast-forward`, `pull --ff-only origin rebuild: Fast-forward`, `pull -q --rebase: Fast-forward`); entries that did not move HEAD (`checkout: moving from rebuild to rebuild`) are ignored. A `commit:` entry (a commit subject that says "Fast-forward" does not match: the whole subject must be a pull or merge), a `reset:`, a `checkout` of another commit or branch, a rebase, or a base that is not in the reflog absorbs nothing: the old behaviour stands. Because the scan starts at the oldest time HEAD was at the base, a commit made here and then hidden by moving HEAD away and back (`reset --hard <base>`, or a switch to a side branch and back) is still inside the scanned range.

The files absorbed are those in `git diff --name-only base HEAD` that are not the session's own: not modified against HEAD (`git diff --name-only HEAD`), not untracked. Deleted files leave the snapshot. The session's own uncommitted work, before or after the fast-forward, still counts and is measured against the new base (`git diff HEAD`), so it does not drag the pulled hunks in.

The function runs on every call of `changed_files`; after the first it returns at once (`base_commit` is HEAD). Callers that persist the state (the Stop hook, `gate.py`) keep the new base; the others recompute the same answer.

Diff of `.project-standard/scripts/changes.py` (the whole change: 35 added lines, 0 removed):

```diff
--- a/.project-standard/scripts/changes.py
+++ b/.project-standard/scripts/changes.py
@@ -14,12 +14,51 @@
 import common as c  # noqa: E402

 HUNK = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@")
+MAINLINE = "refs/remotes/origin/rebuild"  # the remote branch whose commits passed their own session's gates and CI
+FAST_FORWARD = re.compile(r"(?:pull|merge)\b[^:]*: Fast-forward")  # the whole reflog subject of a pull or merge that fast-forwarded


 # ---------------------------------------------------------------- changed files and ranges

+def git_paths(root: Path, *args) -> set:
+    """The paths a NUL-separated git listing prints."""
+    return set(c.git(root, *args).stdout.split("\0")) - {""}
+
+
+def absorb_fast_forward(root: Path, state: dict) -> None:
+    """HEAD got from the task's base commit to the remote mainline (MAINLINE, which must also be the upstream of the checked-out
+    branch) by pulls and merges that fast-forwarded and nothing else: those commits were made and checked elsewhere, so their
+    files join the base like any outside change. Any other move of HEAD since the oldest time HEAD was at the base (a commit, a
+    reset, a checkout, a rebase) or a HEAD that is not on MAINLINE absorbs nothing."""
+    base, snap = state.get("base_commit"), state.get("snapshot")
+    head = c.git(root, "rev-parse", "HEAD").stdout.strip() if base and snap and c.has_git(root) else ""
+    if not head or head == base:
+        return
+    if c.git(root, "rev-parse", "--symbolic-full-name", "@{upstream}").stdout.strip() != MAINLINE:
+        return
+    if c.git(root, "merge-base", "--is-ancestor", head, MAINLINE).returncode:
+        return
+    log = [line.partition(" ")[::2] for line in c.git(root, "reflog", "--format=%H %gs", "HEAD").stdout.splitlines()]
+    at = [i for i, (sha, _) in enumerate(log) if sha == base]
+    if not at:
+        return
+    prev = base
+    for sha, why in reversed(log[:at[-1]]):
+        if sha != prev and not FAST_FORWARD.fullmatch(why):
+            return
+        prev = sha
+    mine = git_paths(root, "diff", "--name-only", "-z", "HEAD") | git_paths(root, "ls-files", "-z", "--others", "--exclude-standard")
+    for rel in git_paths(root, "diff", "--name-only", "--no-renames", "-z", base, head) - mine:
+        if (root / rel).is_file():
+            snap[rel] = c.sha256_file(root / rel)
+        else:
+            snap.pop(rel, None)
+    state["base_commit"] = head
+
+
 def changed_files(root: Path, state: dict) -> dict:
     """{rel: added|modified|deleted} relative to the session snapshot; outside a session (no snapshot) the git working tree against HEAD."""
+    absorb_fast_forward(root, state)
     snapshot = state.get("snapshot") or {}
     if snapshot:
         diff = c.diff_snapshot(snapshot, c.take_snapshot(root))
```

`manifest.json`: the sha256 of `scripts/changes.py` changes from `564ada52ccb2...` to `df757c8b5ba3...` (one line). `PROJECT.md`: one entry appended to the `Rules approved on` row. `apply-ff-absorb.sh` makes these three edits and nothing else, refuses when its targets are not what it expects, and is safe to run twice.

## Alternatives considered

- **Do not change the rule; re-baseline by finishing the task after each pull** (`finish.py`, or a new session). Finishing a task needs the task's gates and closure first, and the pull happens while a task is open; that is the failure being fixed. It also does not help a gate run by hand before the task is finished.
- **Absorb in `hook.py` (`session_changes`, or a new call at each prompt).** A manual `gate.py --stage merge` would then disagree with the Stop hook (the gates read the state themselves). A call in `UserPromptSubmit` that simply calls `baseline()` is worse: it throws away the session's own work.
- **Absorb on any change of HEAD, or on "HEAD is on the remote" alone.** A session that commits its own work, pushes it and has the checkout at `origin/rebuild` would then measure nothing. The reflog condition separates "arrived by pull" from "committed here".
- **Accept any upstream (`@{upstream}`), as the first version did.** Rejected after the review: an upstream can be a feature branch the session pushes its own work to, or a local branch (`git branch -u work`), and the premise that the commits were gated by their own session and CI can only be argued for the mainline. The constant `MAINLINE` costs one line; the tests set it to `refs/remotes/origin/main`.
- **A `[project] mainline` key in `gates.toml` with `rules.py` support.** More code in three places for no gain; this script is already a local patch of the package.
- **Stop scanning the reflog at the newest entry that points at the base (the first version).** Rejected after the review: `reset --hard <base>`, or a switch back from a side branch, put HEAD at the base again and the session's earlier commit dropped out of the scan.
- **Match the reflog subject by the substring "Fast-forward" (the first version).** Rejected after the review: a commit subject such as "Fast-forward the cache" is written into the reflog as `commit: Fast-forward the cache` and matched.
- **Why not absorb at all?** Considered: the session that wrote the commits is not this session, so nothing here vouches for them. Rejected because every commit on `rebuild` is meant to have passed its own unit session's gates and CI, the same standard the project already applies to anything written by someone else while a session was away (`absorb_outside`). See the risks: that premise is a team convention today, not something GitHub enforces.

## Consequences

Two ways to get a session's own work absorbed remain open after the tightening. Both need a deliberate act; this rule does not close them. Every remaining bypass needs a deliberate push of the session's own work to `rebuild`, or a forged `refs/remotes/origin/rebuild`; there is no other.

- **Forged git records.** Anyone who can run `git update-ref`, set `GIT_REFLOG_ACTION` for a `git commit`, or edit `.git/logs/HEAD` can forge the reflog and, with `git update-ref`, the remote-tracking ref `refs/remotes/origin/rebuild`; both are local and unprotected. For example, a plain `git commit` run with `GIT_REFLOG_ACTION="merge origin/rebuild"` and the subject `Fast-forward` writes the reflog line `merge origin/rebuild: Fast-forward`, which matches; once that commit is pushed to `rebuild`, it is absorbed. A commit made the normal way writes `commit: <subject>` and is not absorbed, whether pushed or not. Closing it would mean a network call to the remote on every gate, which this change does not add. Before this change no git command could hide a changed file from the Stop hook (the changed files were a content snapshot kept in protected state); after it, git metadata that the agent controls decides what is absorbed. Accepted as a known, deliberate-only gap.
- **A push to `rebuild` from another clone, then a pull.** A session can push its own work to `rebuild` from another clone or worktree and then fast-forward this checkout; that work is absorbed. The premise ("every commit on `rebuild` passed its own unit session's gates and CI") is a team convention, not something GitHub enforces: `rebuild` has no branch protection (`gh api repos/huanchong-99/SoloDawn/branches/rebuild/protection` answers `Branch not protected`; the repository's only ruleset has enforcement `disabled`), and 62 of the 397 commits from 9a4e116ea to 47ae57c1e have no `(#n)` pull request number in their subject. Turning on branch protection with required status checks for `rebuild` is the owner's GitHub setting; it would make the premise true.

Other consequences:

- **Protected tracked paths.** The files absorbed are not checked against `[tamper] protected`. Today every protected path (`.claude/settings.json`, `.codex/hooks.json`, `.project-standard/**`) is git-excluded and cannot arrive in a commit. If a tracked path is added to `protected` later, a change of it that arrives by fast-forward would be absorbed; the proposal that adds the path should then also exclude it here.
- **The reflog is local and expires** (90 days by default). A missing or truncated reflog, or one without the base, absorbs nothing: the failure is the old behaviour, not a missed gate. The match is on the text git writes into the reflog (`Fast-forward`); checked with git 2.54 on macOS, English locale.
- **Mixed tasks.** One commit, reset, checkout or rebase in this checkout after the base turns absorption off for the rest of the task (the all-or-nothing condition keeps the rule short and the fallback the old behaviour); the next task starts from a new base. So does an earlier visit of HEAD to the base commit: the scan starts at the oldest.
- **The mutation gate is not affected**: its command diffs `git merge-base HEAD rebuild` and does not read the session state.
- The change is to a package script, so a package upgrade (a new `manifest.json`) replaces it; the rule change must then be re-applied or sent upstream.

## Verification

- `python3 -I test_ff_absorb.py <scripts dir>` (26 tests on real git repositories: a bare remote, the checkout, another clone; embedded in `apply-ff-absorb.sh`, which runs them after applying): against the original package `FAILED (failures=7)` (the fast-forward cases); against the first version of the proposal `FAILED (failures=5)`, namely the five attack cases below; against the applied version `Ran 26 tests ... OK`.
  - Attack cases, each measured (counted as the session's), not absorbed: an own commit whose subject says "Fast-forward", pushed; an own commit, push, `reset --hard <base>`, pull; an own commit on a side branch pushed to the mainline, switch back, pull; the session's own work pushed to a feature-branch upstream from another clone, then pulled; a fast-forward merge of a local branch set as the upstream. One more test pins the upstream rule: a fast-forward to the mainline on a branch whose upstream is another branch.
  - Also pinned: nothing moved; fast-forward of added, modified and deleted files; fetch plus `merge --ff-only`; several commits; two fast-forwards in a row; own uncommitted work counts, before and after the pull; own commit pushed; fast-forward then own commit; own commit rebased onto the remote; reset to the remote; checkout of a pushed commit; no upstream; HEAD not on the remote; base missing from the reflog; a repeated call is stable; no snapshot keeps the old behaviour; an honest `pull --rebase` with no local commit is absorbed.
- Damaging one line of the function at a time (the subject match, the oldest-entry scan, the upstream check, the ancestry check, the session's own files, moving the base) makes tests fail each time.
- End to end: a clone of this repository at 9a4e116ea with the coordinator session's state (145 files in the snapshot), fast-forwarded to 47ae57c1e: `hook.session_changes` 1273 files before, 0 after; `dup.py` 29 blocks before, 0 after; `tamper.py` reports no `manifest:` finding after the script ran. With the dup rule change as well, the result is the same.

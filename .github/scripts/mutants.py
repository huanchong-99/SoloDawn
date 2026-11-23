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

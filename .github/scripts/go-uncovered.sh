#!/usr/bin/env bash
# Lists the product statements that a Go cover profile shows as never run, and fails when there is one.
# Usage: go-uncovered.sh <cover profile>
#
# The profile holds the files this system builds, so a file built only on another system is checked
# by that system's job. What is left out is what .project-standard/gates.toml calls test-only or
# generated: features/, the fake CLIs under internal/host/fakecli/, and **/generated/**. Test files
# and testdata are not in a profile. A block without statements has nothing to cover.
set -euo pipefail

profile=${1:?usage: go-uncovered.sh <cover profile>}

awk -v mod='github.com/huanchong-99/SoloDawn/' '
  NR > 1 {
    file = $1; sub(/:.*/, "", file); sub("^" mod, "", file)
    if (file ~ /^features\// || file ~ /^internal\/host\/fakecli\// || file ~ /\/generated\//) next
    block = $1; sub("^" mod, "", block)
    stmts[block] = $2
    if ($3 > 0) run[block] = 1
  }
  END {
    for (b in stmts) {
      if (stmts[b] == 0) continue
      total += stmts[b]
      if (!(b in run)) { printf "%s %d\n", b, stmts[b]; missed += stmts[b] }
    }
    printf "%d of %d product statements are not covered\n", missed, total > "/dev/stderr"
    exit missed > 0
  }' "$profile" | sort | {
    status=0
    while read -r block n; do
      echo "::error file=${block%%:*}::Not covered: $block ($n statement$([ "$n" -gt 1 ] && echo s))"
      echo "- \`$block\`: $n statement(s) never run" >> "${GITHUB_STEP_SUMMARY:-/dev/null}"
      status=1
    done
    exit $status
  }

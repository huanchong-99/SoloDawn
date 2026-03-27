#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

BRANCH=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --branch)
      BRANCH="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1"
      echo "Usage: $0 [--branch <branch>]"
      exit 1
      ;;
  esac
done

cd "$PROJECT_ROOT"

# Default to current branch
if [[ -z "$BRANCH" ]]; then
  BRANCH="$(git rev-parse --abbrev-ref HEAD)"
fi

echo "=== SoloDawn Branch Quality Gate ==="
echo "Project root: $PROJECT_ROOT"
echo "Branch:       $BRANCH"
echo ""

# Compute changed files vs main
CHANGED_FILES="$(git diff --name-only "main...$BRANCH" | tr '\n' ',' | sed 's/,$//')"

if [[ -z "$CHANGED_FILES" ]]; then
  echo "No changed files detected between main and $BRANCH."
  echo "Branch quality gate passed (nothing to check)."
  exit 0
fi

echo "Changed files: $CHANGED_FILES"
echo ""

cargo run --package quality -- \
  --tier branch \
  --config quality/quality-gate.yaml \
  --working-dir "$PROJECT_ROOT" \
  --changed-files "$CHANGED_FILES"

EXIT_CODE=$?

if [[ $EXIT_CODE -eq 0 ]]; then
  echo ""
  echo "Branch quality gate passed."
else
  echo ""
  echo "Branch quality gate failed."
fi

exit $EXIT_CODE

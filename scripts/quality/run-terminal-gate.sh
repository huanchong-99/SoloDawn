#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

WORKING_DIR=""
CHANGED_FILES=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --working-dir)
      WORKING_DIR="$2"
      shift 2
      ;;
    --changed-files)
      CHANGED_FILES="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      echo "Usage: $0 --working-dir <dir> --changed-files <files>" >&2
      exit 1
      ;;
  esac
done

if [[ -z "$WORKING_DIR" ]]; then
  echo "Error: --working-dir is required" >&2
  exit 1
fi

if [[ -z "$CHANGED_FILES" ]]; then
  echo "Error: --changed-files is required" >&2
  exit 1
fi

echo "=== GitCortex Terminal Quality Gate ==="
echo "Project root: $PROJECT_ROOT"
echo "Working dir:  $WORKING_DIR"
echo "Changed files: $CHANGED_FILES"
echo ""

cd "$PROJECT_ROOT"

cargo run --package quality -- \
  --tier terminal \
  --config quality/quality-gate.yaml \
  --working-dir "$WORKING_DIR" \
  --changed-files "$CHANGED_FILES"

EXIT_CODE=$?

if [[ $EXIT_CODE -eq 0 ]]; then
  echo ""
  echo "Terminal quality gate passed."
else
  echo ""
  echo "Terminal quality gate failed." >&2
fi

exit $EXIT_CODE

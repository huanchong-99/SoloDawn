#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

TIER="${1:-repo}"
MODE="${2:-shadow}"
shift 2 2>/dev/null || true

INCLUDE_BASELINE=false
INCLUDE_SECURITY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --include-baseline)
      INCLUDE_BASELINE=true
      shift
      ;;
    --include-security)
      INCLUDE_SECURITY=true
      shift
      ;;
    --all)
      INCLUDE_BASELINE=true
      INCLUDE_SECURITY=true
      shift
      ;;
    *)
      echo "Unknown flag: $1"
      echo "Usage: $0 [tier] [mode] [--include-baseline] [--include-security] [--all]"
      exit 1
      ;;
  esac
done

echo "=== SoloDawn Quality Gate ==="
echo "Project root: $PROJECT_ROOT"
echo "Tier: $TIER"
echo "Mode: $MODE"
echo ""

cd "$PROJECT_ROOT"

# Run the quality engine via cargo
cargo run --package quality -- \
  --project-root "$PROJECT_ROOT" \
  --tier "$TIER" \
  --mode "$MODE"

EXIT_CODE=$?

if [[ $EXIT_CODE -ne 0 ]]; then
  echo ""
  echo "Quality gate failed with exit code $EXIT_CODE."
  exit $EXIT_CODE
fi

echo ""
echo "Quality gate completed successfully."

# Run baseline verification if requested
if [[ "$INCLUDE_BASELINE" == "true" ]]; then
  echo ""
  echo "=== Running Baseline Verification ==="
  source "$PROJECT_ROOT/scripts/verify-baseline.sh"
fi

# Run security audit if requested
if [[ "$INCLUDE_SECURITY" == "true" ]]; then
  echo ""
  echo "=== Running Security Audit ==="
  source "$PROJECT_ROOT/scripts/audit-security.sh"
fi

#!/bin/bash
set -euo pipefail

API_BASE="http://localhost:23456/api"
LIMIT=10
OUTPUT_JSON=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --json)
      OUTPUT_JSON=true
      shift
      ;;
    --limit)
      LIMIT="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1"
      echo "Usage: $0 [--json] [--limit <n>]"
      exit 1
      ;;
  esac
done

echo "=== SoloDawn Quality Report ==="
echo ""

RESPONSE="$(curl -s "${API_BASE}/quality/runs?limit=${LIMIT}")"

if [[ "$OUTPUT_JSON" == "true" ]]; then
  echo "$RESPONSE"
  exit 0
fi

# Format as summary table
printf "%-38s %-12s %-14s %-20s\n" "RUN_ID" "STATUS" "ISSUES_COUNT" "CREATED_AT"
printf "%-38s %-12s %-14s %-20s\n" "------" "------" "------------" "----------"

echo "$RESPONSE" | python3 -c "
import json, sys
data = json.load(sys.stdin)
runs = data if isinstance(data, list) else data.get('runs', data.get('data', []))
for r in runs:
    run_id = r.get('run_id', r.get('id', 'N/A'))
    status = r.get('status', 'N/A')
    issues = r.get('issues_count', r.get('issue_count', 0))
    created = r.get('created_at', 'N/A')
    print(f'{run_id:<38} {status:<12} {str(issues):<14} {created:<20}')
" 2>/dev/null || echo "(Could not parse response. Use --json to see raw output.)"

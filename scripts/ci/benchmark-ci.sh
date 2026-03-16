#!/usr/bin/env bash
set -euo pipefail

# CI Benchmark Script
# Records timing for each CI step and produces a JSON summary report.
#
# Usage:
#   benchmark-ci.sh start <step_name>   - Record step start time
#   benchmark-ci.sh end <step_name>     - Record step end time, print duration
#   benchmark-ci.sh report              - Output JSON summary of all steps

CI_BENCHMARK_DIR="${CI_BENCHMARK_DIR:-/tmp/ci-benchmark}"

usage() {
  echo "Usage: $0 {start|end|report} [step_name]"
  echo ""
  echo "Commands:"
  echo "  start <step_name>  Record the start timestamp for a step"
  echo "  end <step_name>    Record the end timestamp, compute and print duration"
  echo "  report             Output a JSON summary of all recorded steps"
  return 1
}

ensure_dir() {
  mkdir -p "${CI_BENCHMARK_DIR}"
  return 0
}

cmd_start() {
  local step_name="$1"
  ensure_dir
  date +%s > "${CI_BENCHMARK_DIR}/${step_name}.start"
  echo "[benchmark] Started step: ${step_name}"
  return 0
}

cmd_end() {
  local step_name="$1"
  local start_file="${CI_BENCHMARK_DIR}/${step_name}.start"

  if [[ ! -f "${start_file}" ]]; then
    echo "Error: No start timestamp found for step '${step_name}'." >&2
    echo "       Did you forget to run: $0 start ${step_name}" >&2
    exit 1
  fi

  local start_ts end_ts duration
  start_ts=$(cat "${start_file}")
  end_ts=$(date +%s)
  duration=$(( end_ts - start_ts ))

  # Append result as a line: name duration_seconds
  echo "${step_name} ${duration}" >> "${CI_BENCHMARK_DIR}/results.txt"

  # Clean up the start marker
  rm -f "${start_file}"

  echo "[benchmark] Finished step: ${step_name} (${duration}s)"

  # If running in GitHub Actions, also append to the step summary
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    echo "| ${step_name} | ${duration}s |" >> "${GITHUB_STEP_SUMMARY}"
  fi
  return 0
}

cmd_report() {
  local results_file="${CI_BENCHMARK_DIR}/results.txt"

  if [[ ! -f "${results_file}" ]]; then
    echo '{"steps":[],"total_seconds":0}'
    return
  fi

  local total=0
  local steps_json=""
  local first=true

  while IFS=' ' read -r name duration; do
    total=$(( total + duration ))
    if [[ "${first}" == "true" ]]; then
      first=false
    else
      steps_json="${steps_json},"
    fi
    steps_json="${steps_json}
    {\"name\":\"${name}\",\"duration_seconds\":${duration}}"
  done < "${results_file}"

  local report
  report=$(cat <<ENDJSON
{
  "steps": [${steps_json}
  ],
  "total_seconds": ${total}
}
ENDJSON
)

  echo "${report}"

  # If running in GitHub Actions, write the full report to the step summary
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    {
      echo ""
      echo "### CI Benchmark Report"
      echo ""
      echo "| Step | Duration |"
      echo "|------|----------|"
      while IFS=' ' read -r name duration; do
        echo "| ${name} | ${duration}s |"
      done < "${results_file}"
      echo ""
      echo "**Total: ${total}s**"
    } >> "${GITHUB_STEP_SUMMARY}"
  fi
}

# --- Main ---

if [[ $# -lt 1 ]]; then
  usage
fi

command="$1"
shift

case "${command}" in
  start)
    if [[ $# -lt 1 ]]; then
      echo "Error: 'start' requires a step name." >&2
      usage
    fi
    cmd_start "$1"
    ;;
  end)
    if [[ $# -lt 1 ]]; then
      echo "Error: 'end' requires a step name." >&2
      usage
    fi
    cmd_end "$1"
    ;;
  report)
    cmd_report
    ;;
  *)
    echo "Error: Unknown command '${command}'." >&2
    usage
    ;;
esac

exit 0

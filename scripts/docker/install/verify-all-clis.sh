#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

log_info "=== CLI Verification ==="

TOTAL=0; OK=0; REQUIRED_FAIL=0

check_required() {
    local name="$1"; shift
    TOTAL=$((TOTAL + 1))
    if verify_cli "$name" "$@"; then
        OK=$((OK + 1))
    else
        REQUIRED_FAIL=$((REQUIRED_FAIL + 1))
    fi
    return 0
}

check_optional() {
    local name="$1"; shift
    TOTAL=$((TOTAL + 1))
    if verify_cli "$name" "$@"; then OK=$((OK + 1)); fi
    return 0
}

check_required "Claude Code"  claude --version
check_required "Codex CLI"    codex --version

check_optional "Qwen Code"    qwen --version
check_optional "Amp"          amp --version
check_optional "OpenCode"     opencode --version
check_optional "Droid"        droid --version
check_optional "Copilot"      copilot --version

log_info "=== Verification: $OK/$TOTAL available, required failures: $REQUIRED_FAIL ==="

if (( REQUIRED_FAIL > 0 )); then
    exit 1
fi

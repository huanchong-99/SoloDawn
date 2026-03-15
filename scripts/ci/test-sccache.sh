#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────
# test-sccache.sh — Verify sccache integration is working
# ─────────────────────────────────────────────────────────────

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
TOTAL_CHECKS=0

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
  echo -e "  ${GREEN}PASS${RESET}  $1"
}

fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
  echo -e "  ${RED}FAIL${RESET}  $1"
}

info() {
  echo -e "  ${CYAN}INFO${RESET}  $1"
}

section() {
  echo ""
  echo -e "${BOLD}[$1]${RESET}"
}

# ─────────────────────────────────────────────────────────────
# 1. Check sccache binary availability
# ─────────────────────────────────────────────────────────────
section "1/5  Checking sccache binary"

if command -v sccache &>/dev/null; then
  SCCACHE_PATH="$(command -v sccache)"
  SCCACHE_VER="$(sccache --version 2>/dev/null || echo 'unknown')"
  pass "sccache found at ${SCCACHE_PATH} (${SCCACHE_VER})"
else
  fail "sccache not found in PATH"
  echo -e "\n${RED}${BOLD}Cannot continue without sccache. Aborting.${RESET}"
  exit 1
fi

# ─────────────────────────────────────────────────────────────
# 2. Check environment variables
# ─────────────────────────────────────────────────────────────
section "2/5  Checking environment variables"

if [[ "${RUSTC_WRAPPER:-}" == *"sccache"* ]]; then
  pass "RUSTC_WRAPPER is set to '${RUSTC_WRAPPER}'"
else
  fail "RUSTC_WRAPPER is '${RUSTC_WRAPPER:-<unset>}' (expected sccache)"
fi

if [[ "${SCCACHE_GHA_ENABLED:-}" == "true" ]]; then
  pass "SCCACHE_GHA_ENABLED is 'true'"
else
  fail "SCCACHE_GHA_ENABLED is '${SCCACHE_GHA_ENABLED:-<unset>}' (expected true)"
fi

# ─────────────────────────────────────────────────────────────
# 3. Capture initial sccache stats
# ─────────────────────────────────────────────────────────────
section "3/5  Capturing initial sccache stats"

# Extract compile-request count from sccache stats.
# The output format varies between versions; try to find a
# "Compile requests" or "compile_requests" line. We fall back
# to 0 if parsing fails so the rest of the script still works.
parse_requests() {
  local stats_output="$1"
  # Try "Compile requests" (human-readable output)
  local count
  count=$(echo "$stats_output" | grep -i 'compile requests' | head -1 | grep -oE '[0-9]+' | head -1 || true)
  if [[ -z "$count" ]]; then
    # Try "Cache hits" as alternative metric
    count=$(echo "$stats_output" | grep -i 'cache hits' | head -1 | grep -oE '[0-9]+' | head -1 || true)
  fi
  echo "${count:-0}"
}

parse_cache_hits() {
  local stats_output="$1"
  local count
  count=$(echo "$stats_output" | grep -i 'cache hits' | head -1 | grep -oE '[0-9]+' | head -1 || true)
  echo "${count:-0}"
}

STATS_BEFORE="$(sccache --show-stats 2>&1)" || true
REQUESTS_BEFORE=$(parse_requests "$STATS_BEFORE")
HITS_BEFORE=$(parse_cache_hits "$STATS_BEFORE")

info "Compile requests before: ${REQUESTS_BEFORE}"
info "Cache hits before:       ${HITS_BEFORE}"
pass "Initial stats captured"

# ─────────────────────────────────────────────────────────────
# 4. Run a lightweight compile test
# ─────────────────────────────────────────────────────────────
section "4/5  Running compile test (cargo check -p utils)"

# Clean the target for the specific crate so we force recompilation
info "Cleaning utils crate artifacts..."
cargo clean -p utils 2>/dev/null || true

info "Running cargo check -p utils..."
if cargo check -p utils 2>&1; then
  pass "cargo check -p utils succeeded"
else
  fail "cargo check -p utils failed"
fi

# ─────────────────────────────────────────────────────────────
# 5. Compare sccache stats
# ─────────────────────────────────────────────────────────────
section "5/5  Comparing sccache stats after compilation"

STATS_AFTER="$(sccache --show-stats 2>&1)" || true
REQUESTS_AFTER=$(parse_requests "$STATS_AFTER")
HITS_AFTER=$(parse_cache_hits "$STATS_AFTER")

info "Compile requests after:  ${REQUESTS_AFTER}"
info "Cache hits after:        ${HITS_AFTER}"

if [[ "$REQUESTS_AFTER" -gt "$REQUESTS_BEFORE" ]]; then
  pass "Compile requests increased (${REQUESTS_BEFORE} -> ${REQUESTS_AFTER})"
else
  fail "Compile requests did not increase (${REQUESTS_BEFORE} -> ${REQUESTS_AFTER}). sccache may not be intercepting rustc calls."
fi

if [[ "$HITS_AFTER" -gt "$HITS_BEFORE" ]]; then
  pass "Cache hits increased (${HITS_BEFORE} -> ${HITS_AFTER}) — cache is warm"
else
  info "Cache hits did not increase (${HITS_BEFORE} -> ${HITS_AFTER}) — this is normal on a cold cache"
fi

# ─────────────────────────────────────────────────────────────
# Summary
# ─────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "${BOLD}  Summary: ${TOTAL_CHECKS} checks, ${GREEN}${PASS_COUNT} passed${RESET}, ${RED}${FAIL_COUNT} failed${RESET}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo -e "\n${RED}${BOLD}RESULT: FAIL${RESET} — ${FAIL_COUNT} check(s) did not pass."
  exit 1
else
  echo -e "\n${GREEN}${BOLD}RESULT: PASS${RESET} — sccache integration is working correctly."
  exit 0
fi

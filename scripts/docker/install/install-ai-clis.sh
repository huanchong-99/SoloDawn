#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

trap 'log_error "Installation failed at line $LINENO"' ERR

require_command node
require_command npm

log_info "=== SoloDawn AI CLI Installation ==="

CORE_CLIS=(
    "${CLAUDE_CODE_NPM_PKG:-@anthropic-ai/claude-code}"
    "${CODEX_NPM_PKG:-@openai/codex}"
)

EXTENDED_CLIS=(
    "${QWEN_NPM_PKG:-@qwen-code/qwen-code@latest}"
    "${AMP_NPM_PKG:-@ampcode/cli@latest}"
    "${OPENCODE_NPM_PKG:-opencode-ai@latest}"
    "${DROID_NPM_PKG:-droid}"
    "${COPILOT_NPM_PKG:-@github/copilot}"
)

FAILED=0

log_info "--- Installing core CLIs ---"
for pkg in "${CORE_CLIS[@]}"; do
    npm_install_global "$pkg" 3 || FAILED=$((FAILED + 1))
done

log_info "--- Installing extended CLIs (best-effort) ---"
for pkg in "${EXTENDED_CLIS[@]}"; do
    npm_install_global "$pkg" 2 || log_warn "Skipping optional: $pkg"
done

log_info "=== Installation complete (core failures: $FAILED) ==="
bash "$SCRIPT_DIR/verify-all-clis.sh" || FAILED=$((FAILED + 1))
exit $FAILED

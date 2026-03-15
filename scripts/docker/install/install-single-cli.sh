#!/usr/bin/env bash
# Install or uninstall a single AI CLI tool.
# Usage: install-single-cli.sh <action> <cli_name>
#   action: install | uninstall
#   cli_name: claude-code | codex | gemini-cli | amp | cursor-agent | qwen-code | copilot | opencode | droid
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

trap 'log_error "Operation failed at line $LINENO"' ERR

# --- Argument validation ---

if [[ $# -lt 2 ]]; then
    log_error "Usage: install-single-cli.sh <action> <cli_name>"
    log_error "  action:   install | uninstall"
    log_error "  cli_name: claude-code | codex | gemini-cli | amp | cursor-agent | qwen-code | copilot | opencode | droid"
    exit 1
fi

ACTION="$1"
CLI_NAME="$2"

if [[ "$ACTION" != "install" && "$ACTION" != "uninstall" ]]; then
    log_error "Invalid action: $ACTION (must be 'install' or 'uninstall')"
    exit 1
fi

# --- Package mapping ---
# Maps cli_name -> npm package (or special handler for copilot)
# Supports env var overrides matching the existing install-ai-clis.sh pattern.

resolve_package() {
    local cli="$1"
    case "$cli" in
        claude-code)    echo "${CLAUDE_CODE_NPM_PKG:-@anthropic-ai/claude-code}" ;;
        codex)          echo "${CODEX_NPM_PKG:-@openai/codex}" ;;
        gemini-cli)     echo "${GEMINI_NPM_PKG:-@google/gemini-cli}" ;;
        amp)            echo "${AMP_NPM_PKG:-@sourcegraph/amp}" ;;
        qwen-code)      echo "${QWEN_NPM_PKG:-@qwen-code/qwen-code}" ;;
        opencode)       echo "${OPENCODE_NPM_PKG:-opencode-ai}" ;;
        droid)          echo "${KILOCODE_NPM_PKG:-@kilocode/cli}" ;;
        cursor-agent)   echo "${CURSOR_AGENT_NPM_PKG:-cursor-agent}" ;;
        copilot)        echo "__gh_extension__" ;;
        *)
            log_error "Unknown CLI name: $cli"
            exit 1
            ;;
    esac
}

# --- Detect command mapping (for post-install verification) ---

detect_command_for() {
    local cli="$1"
    case "$cli" in
        claude-code)    echo "claude --version" ;;
        codex)          echo "codex --version" ;;
        gemini-cli)     echo "gemini --version" ;;
        amp)            echo "amp --version" ;;
        qwen-code)      echo "qwen --version" ;;
        opencode)       echo "opencode --version" ;;
        droid)          echo "droid --version" ;;
        cursor-agent)   echo "cursor-agent --version" ;;
        copilot)        echo "gh copilot --version" ;;
        *)              echo "" ;;
    esac
}

# --- Validate cli_name against whitelist ---

VALID_CLIS="claude-code codex gemini-cli amp cursor-agent qwen-code copilot opencode droid"

validate_cli_name() {
    local cli="$1"
    local valid
    for valid in $VALID_CLIS; do
        if [[ "$cli" == "$valid" ]]; then
            return 0
        fi
    done
    log_error "Invalid CLI name: $cli"
    log_error "Valid names: $VALID_CLIS"
    exit 1
}

validate_cli_name "$CLI_NAME"

# --- Copilot (gh extension) handlers ---

install_copilot() {
    if ! command -v gh >/dev/null 2>&1; then
        log_error "gh CLI not found; cannot install gh-copilot extension"
        return 1
    fi

    : "${GH_EXTENSIONS_DIR:=/opt/gitcortex/gh-extensions}"
    export GH_EXTENSIONS_DIR
    mkdir -p "$GH_EXTENSIONS_DIR"

    if gh extension list 2>/dev/null | awk '{print $1}' | grep -Fxq "github/gh-copilot"; then
        log_info "GitHub Copilot extension already installed"
        return 0
    fi

    log_info "Installing GitHub Copilot CLI extension..."
    gh extension install github/gh-copilot 2>&1
}

uninstall_copilot() {
    if ! command -v gh >/dev/null 2>&1; then
        log_error "gh CLI not found; cannot uninstall gh-copilot extension"
        return 1
    fi

    : "${GH_EXTENSIONS_DIR:=/opt/gitcortex/gh-extensions}"
    export GH_EXTENSIONS_DIR

    log_info "Removing GitHub Copilot CLI extension..."
    gh extension remove github/gh-copilot 2>&1
}

# --- Main logic ---

PKG="$(resolve_package "$CLI_NAME")"

if [[ "$ACTION" == "install" ]]; then
    log_info "Installing CLI: $CLI_NAME"

    if [[ "$PKG" == "__gh_extension__" ]]; then
        install_copilot
    else
        require_command node
        require_command npm
        npm_install_global "$PKG" 3
    fi

    # Post-install verification
    DETECT_CMD="$(detect_command_for "$CLI_NAME")"
    if [[ -n "$DETECT_CMD" ]]; then
        log_info "Verifying installation..."
        # shellcheck disable=SC2086
        if output=$(eval $DETECT_CMD 2>&1); then
            version="${output%%$'\n'*}"
            log_info "Verified $CLI_NAME: ${version:-installed}"
        else
            log_warn "Verification failed for $CLI_NAME (command may need PATH refresh)"
        fi
    fi

    log_info "Install complete: $CLI_NAME"

elif [[ "$ACTION" == "uninstall" ]]; then
    log_info "Uninstalling CLI: $CLI_NAME"

    if [[ "$PKG" == "__gh_extension__" ]]; then
        uninstall_copilot
    else
        require_command npm

        # Strip version specifier for uninstall (e.g., @latest, @1.2.3)
        # For scoped packages like @scope/name@version, strip only the trailing @version
        UNINSTALL_PKG="$PKG"
        if [[ "$UNINSTALL_PKG" == @*/* ]]; then
            # Scoped package: @scope/name@version -> @scope/name
            UNINSTALL_PKG="$(echo "$UNINSTALL_PKG" | sed 's/\(@[^/]*\/[^@]*\)@.*/\1/')"
        else
            # Unscoped package: name@version -> name
            UNINSTALL_PKG="${UNINSTALL_PKG%%@*}"
        fi

        log_info "Running: npm uninstall -g $UNINSTALL_PKG"
        npm uninstall -g "$UNINSTALL_PKG" 2>&1
    fi

    log_info "Uninstall complete: $CLI_NAME"
fi

exit 0

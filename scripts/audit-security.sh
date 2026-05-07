#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR" || exit 1

failures=0

note() {
  echo "==> $*"
  return 0
}

fail() {
  echo "FAIL: $*"
  failures=$((failures + 1))
  return 0
}

SEARCH_TOOL="rg"
if ! command -v rg >/dev/null 2>&1; then
  SEARCH_TOOL="grep"
fi

search() {
  local pattern="$1"
  shift
  if [[ "$SEARCH_TOOL" = "rg" ]]; then
    rg -n "$pattern" "$@"
  else
    grep -R -n -E "$pattern" "$@"
  fi
  return 0
}

note "Check: API key encryption"
if ! search "#\[serde\(skip\)\]" crates/db/src/models/workflow.rs >/dev/null 2>&1; then
  fail "orchestrator_api_key is missing #[serde(skip)] in crates/db/src/models/workflow.rs"
fi
if ! search "fn set_api_key" crates/db/src/models/workflow.rs >/dev/null 2>&1; then
  fail "set_api_key not found in crates/db/src/models/workflow.rs"
fi
if ! search "fn get_api_key" crates/db/src/models/workflow.rs >/dev/null 2>&1; then
  fail "get_api_key not found in crates/db/src/models/workflow.rs"
fi
if ! search "SOLODAWN_ENCRYPTION_KEY" crates/db/src/models/workflow.rs >/dev/null 2>&1; then
  fail "SOLODAWN_ENCRYPTION_KEY not referenced in crates/db/src/models/workflow.rs"
fi
if ! search "set_api_key" crates/server/src/routes/workflows.rs >/dev/null 2>&1; then
  fail "Workflow API does not use set_api_key for encryption"
fi

note "Check: Logging security"
log_hits="$(search "(tracing::)?(trace|debug|info|warn|error)!\([^\n]*?(api[_-]?key|token|secret|authorization)" crates 2>/dev/null || true)"
if [[ -n "$log_hits" ]]; then
  fail "Potential secret logging detected"
  echo "$log_hits"
fi

note "Check: Git security (.env in .gitignore)"
if ! search "^\.env(\..*)?$" .gitignore >/dev/null 2>&1; then
  fail ".env is not ignored in .gitignore"
fi
if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  tracked_env="$(git ls-files ".env" ".env.*")"
  if [[ -n "$tracked_env" ]]; then
    fail ".env files are tracked by git"
    echo "$tracked_env"
  fi
fi

note "Check: Hardcoded secrets"
if [[ "$SEARCH_TOOL" = "rg" ]]; then
  SECRET_EXCLUDES=(--glob "!docs/**" --glob "!tests/**" --glob "!README.md" --glob "!.git/**")
else
  SECRET_EXCLUDES=(--exclude-dir=docs --exclude-dir=tests --exclude=README.md --exclude-dir=.git)
fi

secret_patterns=(
  "AKIA[0-9A-Z]{16}"
  "ASIA[0-9A-Z]{16}"
  "ghp_[A-Za-z0-9]{36}"
  "github_pat_[A-Za-z0-9_]{20,}"
  "sk-[A-Za-z0-9]{16,}"
  "xox[baprs]-[A-Za-z0-9-]{10,}"
  "AIza[0-9A-Za-z\-_]{35}"
  "-----BEGIN (RSA|EC|OPENSSH) PRIVATE KEY-----"
)

for pattern in "${secret_patterns[@]}"; do
  hits="$(search "$pattern" crates "${SECRET_EXCLUDES[@]}" 2>/dev/null || true)"
  if [[ -n "$hits" ]]; then
    fail "Hardcoded secret pattern detected: $pattern"
    echo "$hits"
  fi
done

note "Check: Encryption implementation (AES-GCM)"
if ! search "Aes256Gcm" crates/db/src/models/workflow.rs >/dev/null 2>&1; then
  fail "Aes256Gcm usage not found in crates/db/src/models/workflow.rs"
fi
if ! search "\.encrypt\(" crates/db/src/models/workflow.rs >/dev/null 2>&1; then
  fail "encrypt() call not found in crates/db/src/models/workflow.rs"
fi
if ! search "\.decrypt\(" crates/db/src/models/workflow.rs >/dev/null 2>&1; then
  fail "decrypt() call not found in crates/db/src/models/workflow.rs"
fi

if [[ "$failures" -ne 0 ]]; then
  echo "Security audit failed: $failures issue(s)"
  exit 1
fi

echo "Security audit passed"

#!/usr/bin/env bash
# init-sonar.sh — Initialize SonarQube for SoloDawn
# Usage: ./init-sonar.sh [sonar_url] [admin_password]
set -euo pipefail

SONAR_URL="${1:-http://localhost:9000}"
# Require admin password from CLI arg or SONAR_ADMIN_PASSWORD env; no insecure default.
ADMIN_PASS="${2:-${SONAR_ADMIN_PASSWORD:-}}"
if [[ -z "$ADMIN_PASS" ]]; then
    echo "[init-sonar] ERROR: SonarQube admin password is not set. Provide it as the second argument or via SONAR_ADMIN_PASSWORD." >&2
    exit 2
fi
PROJECT_KEY="solodawn"
PROJECT_NAME="SoloDawn"
MAX_WAIT=300
POLL_INTERVAL=5

# ── Step 1: Wait for SonarQube to be healthy ──────────────────────
echo "[init-sonar] Waiting for SonarQube at ${SONAR_URL} ..."
elapsed=0
while true; do
    status=$(curl -sf "${SONAR_URL}/api/system/status" 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)
    if [[ "$status" = "UP" ]]; then
        echo "[init-sonar] SonarQube is UP (${elapsed}s)"
        break
    fi
    if [[ "$elapsed" -ge "$MAX_WAIT" ]]; then
        echo "[init-sonar] ERROR: SonarQube not ready after ${MAX_WAIT}s (status=${status:-unknown})" >&2
        exit 1
    fi
    sleep "$POLL_INTERVAL"
    elapsed=$((elapsed + POLL_INTERVAL))
done

# ── Step 2: Create project if it doesn't exist ────────────────────
echo "[init-sonar] Checking project '${PROJECT_KEY}' ..."
project_exists=$(curl -sf -u "admin:${ADMIN_PASS}" \
    "${SONAR_URL}/api/projects/search?projects=${PROJECT_KEY}" \
    | grep -c "\"key\":\"${PROJECT_KEY}\"" || true)

if [[ "$project_exists" -eq 0 ]]; then
    echo "[init-sonar] Creating project '${PROJECT_KEY}' ..."
    curl -sf -u "admin:${ADMIN_PASS}" -X POST \
        "${SONAR_URL}/api/projects/create" \
        -d "project=${PROJECT_KEY}&name=${PROJECT_NAME}" \
        > /dev/null
    echo "[init-sonar] Project created."
else
    echo "[init-sonar] Project already exists, skipping."
fi

# ── Step 3: Set up quality profiles ───────────────────────────────
echo "[init-sonar] Configuring quality profiles ..."
for lang in ts js; do
    # Use the built-in "Sonar way" profile as default
    curl -sf -u "admin:${ADMIN_PASS}" -X POST \
        "${SONAR_URL}/api/qualityprofiles/set_default" \
        -d "language=${lang}&qualityProfile=Sonar%20way" \
        > /dev/null 2>&1 || echo "[init-sonar] WARN: Could not set default profile for ${lang}" >&2
done
echo "[init-sonar] Quality profiles configured."

# ── Step 4: Configure webhook for quality gate results ────────────
WEBHOOK_NAME="SoloDawn Quality Gate"
WEBHOOK_URL="http://solodawn:23456/api/webhooks/sonar"

echo "[init-sonar] Checking webhooks ..."
webhook_exists=$(curl -sf -u "admin:${ADMIN_PASS}" \
    "${SONAR_URL}/api/webhooks/list?project=${PROJECT_KEY}" \
    | grep -c "${WEBHOOK_URL}" || true)

if [[ "$webhook_exists" -eq 0 ]]; then
    echo "[init-sonar] Creating webhook -> ${WEBHOOK_URL} ..."
    curl -sf -u "admin:${ADMIN_PASS}" -X POST \
        "${SONAR_URL}/api/webhooks/create" \
        -d "name=${WEBHOOK_NAME}&project=${PROJECT_KEY}&url=${WEBHOOK_URL}" \
        > /dev/null
    echo "[init-sonar] Webhook created."
else
    echo "[init-sonar] Webhook already exists, skipping."
fi

echo "[init-sonar] Initialization complete."

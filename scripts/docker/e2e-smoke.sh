#!/usr/bin/env bash
set -euo pipefail

echo "=== GitCortex Docker E2E Smoke Test ==="

COMPOSE_FILE="docker/compose/docker-compose.dev.yml"
BASE_URL="http://localhost:23456"
MAX_WAIT=120

cleanup() { docker compose -f "$COMPOSE_FILE" down 2>/dev/null; return 0; }
trap cleanup EXIT

echo "Building and starting container..."
docker compose -f "$COMPOSE_FILE" up -d --build

echo "Waiting for /healthz (max ${MAX_WAIT}s)..."
elapsed=0
while [[ $elapsed -lt $MAX_WAIT ]]; do
    if curl -sf "$BASE_URL/healthz" > /dev/null 2>&1; then
        echo "OK /healthz after ${elapsed}s"
        break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
done
if [[ $elapsed -ge $MAX_WAIT ]]; then
    echo "FAIL: Timeout waiting for /healthz"
    docker compose -f "$COMPOSE_FILE" logs
    exit 1
fi

echo "Checking /readyz..."
READYZ=$(curl -s "$BASE_URL/readyz" || true)
echo "readyz: $READYZ"
if ! echo "$READYZ" | grep -q '"ready":true'; then
    echo "FAIL: /readyz not ready"
    docker compose -f "$COMPOSE_FILE" logs
    exit 1
fi
echo "OK /readyz ready=true"

echo "Checking frontend..."
HTTP_CODE=$(curl -so /dev/null -w "%{http_code}" "$BASE_URL/")
if [[ "$HTTP_CODE" != "200" ]]; then
    echo "FAIL: Frontend returned $HTTP_CODE"
    docker compose -f "$COMPOSE_FILE" logs
    exit 1
fi
echo "OK frontend 200"

echo "Checking planning-drafts API..."
PD_CODE=$(curl -so /dev/null -w "%{http_code}" "$BASE_URL/api/planning-drafts?project_id=00000000-0000-0000-0000-000000000000")
if [[ "$PD_CODE" != "200" ]]; then
    echo "FAIL: planning-drafts API returned $PD_CODE"
    docker compose -f "$COMPOSE_FILE" logs
    exit 1
fi
echo "OK planning-drafts API 200"

echo "Testing restart persistence..."
docker compose -f "$COMPOSE_FILE" restart
elapsed=0
while [[ $elapsed -lt 30 ]]; do
    if curl -sf "$BASE_URL/healthz" > /dev/null 2>&1; then
        echo "OK recovered after restart (${elapsed}s)"
        break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
done
if [[ $elapsed -ge 30 ]]; then
    echo "FAIL: No recovery after restart"
    docker compose -f "$COMPOSE_FILE" logs
    exit 1
fi

echo "=== Smoke test PASSED ==="

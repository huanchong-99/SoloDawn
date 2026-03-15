#!/usr/bin/env bash
# cleanup-quality-data.sh — Retention cleanup for quality data, logs, and disk
# Usage: ./cleanup-quality-data.sh [db_path] [log_dir]
#
# Schedule via cron (host) or a one-shot container:
#   0 3 * * 0  /path/to/cleanup-quality-data.sh /var/lib/gitcortex/data.db /var/log/gitcortex
set -euo pipefail

DB_PATH="${1:-/var/lib/gitcortex/data.db}"
LOG_DIR="${2:-/var/log/gitcortex}"
RETENTION_DAYS=90
LOG_COMPRESS_DAYS=7

echo "=== Quality Data Cleanup ==="
echo "  DB path       : ${DB_PATH}"
echo "  Log dir       : ${LOG_DIR}"
echo "  DB retention  : ${RETENTION_DAYS} days"
echo "  Log compress  : ${LOG_COMPRESS_DAYS} days"
echo ""

# ── Step 1: SQLite DB cleanup ─────────────────────────────────────
if [[ -f "${DB_PATH}" ]]; then
    echo "[cleanup] Cleaning quality_run records older than ${RETENTION_DAYS} days ..."
    cutoff=$(date -u -d "-${RETENTION_DAYS} days" +"%Y-%m-%dT%H:%M:%S" 2>/dev/null \
        || date -u -v-${RETENTION_DAYS}d +"%Y-%m-%dT%H:%M:%S")

    sqlite3 "${DB_PATH}" "DELETE FROM quality_issue WHERE quality_run_id IN (SELECT id FROM quality_run WHERE created_at < '${cutoff}');"

    deleted=$(sqlite3 "${DB_PATH}" <<SQL
DELETE FROM quality_run WHERE created_at < '${cutoff}';
SELECT changes();
SQL
    )
    echo "[cleanup]   Deleted ${deleted} quality_run rows."

    # Also clean old terminal_log entries (keep 90 days)
    deleted_logs=$(sqlite3 "${DB_PATH}" <<SQL
DELETE FROM terminal_log WHERE created_at < '${cutoff}';
SELECT changes();
SQL
    )
    echo "[cleanup]   Deleted ${deleted_logs} terminal_log rows."

    # Reclaim space
    sqlite3 "${DB_PATH}" "VACUUM;"
    echo "[cleanup]   Database vacuumed."
else
    echo "[cleanup] WARN: Database not found at ${DB_PATH}, skipping." >&2
fi

# ── Step 2: Log rotation ──────────────────────────────────────────
if [[ -d "${LOG_DIR}" ]]; then
    echo "[cleanup] Compressing logs older than ${LOG_COMPRESS_DAYS} days ..."
    compressed=0
    while IFS= read -r -d '' logfile; do
        gzip "${logfile}"
        compressed=$((compressed + 1))
    done < <(find "${LOG_DIR}" -name "*.log" -mtime +${LOG_COMPRESS_DAYS} -print0 2>/dev/null)
    echo "[cleanup]   Compressed ${compressed} log files."

    # Remove compressed logs older than retention period
    removed=0
    while IFS= read -r -d '' gzfile; do
        rm -f "${gzfile}"
        removed=$((removed + 1))
    done < <(find "${LOG_DIR}" -name "*.log.gz" -mtime +${RETENTION_DAYS} -print0 2>/dev/null)
    echo "[cleanup]   Removed ${removed} old compressed logs."
else
    echo "[cleanup] WARN: Log directory not found at ${LOG_DIR}, skipping." >&2
fi

# ── Step 3: Disk usage report ─────────────────────────────────────
echo ""
echo "[cleanup] Disk usage report:"
if [[ -f "${DB_PATH}" ]]; then
    db_size=$(du -h "${DB_PATH}" | cut -f1)
    echo "  Database     : ${db_size}"
fi
if [[ -d "${LOG_DIR}" ]]; then
    log_size=$(du -sh "${LOG_DIR}" | cut -f1)
    echo "  Logs         : ${log_size}"
fi
# Docker volumes (if running inside compose network)
docker_usage=$(docker system df --format '{{.Type}}\t{{.Size}}' 2>/dev/null || echo "  (docker not available)")
echo "  Docker usage :"
echo "${docker_usage}" | sed 's/^/    /'

echo ""
echo "=== Cleanup Complete ==="

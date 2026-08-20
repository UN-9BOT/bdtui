#!/usr/bin/env bash
# Wait for the ChatGPT marker by polling a snapshot file.
#
# The agent loop dumps chrome-devtools__take_snapshot output to a file via
# `scripts/review/dump_snapshot.py > snapshot.txt`, then calls this script.
# Exits 0 on APPROVE / REQUEST_CHANGES, 1 on timeout / no marker.
#
# Usage:
#   scripts/review/wait_marker.sh <sha> <snapshot-file> [interval_seconds] [max_seconds]
set -euo pipefail

SHA="${1:?need sha}"
FILE="${2:?need snapshot file}"
INTERVAL="${3:-15}"
MAX="${4:-600}"

elapsed=0
last_status="PENDING"
while (( elapsed < MAX )); do
    if [[ -s "$FILE" ]]; then
        status="$(scripts/review/parse_marker.py --quiet "$SHA" "$FILE" 2>/dev/null || true)"
        case "$status" in
            APPROVE|REQUEST_CHANGES)
                echo "$status"
                exit 0
                ;;
        esac
        last_status="$status"
    fi
    sleep "$INTERVAL"
    elapsed=$((elapsed + INTERVAL))
done
echo "TIMEOUT (last=$last_status)"
exit 1
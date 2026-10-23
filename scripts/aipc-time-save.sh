#!/bin/bash
# ============================================================================
# AIPC Platform - last-known-time shutdown snapshot
# ============================================================================
# platform-api saves /data/aipc/etc/last-known-time.json periodically while
# running, but a short-lived session (fresh flash, factory bring-up,
# commissioning) may power off before that saver ever fires — firstboot then
# restores a stale timestamp and the wall clock jumps forward on boot. A
# large jump is what ignited the 2026-10-21 journald retention storm, so
# keeping this file as fresh as possible matters more than it looks.
#
# Ordered by aipc-time-save.service to run once at shutdown/reboot. Guarded
# to plausible years: a device shutting down on a dead clock must never
# poison the file with a bogus timestamp.

set -euo pipefail

MIN_PLAUSIBLE_YEAR=2024
DEST="/data/aipc/etc/last-known-time.json"

[[ "$(date +%Y)" -ge "$MIN_PLAUSIBLE_YEAR" ]] || exit 0

mkdir -p "$(dirname "$DEST")"
TZ_NAME="$(timedatectl show -p Timezone --value 2>/dev/null || echo UTC)"
SAVED_AT="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
TS="$(date +%s)"

# Single-line JSON: aipc-firstboot greps '"unix_timestamp": <n>' from this
# file, and any JSON parser handles it either way.
printf '{"unix_timestamp": %s, "timezone": "%s", "saved_at": "%s"}\n' \
    "$TS" "$TZ_NAME" "$SAVED_AT" >"${DEST}.tmp" && mv -f "${DEST}.tmp" "$DEST"

exit 0

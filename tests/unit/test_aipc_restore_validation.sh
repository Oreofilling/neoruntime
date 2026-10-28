#!/bin/bash
# Behavioral test for validate_restored_release in scripts/aipc-restore.sh.
# The function validates fixed /data/aipc paths, so the helpers are extracted
# from the script and the prefix is rewritten into this sandbox.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RESTORE="$ROOT/scripts/aipc-restore.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

{
    sed -n '/^require_nonempty()/,/^}/p' "$RESTORE"
    sed -n '/^require_executable()/,/^}/p' "$RESTORE"
    sed -n '/^validate_restored_release()/,/^}/p' "$RESTORE"
} | sed "s#/data/aipc#${TMP}/data/aipc#g" >"$TMP/validate.sh"

fail() {
    echo "unexpected validation failure: $*" >&2
    exit 1
}
log() { echo "[log] $*"; }
# shellcheck source=/dev/null
source "$TMP/validate.sh"

make_tree() {
    local base="${TMP}/data/aipc" f
    rm -rf "$base"
    mkdir -p "$base/bin" "$base/libexec" "$base/scripts" "$base/systemd" "$base/etc"
    printf 'manifest\n' >"$base/app-manifest.json"
    printf '1.2.0\n' >"$base/VERSION"
    printf '[Unit]\n' >"$base/systemd/aipc-platform.target"
    printf 'listen: 8443\n' >"$base/etc/platform-api.yaml"
    printf 'driver: test\n' >"$base/etc/camera-daemon.yaml"
    for f in bin/platform-api bin/camera-daemon \
        libexec/aipc-compat-check libexec/aipc-os-updater \
        scripts/aipc-install-current-root.sh scripts/aipc-firstboot.sh; do
        printf '#!/bin/sh\n' >"$base/$f"
        chmod 0755 "$base/$f"
    done
}

run_validation() {
    local out
    set +e
    out="$(validate_restored_release 2>&1)"
    STATUS=$?
    OUTPUT="$out"
    set -e
}

# 1. Current layout: the time saver is present, validation passes cleanly.
make_tree
printf '#!/bin/sh\n' >"${TMP}/data/aipc/scripts/aipc-time-save.sh"
chmod 0755 "${TMP}/data/aipc/scripts/aipc-time-save.sh"
run_validation
[[ $STATUS -eq 0 ]] || { echo "FAIL: current layout rejected"; exit 1; }
[[ "$OUTPUT" != *RESTORE_LEGACY_LAYOUT* ]] || { echo "FAIL: current layout warned about legacy runtime"; exit 1; }

# 2. Legacy layout (runtime predates the time saver): warn, do not fail — an
#    OS A/B upgrade may restore a previous runtime release that never shipped
#    the script, and a hard failure there rolls back a healthy upgrade.
make_tree
run_validation
[[ $STATUS -eq 0 ]] || { echo "FAIL: legacy layout rejected"; exit 1; }
[[ "$OUTPUT" == *RESTORE_LEGACY_LAYOUT* ]] || { echo "FAIL: legacy layout did not warn"; exit 1; }

# 3. An empty restored platform binary still fails.
make_tree
printf '#!/bin/sh\n' >"${TMP}/data/aipc/scripts/aipc-time-save.sh"
chmod 0755 "${TMP}/data/aipc/scripts/aipc-time-save.sh"
: >"${TMP}/data/aipc/bin/platform-api"
run_validation
[[ $STATUS -ne 0 ]] || { echo "FAIL: empty platform-api accepted"; exit 1; }

# 4. An empty immutable file under scripts/ still fails.
make_tree
: >"${TMP}/data/aipc/scripts/leftover-empty"
run_validation
[[ $STATUS -ne 0 ]] || { echo "FAIL: empty restored file accepted"; exit 1; }

echo "restore validation tests passed"

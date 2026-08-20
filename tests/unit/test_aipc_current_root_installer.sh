#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
INSTALLER="$ROOT/scripts/aipc-install-current-root.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

DATA="$TMP/data/aipc"
ROOTFS="$TMP/rootfs"
mkdir -p \
    "$DATA/bin" \
    "$DATA/libexec" \
    "$DATA/lib/hal" \
    "$DATA/scripts" \
    "$DATA/systemd" \
    "$DATA/etc/systemd/system.conf.d" \
    "$DATA/etc/systemd/journald.conf.d" \
    "$DATA/etc/sysctl.d" \
    "$DATA/etc/security" \
    "$ROOTFS/etc"

cp "$ROOT/scripts/aipc-compat-check.sh" "$DATA/libexec/aipc-compat-check"
chmod 0755 "$DATA/libexec/aipc-compat-check"
cat >"$DATA/app-manifest.json" <<'EOF'
{
  "app_version": "test",
  "machine": "hailo15-ne503",
  "product": "ne503",
  "min_os_version": "1.12.0",
  "max_os_version": "1.12.0",
  "supported_data_schema": [1],
  "target_data_schema": 1
}
EOF
cat >"$ROOTFS/etc/aipc-os-release" <<'EOF'
OS_VERSION=1.12.0
MACHINE=hailo15-ne503
PRODUCT=ne503
EOF
cp "$ROOTFS/etc/aipc-os-release" "$TMP/os-release.before"
printf 'test\n' >"$DATA/VERSION"
printf '#!/bin/sh\nexit 0\n' >"$DATA/bin/platform-api"
printf '#!/bin/sh\nexit 0\n' >"$DATA/bin/camera-daemon"
printf '#!/bin/sh\nexit 0\n' >"$DATA/libexec/aipc-restore"
printf '#!/bin/sh\nexit 0\n' >"$DATA/libexec/aipc-os-updater"
printf '#!/bin/sh\nexit 0\n' >"$DATA/scripts/aipc-configure-platform-api-gateway.py"
printf '#!/bin/sh\nexit 0\n' >"$DATA/scripts/aipc-install-current-root.sh"
printf '#!/bin/sh\nexit 0\n' >"$DATA/scripts/aipc-firstboot.sh"
chmod 0755 "$DATA/bin/platform-api" "$DATA/bin/camera-daemon" \
    "$DATA/libexec/aipc-restore" "$DATA/libexec/aipc-os-updater" \
    "$DATA/scripts/aipc-configure-platform-api-gateway.py" \
    "$DATA/scripts/aipc-install-current-root.sh" "$DATA/scripts/aipc-firstboot.sh"
printf 'listen: 8443\n' >"$DATA/etc/platform-api.yaml"
printf 'driver: test\n' >"$DATA/etc/camera-daemon.yaml"
printf '[Unit]\nDescription=Test\n' >"$DATA/systemd/aipc-platform.target"
printf '[Manager]\nRuntimeWatchdogSec=20s\n' >"$DATA/etc/systemd/system.conf.d/watchdog.conf"
printf '[Journal]\nStorage=persistent\n' >"$DATA/etc/systemd/journald.conf.d/persist.conf"
printf 'kernel.panic = 10\n' >"$DATA/etc/sysctl.d/panic.conf"
printf '{}\n' >"$DATA/etc/security/seccomp-default.json"

AIPC_INSTALL_ROOT="$DATA" AIPC_ROOTFS_PREFIX="$ROOTFS" "$INSTALLER"

cmp "$TMP/os-release.before" "$ROOTFS/etc/aipc-os-release"
grep -qx 'OS_VERSION=1.12.0' "$ROOTFS/etc/aipc-os-release"
grep -qx 'MACHINE=hailo15-ne503' "$ROOTFS/etc/aipc-os-release"
grep -qx 'PRODUCT=ne503' "$ROOTFS/etc/aipc-os-release"
[[ "$(readlink "$ROOTFS/usr/bin/platform-api")" == "$DATA/bin/platform-api" ]]
[[ "$(readlink "$ROOTFS/usr/bin/camera-daemon")" == "$DATA/bin/camera-daemon" ]]
[[ "$(readlink "$ROOTFS/usr/libexec/aipc-restore")" == "$DATA/libexec/aipc-restore" ]]
[[ "$(readlink "$ROOTFS/usr/libexec/aipc-os-updater")" == "$DATA/libexec/aipc-os-updater" ]]
cmp "$DATA/systemd/aipc-platform.target" "$ROOTFS/etc/systemd/system/aipc-platform.target"
cmp "$DATA/etc/systemd/system.conf.d/watchdog.conf" "$ROOTFS/etc/systemd/system.conf.d/watchdog.conf"
cmp "$DATA/etc/systemd/journald.conf.d/persist.conf" "$ROOTFS/etc/systemd/journald.conf.d/persist.conf"
cmp "$DATA/etc/sysctl.d/panic.conf" "$ROOTFS/etc/sysctl.d/panic.conf"
cmp "$DATA/etc/security/seccomp-default.json" "$ROOTFS/etc/aipc/seccomp-default.json"
grep -qx "$DATA/lib/hal" "$ROOTFS/etc/ld.so.conf.d/aipc.conf"

# A failed rootfs copy must propagate out of the installer. This specifically
# guards the errexit contract used by the generic OS launcher.
mkdir "$TMP/fail-bin"
cat >"$TMP/fail-bin/cp" <<'EOF'
#!/bin/sh
exit 42
EOF
chmod 0755 "$TMP/fail-bin/cp"
if PATH="$TMP/fail-bin:$PATH" AIPC_INSTALL_ROOT="$DATA" AIPC_ROOTFS_PREFIX="$ROOTFS" \
    "$INSTALLER" >/dev/null 2>&1; then
    echo "installer swallowed a rootfs copy failure" >&2
    exit 1
fi

# Missing required release content must be fatal; the launcher must never
# report success after a partial current-root rebuild.
mv "$DATA/libexec/aipc-compat-check" "$DATA/libexec/aipc-compat-check.missing"
if AIPC_INSTALL_ROOT="$DATA" AIPC_ROOTFS_PREFIX="$ROOTFS" "$INSTALLER" >/dev/null 2>&1; then
    echo "installer unexpectedly accepted a missing compatibility checker" >&2
    exit 1
fi
mv "$DATA/libexec/aipc-compat-check.missing" "$DATA/libexec/aipc-compat-check"

# Legacy manifests that only carry required_compat_level (no min/max OS
# version range) must be rejected: the version-range metadata is mandatory.
cat >"$DATA/app-manifest.json" <<'EOF'
{
  "app_version": "test",
  "machine": "hailo15-ne503",
  "product": "ne503",
  "required_compat_level": 1,
  "supported_data_schema": [1],
  "target_data_schema": 1
}
EOF
if AIPC_INSTALL_ROOT="$DATA" AIPC_ROOTFS_PREFIX="$ROOTFS" \
    "$INSTALLER" >"$TMP/legacy.out" 2>"$TMP/legacy.err"; then
    echo "installer unexpectedly accepted a legacy compat-level manifest" >&2
    exit 1
fi
grep -q 'persistent app manifest has incomplete compatibility metadata' "$TMP/legacy.err" || {
    echo "legacy manifest rejection message mismatch:" >&2
    cat "$TMP/legacy.err" >&2
    exit 1
}

echo "test_aipc_current_root_installer: OK"

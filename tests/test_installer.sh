#!/bin/sh
set -eu
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
sh -n "$ROOT/installer/install.sh" "$ROOT/installer/uninstall.sh" "$ROOT/scripts/build-release.sh" "$ROOT/scripts/fetch-ccusage.sh"
grep -q 'Restart=always' "$ROOT/installer/install.sh"
grep -q 'RestartSec=5' "$ROOT/installer/install.sh"
grep -q 'KeepAlive' "$ROOT/installer/install.sh"
grep -q 'Checksum verification failed' "$ROOT/installer/install.sh"
grep -q 'Tên hiển thị trên dashboard' "$ROOT/installer/install.sh"
grep -q -- '--name' "$ROOT/installer/install.sh"
if grep -q -- '--enroll\|--token' "$ROOT/installer/install.sh"; then
  echo "installer still requires a pre-created enrollment token" >&2
  exit 1
fi
grep -q 'log_user_prompt = false' "$ROOT/agent/codex-guard/internal/agent/detect.go"
if grep -R -n 'npx ccusage@latest' "$ROOT" --exclude=README.md --exclude=test_installer.sh; then
  echo "unpinned ccusage invocation found" >&2
  exit 1
fi
echo "installer static checks passed"

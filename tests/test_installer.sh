#!/bin/sh
set -eu
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
sh -n "$ROOT/installer/install.sh" "$ROOT/installer/uninstall.sh" "$ROOT/scripts/build-release.sh" "$ROOT/scripts/fetch-ccusage.sh"
grep -q 'Restart=always' "$ROOT/installer/install.sh"
grep -q 'RestartSec=5' "$ROOT/installer/install.sh"
grep -q 'KeepAlive' "$ROOT/installer/install.sh"
grep -q 'codex-guard-watchdog.timer' "$ROOT/installer/install.sh"
grep -q 'ProtectKernelModules=true' "$ROOT/installer/install.sh"
grep -q 'Checksum verification failed' "$ROOT/installer/install.sh"
grep -q 'Tên hiển thị trên dashboard' "$ROOT/installer/install.sh"
grep -q -- '--name' "$ROOT/installer/install.sh"
if grep -q -- '--enroll\|--token' "$ROOT/installer/install.sh"; then
  echo "installer still requires a pre-created enrollment token" >&2
  exit 1
fi
grep -q 'log_user_prompt = false' "$ROOT/agent/codex-guard/internal/agent/detect.go"
grep -q 'windows-dpapi-machine' "$ROOT/agent/codex-guard/internal/config/secrets_windows.go"
grep -q 'sc.exe create CodexGuard' "$ROOT/installer/install.ps1"
grep -q 'schtasks.exe /Create /TN CodexGuardWatchdog' "$ROOT/installer/install.ps1"
grep -q 'Get-FileHash -Algorithm SHA256' "$ROOT/installer/install.ps1"
grep -q 'Get-InteractiveProfilePath' "$ROOT/installer/install.ps1"
grep -q 'build_agent windows amd64' "$ROOT/scripts/build-release.sh"
grep -q 'fetch_one windows-amd64' "$ROOT/scripts/fetch-ccusage.sh"
if command -v pwsh >/dev/null 2>&1; then
  PS_SCRIPT_PATH="$ROOT/installer/install.ps1" pwsh -NoProfile -NonInteractive -Command '$errors=$null; [System.Management.Automation.Language.Parser]::ParseFile($env:PS_SCRIPT_PATH,[ref]$null,[ref]$errors) > $null; if ($errors.Count) { $errors | Out-String | Write-Error; exit 1 }'
  PS_SCRIPT_PATH="$ROOT/installer/uninstall.ps1" pwsh -NoProfile -NonInteractive -Command '$errors=$null; [System.Management.Automation.Language.Parser]::ParseFile($env:PS_SCRIPT_PATH,[ref]$null,[ref]$errors) > $null; if ($errors.Count) { $errors | Out-String | Write-Error; exit 1 }'
fi
if grep -R -n 'npx ccusage@latest' "$ROOT" --exclude=README.md --exclude=test_installer.sh; then
  echo "unpinned ccusage invocation found" >&2
  exit 1
fi
echo "installer static checks passed"

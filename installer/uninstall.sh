#!/bin/sh
set -eu
if [ "$(id -u)" -ne 0 ]; then echo "Run with sudo." >&2; exit 1; fi
os="$(uname -s)"
if [ "$os" = Linux ]; then
  systemctl disable --now codex-guard-watchdog.timer 2>/dev/null || true
  systemctl stop codex-guard.service 2>/dev/null || true
elif [ "$os" = Darwin ]; then
  launchctl bootout system/com.openai.codex-guard-watchdog 2>/dev/null || true
  launchctl bootout system/com.openai.codex-guard 2>/dev/null || true
fi
/usr/local/bin/codex-guard uninstall-notice --config /etc/codex-guard/config.json 2>/dev/null || true
if [ "$os" = Linux ]; then
  systemctl disable codex-guard.service 2>/dev/null || true
  rm -f /etc/systemd/system/codex-guard.service /etc/systemd/system/codex-guard-watchdog.service /etc/systemd/system/codex-guard-watchdog.timer
  systemctl daemon-reload
elif [ "$os" = Darwin ]; then
  rm -f /Library/LaunchDaemons/com.openai.codex-guard.plist /Library/LaunchDaemons/com.openai.codex-guard-watchdog.plist
fi
rm -f /usr/local/bin/codex-guard /usr/local/lib/codex-guard/ccusage
echo "Codex Classroom Monitor removed. Credentials and integrity state were retained in /etc/codex-guard and /var/lib/codex-guard for recoverability. The dashboard will mark this device UNREACHABLE after its configured timeout."

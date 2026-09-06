#!/bin/sh
set -eu
if [ "$(id -u)" -ne 0 ]; then echo "Run with sudo." >&2; exit 1; fi
os="$(uname -s)"
if [ "$os" = Linux ]; then
  systemctl disable --now codex-guard.service 2>/dev/null || true
  rm -f /etc/systemd/system/codex-guard.service
  systemctl daemon-reload
elif [ "$os" = Darwin ]; then
  launchctl bootout system/com.openai.codex-guard 2>/dev/null || true
  rm -f /Library/LaunchDaemons/com.openai.codex-guard.plist
fi
rm -f /usr/local/bin/codex-guard /usr/local/lib/codex-guard/ccusage
echo "Codex Classroom Monitor removed. Credentials and integrity state were retained in /etc/codex-guard and /var/lib/codex-guard for recoverability. The dashboard will mark this device UNREACHABLE after its configured timeout."

#!/bin/sh
set -eu

SERVER="${CODEX_GUARD_SERVER:-https://codex-classroom-monitor.vercel.app}"
DISPLAY_NAME=""
CODEX_HOME_ARG=""
DEVICE_LABEL=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --server) SERVER="${2:-}"; shift 2 ;;
    --name) DISPLAY_NAME="${2:-}"; shift 2 ;;
    --codex-home) CODEX_HOME_ARG="${2:-}"; shift 2 ;;
    --device-label) DEVICE_LABEL="${2:-}"; shift 2 ;;
    --upgrade) shift ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then echo "Run this installer through sudo." >&2; exit 1; fi
[ -n "$SERVER" ] || { echo "--server cannot be empty." >&2; exit 2; }
SERVER="${SERVER%/}"

os="$(uname -s)"
case "$os" in Linux) os=linux ;; Darwin) os=darwin ;; *) echo "Unsupported OS: $os" >&2; exit 1 ;; esac
arch="$(uname -m)"
case "$arch" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; esac

student_user="${SUDO_USER:-}"
if [ ! -s /etc/codex-guard/config.json ]; then
  suggested_name="$student_user"
  if [ -z "$suggested_name" ] || [ "$suggested_name" = "root" ]; then suggested_name="$(id -un 2>/dev/null || true)"; fi
  if [ "$suggested_name" = "root" ]; then suggested_name=""; fi
  if [ -z "$DISPLAY_NAME" ]; then
    if [ -r /dev/tty ]; then
      if [ -n "$suggested_name" ]; then
        printf 'Tên hiển thị trên dashboard [%s]: ' "$suggested_name" > /dev/tty
      else
        printf 'Tên hiển thị trên dashboard: ' > /dev/tty
      fi
      IFS= read -r DISPLAY_NAME < /dev/tty || true
    fi
    DISPLAY_NAME="${DISPLAY_NAME:-$suggested_name}"
  fi
  [ -n "$DISPLAY_NAME" ] || { echo "Không đọc được tên. Chạy lại với --name \"Tên của bạn\"." >&2; exit 2; }
  [ "${#DISPLAY_NAME}" -le 200 ] || { echo "Tên không được dài quá 200 ký tự." >&2; exit 2; }

  if [ -z "$DEVICE_LABEL" ]; then
    if [ "$os" = darwin ]; then DEVICE_LABEL="$(scutil --get ComputerName 2>/dev/null || hostname 2>/dev/null || true)"; else DEVICE_LABEL="$(hostname 2>/dev/null || true)"; fi
  fi
  DEVICE_LABEL="${DEVICE_LABEL:-Máy của $DISPLAY_NAME}"
  [ "${#DEVICE_LABEL}" -le 200 ] || DEVICE_LABEL="$(printf '%s' "$DEVICE_LABEL" | cut -c1-200)"
fi

if [ -z "$CODEX_HOME_ARG" ]; then
  if [ -n "$student_user" ] && [ "$student_user" != "root" ]; then
    detected_codex_home="$(sudo -u "$student_user" -H sh -lc 'printf %s "${CODEX_HOME:-}"' 2>/dev/null || true)"
    if [ -n "$detected_codex_home" ]; then CODEX_HOME_ARG="$detected_codex_home"; fi
  fi
fi
if [ -z "$CODEX_HOME_ARG" ]; then
  if [ -n "$student_user" ] && [ "$student_user" != "root" ]; then
    if [ "$os" = linux ]; then student_home="$(getent passwd "$student_user" | cut -d: -f6)"; else student_home="$(dscl . -read "/Users/$student_user" NFSHomeDirectory | awk '{print $2}')"; fi
  else
    student_home="${HOME:-}"
  fi
  if [ -z "$student_home" ] || [ "$student_home" = "/root" ] || [ "$student_home" = "/var/root" ]; then echo "Cannot detect the student's home. Re-run with --codex-home /path/to/.codex" >&2; exit 1; fi
  CODEX_HOME_ARG="$student_home/.codex"
fi
case "$CODEX_HOME_ARG" in /*) ;; *) echo "CODEX_HOME must be an absolute path." >&2; exit 1 ;; esac

codex_path="codex"
if [ -n "$student_user" ] && [ "$student_user" != "root" ]; then
  detected_codex="$(sudo -u "$student_user" -H sh -lc 'command -v codex' 2>/dev/null || true)"
  if [ -n "$detected_codex" ]; then codex_path="$detected_codex"; fi
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM
echo "Downloading verified release for $os/$arch..."
curl -fsSL --proto '=https' --tlsv1.2 "$SERVER/downloads/checksums.txt" -o "$tmp/checksums.txt"
for name in "codex-guard-$os-$arch" "ccusage-$os-$arch"; do
  curl -fsSL --proto '=https' --tlsv1.2 "$SERVER/downloads/$name" -o "$tmp/$name"
  expected="$(awk -v n="$name" '$2 == n {print $1}' "$tmp/checksums.txt")"
  [ -n "$expected" ] || { echo "No checksum published for $name" >&2; exit 1; }
  if command -v sha256sum >/dev/null 2>&1; then actual="$(sha256sum "$tmp/$name" | awk '{print $1}')"; else actual="$(shasum -a 256 "$tmp/$name" | awk '{print $1}')"; fi
  [ "$actual" = "$expected" ] || { echo "Checksum verification failed for $name" >&2; exit 1; }
done

# Stop existing execution paths only after all replacement artifacts verify.
# This prevents config/queue sequence races during an in-place update.
if [ -s /etc/codex-guard/config.json ]; then
  if [ "$os" = linux ]; then
    systemctl disable --now codex-guard-watchdog.timer 2>/dev/null || true
    systemctl stop codex-guard.service 2>/dev/null || true
  else
    launchctl bootout system/com.openai.codex-guard-watchdog 2>/dev/null || true
    launchctl bootout system/com.openai.codex-guard 2>/dev/null || true
  fi
fi

install -d -m 0755 /usr/local/lib/codex-guard /etc/codex-guard /var/lib/codex-guard
install -m 0755 "$tmp/codex-guard-$os-$arch" /usr/local/bin/codex-guard
install -m 0755 "$tmp/ccusage-$os-$arch" /usr/local/lib/codex-guard/ccusage

if [ ! -s /etc/codex-guard/config.json ]; then
  echo "Đang thêm $DISPLAY_NAME ($DEVICE_LABEL) lên dashboard..."
  /usr/local/bin/codex-guard enroll --server "$SERVER" --name "$DISPLAY_NAME" --codex-home "$CODEX_HOME_ARG" --codex "$codex_path" --ccusage /usr/local/lib/codex-guard/ccusage --agent /usr/local/bin/codex-guard --state-dir /var/lib/codex-guard --config /etc/codex-guard/config.json --device-label "$DEVICE_LABEL"
fi
/usr/local/bin/codex-guard configure-telemetry --config /etc/codex-guard/config.json
chmod 0600 /etc/codex-guard/config.json
if [ -n "$student_user" ] && [ "$student_user" != "root" ]; then
  chown "$student_user:$(id -gn "$student_user")" "$CODEX_HOME_ARG" "$CODEX_HOME_ARG/config.toml"
  chmod 0700 "$CODEX_HOME_ARG"
  chmod 0600 "$CODEX_HOME_ARG/config.toml"
fi

if [ "$os" = linux ]; then
  install -m 0644 /dev/null /etc/systemd/system/codex-guard.service
  printf '%s\n' '[Unit]' 'Description=Codex Classroom Monitor Agent' 'After=network-online.target' 'Wants=network-online.target' '' '[Service]' 'Type=simple' 'ExecStart=/usr/local/bin/codex-guard run --config /etc/codex-guard/config.json' 'Restart=always' 'RestartSec=5' 'UMask=0077' 'NoNewPrivileges=true' 'PrivateTmp=true' 'PrivateDevices=true' 'ProtectSystem=strict' 'ProtectControlGroups=true' 'ProtectKernelModules=true' 'ProtectKernelTunables=true' 'RestrictSUIDSGID=true' 'LockPersonality=true' 'MemoryDenyWriteExecute=true' 'CapabilityBoundingSet=CAP_DAC_READ_SEARCH' 'ReadWritePaths=/var/lib/codex-guard /etc/codex-guard' "ReadOnlyPaths=$CODEX_HOME_ARG" '' '[Install]' 'WantedBy=multi-user.target' > /etc/systemd/system/codex-guard.service
  printf '%s\n' '[Unit]' 'Description=Codex Classroom Monitor watchdog' 'After=network-online.target' '' '[Service]' 'Type=oneshot' 'ExecStart=/usr/local/bin/codex-guard watchdog --config /etc/codex-guard/config.json' 'UMask=0077' 'NoNewPrivileges=true' > /etc/systemd/system/codex-guard-watchdog.service
  printf '%s\n' '[Unit]' 'Description=Run Codex Classroom Monitor watchdog' '' '[Timer]' 'OnBootSec=2min' 'OnUnitActiveSec=2min' 'Persistent=true' '' '[Install]' 'WantedBy=timers.target' > /etc/systemd/system/codex-guard-watchdog.timer
  systemctl daemon-reload
  systemctl enable --now codex-guard.service
  systemctl enable --now codex-guard-watchdog.timer
else
  plist=/Library/LaunchDaemons/com.openai.codex-guard.plist
  install -m 0644 /dev/null "$plist"
  printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>' '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' '<plist version="1.0"><dict>' '<key>Label</key><string>com.openai.codex-guard</string>' '<key>ProgramArguments</key><array><string>/usr/local/bin/codex-guard</string><string>run</string><string>--config</string><string>/etc/codex-guard/config.json</string></array>' '<key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ThrottleInterval</key><integer>5</integer><key>Umask</key><integer>63</integer>' '<key>StandardOutPath</key><string>/var/log/codex-guard.log</string>' '<key>StandardErrorPath</key><string>/var/log/codex-guard.err.log</string>' '</dict></plist>' > "$plist"
  launchctl bootout system/com.openai.codex-guard 2>/dev/null || true
  launchctl bootstrap system "$plist"
  watchdog_plist=/Library/LaunchDaemons/com.openai.codex-guard-watchdog.plist
  install -m 0644 /dev/null "$watchdog_plist"
  printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>' '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' '<plist version="1.0"><dict>' '<key>Label</key><string>com.openai.codex-guard-watchdog</string>' '<key>ProgramArguments</key><array><string>/usr/local/bin/codex-guard</string><string>watchdog</string><string>--config</string><string>/etc/codex-guard/config.json</string></array>' '<key>RunAtLoad</key><true/><key>StartInterval</key><integer>120</integer><key>Umask</key><integer>63</integer>' '<key>StandardOutPath</key><string>/var/log/codex-guard-watchdog.log</string>' '<key>StandardErrorPath</key><string>/var/log/codex-guard-watchdog.err.log</string>' '</dict></plist>' > "$watchdog_plist"
  launchctl bootout system/com.openai.codex-guard-watchdog 2>/dev/null || true
  launchctl bootstrap system "$watchdog_plist"
fi

sleep 2
heartbeat_status=connected
if ! curl -fsS --proto '=https' --tlsv1.2 "$SERVER/healthz" >/dev/null; then heartbeat_status="queued for retry"; fi

echo ""
echo "Cài đặt Codex Classroom Monitor thành công."
echo ""
/usr/local/bin/codex-guard status --config /etc/codex-guard/config.json
echo "Heartbeat    $heartbeat_status"

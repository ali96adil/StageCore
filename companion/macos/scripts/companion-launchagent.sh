#!/usr/bin/env bash
set -euo pipefail

LABEL="com.stagecore.companion"
DOMAIN="gui/$(id -u)"
APP_DIR="${HOME}/Library/Application Support/StageCore/Companion"
DEFAULT_BIN="${APP_DIR}/bin/stagecore-companion"
DEFAULT_CONFIG="${APP_DIR}/config.json"
BIN="${STAGECORE_COMPANION_BIN:-$DEFAULT_BIN}"
CONFIG="${STAGECORE_COMPANION_CONFIG:-$DEFAULT_CONFIG}"
PLIST="${HOME}/Library/LaunchAgents/${LABEL}.plist"
LOG_DIR="${HOME}/Library/Logs/StageCore"
STDOUT_LOG="${LOG_DIR}/Companion.log"
STDERR_LOG="${LOG_DIR}/Companion-error.log"

usage() {
  cat <<'EOF'
usage: companion-launchagent.sh <install|uninstall|status|restart>

Environment overrides:
  STAGECORE_COMPANION_BIN      Companion executable path
  STAGECORE_COMPANION_CONFIG   Companion config.json path

The LaunchAgent runs in the current logged-in macOS user session and reuses the
existing StageCore Companion config and Keychain identity.
EOF
}

fail() {
  echo "StageCore Companion LaunchAgent: $*" >&2
  exit 1
}

require_macos() {
  [ "$(uname -s)" = "Darwin" ] || fail "macOS is required"
  command -v launchctl >/dev/null 2>&1 || fail "launchctl is required"
  command -v plutil >/dev/null 2>&1 || fail "plutil is required"
}

xml_escape() {
  printf '%s' "$1" | sed     -e 's/&/\&amp;/g'     -e 's/</\&lt;/g'     -e 's/>/\&gt;/g'     -e 's/"/\&quot;/g'     -e "s/'/\&apos;/g"
}

service_loaded() {
  launchctl print "${DOMAIN}/${LABEL}" >/dev/null 2>&1
}

running_unmanaged_pids() {
  pgrep -f "$BIN" 2>/dev/null || true
}

write_plist() {
  mkdir -p "$(dirname "$PLIST")" "$LOG_DIR"
  chmod 700 "$LOG_DIR"

  local bin_xml config_xml stdout_xml stderr_xml
  bin_xml="$(xml_escape "$BIN")"
  config_xml="$(xml_escape "$CONFIG")"
  stdout_xml="$(xml_escape "$STDOUT_LOG")"
  stderr_xml="$(xml_escape "$STDERR_LOG")"

  umask 077
  cat >"$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>${bin_xml}</string>
    <string>--config</string>
    <string>${config_xml}</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>2</integer>
  <key>ProcessType</key>
  <string>Interactive</string>
  <key>StandardOutPath</key>
  <string>${stdout_xml}</string>
  <key>StandardErrorPath</key>
  <string>${stderr_xml}</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
</dict>
</plist>
EOF

  chmod 600 "$PLIST"
  plutil -lint "$PLIST" >/dev/null
}

install_agent() {
  require_macos
  [ -x "$BIN" ] || fail "Companion executable not found or not executable: $BIN"
  [ -f "$CONFIG" ] || fail "Companion config not found: $CONFIG"

  if service_loaded; then
    launchctl bootout "${DOMAIN}/${LABEL}" >/dev/null
  fi

  local pids
  pids="$(running_unmanaged_pids)"
  if [ -n "$pids" ]; then
    fail "an unmanaged Companion process is already running (PID(s): ${pids//$'\n'/, }); stop it before installing the LaunchAgent"
  fi

  write_plist
  launchctl bootstrap "$DOMAIN" "$PLIST"
  launchctl enable "${DOMAIN}/${LABEL}"
  launchctl kickstart -k "${DOMAIN}/${LABEL}"

  sleep 1
  if ! service_loaded; then
    fail "LaunchAgent did not load"
  fi
  echo "StageCore Companion LaunchAgent installed: $PLIST"
  echo "Logs: $STDOUT_LOG"
  echo "Errors: $STDERR_LOG"
}

uninstall_agent() {
  require_macos
  if service_loaded; then
    launchctl bootout "${DOMAIN}/${LABEL}" >/dev/null
  fi
  rm -f "$PLIST"
  echo "StageCore Companion LaunchAgent removed."
  echo "Companion binary, config and Keychain identity were preserved."
}

status_agent() {
  require_macos
  if service_loaded; then
    launchctl print "${DOMAIN}/${LABEL}"
    exit 0
  fi
  echo "StageCore Companion LaunchAgent is not loaded."
  exit 1
}

restart_agent() {
  require_macos
  service_loaded || fail "LaunchAgent is not loaded; run install first"
  launchctl kickstart -k "${DOMAIN}/${LABEL}"
  echo "StageCore Companion LaunchAgent restarted."
}

command="${1:-}"
case "$command" in
  install) install_agent ;;
  uninstall) uninstall_agent ;;
  status) status_agent ;;
  restart) restart_agent ;;
  help|-h|--help|"") usage ;;
  *) usage; fail "unknown command: $command" ;;
esac

#!/bin/sh
QPKG_NAME="docExtractor"
export QNAP_QPKG="$QPKG_NAME"
QPKG_ROOT="$(/sbin/getcfg "$QPKG_NAME" Install_Path -f /etc/config/qpkg.conf)"
BIN="$QPKG_ROOT/docExtractor"
VAR="$QPKG_ROOT/var"
PIDFILE="$VAR/docExtractor.pid"
LOGFILE="$VAR/service.log"
CONF="$QPKG_ROOT/docExtractor.conf"

LISTEN="0.0.0.0:8765"
ROOT="/share/Download/Temp"
BROWSE_ROOT="/share"
SETTINGS_FILE="/etc/config/docExtractor.settings.json"
WORKERS="2"
BUFFER_MIB="8"
MAX_DICT_MIB="512"
COMPRESSION="balanced"
FULL_VERIFY="0"
[ -f "$CONF" ] && . "$CONF"

mkdir -p "$VAR"

log_service() {
  printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >>"$LOGFILE"
}

is_running() {
  [ -f "$PIDFILE" ] || return 1
  PID="$(cat "$PIDFILE" 2>/dev/null)"
  [ -n "$PID" ] || return 1
  kill -0 "$PID" 2>/dev/null || return 1
  if [ -L "/proc/$PID/exe" ] && command -v readlink >/dev/null 2>&1; then
    EXE="$(readlink "/proc/$PID/exe" 2>/dev/null)"
    [ "$EXE" = "$BIN" ] || return 1
  fi
  return 0
}

rotate_log() {
  [ -f "$LOGFILE" ] || return 0
  LOGSIZE="$(wc -c < "$LOGFILE" 2>/dev/null | tr -d ' ')"
  if [ -n "$LOGSIZE" ] && [ "$LOGSIZE" -gt 5242880 ] 2>/dev/null; then
    rm -f "$LOGFILE.1"
    mv -f "$LOGFILE" "$LOGFILE.1"
  fi
}

start() {
  if is_running; then
    return 0
  fi
  rm -f "$PIDFILE"
  rotate_log
  if [ ! -x "$BIN" ]; then
    log_service "ERROR binary not executable: $BIN"
    return 1
  fi
  if ! mkdir -p "$ROOT"; then
    log_service "ERROR cannot create/access ROOT: $ROOT"
    return 1
  fi

  VERIFY_ARG=""
  [ "$FULL_VERIFY" = "1" ] && VERIFY_ARG="--full-verify"
  "$BIN" --listen "$LISTEN" --root "$ROOT" --browse-root "$BROWSE_ROOT" \
    --settings-file "$SETTINGS_FILE" --data-dir "$VAR" \
    --workers "$WORKERS" --buffer-mib "$BUFFER_MIB" --max-dict-mib "$MAX_DICT_MIB" --compression "$COMPRESSION" \
    $VERIFY_ARG >>"$LOGFILE" 2>&1 &
  PID=$!
  echo "$PID" > "$PIDFILE"
  sleep 1
  if ! is_running; then
    log_service "ERROR service exited during startup"
    rm -f "$PIDFILE"
    return 1
  fi
  log_service "service started pid=$PID listen=$LISTEN"
  return 0
}

stop() {
  if is_running; then
    PID="$(cat "$PIDFILE")"
    kill "$PID" 2>/dev/null || true
    i=0
    while kill -0 "$PID" 2>/dev/null && [ "$i" -lt 20 ]; do
      sleep 1
      i=$((i + 1))
    done
    if kill -0 "$PID" 2>/dev/null; then
      log_service "WARN graceful stop timed out; sending SIGKILL pid=$PID"
      kill -9 "$PID" 2>/dev/null || true
    fi
  fi
  rm -f "$PIDFILE"
  return 0
}

case "$1" in
  start) start ;;
  stop) stop ;;
  restart) stop && start ;;
  status) is_running && echo "running" || { echo "stopped"; exit 1; } ;;
  *) echo "Usage: $0 {start|stop|restart|status}"; exit 1 ;;
esac

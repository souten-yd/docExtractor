#!/bin/sh
QPKG_NAME="docExtractor"
export QNAP_QPKG="$QPKG_NAME"
QPKG_ROOT="$(/sbin/getcfg "$QPKG_NAME" Install_Path -f /etc/config/qpkg.conf)"
BIN="$QPKG_ROOT/docExtractor"
VAR="$QPKG_ROOT/var"
PIDFILE="$VAR/docExtractor.pid"
LOGFILE="$VAR/service.log"
CONF="$QPKG_ROOT/docExtractor.conf"

ROOT="/share/Download/Temp"
WORKERS="2"
BUFFER_MIB="8"
COMPRESSION="balanced"
[ -f "$CONF" ] && . "$CONF"

mkdir -p "$VAR"

is_running() {
  [ -f "$PIDFILE" ] || return 1
  PID="$(cat "$PIDFILE" 2>/dev/null)"
  [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null
}

start() {
  is_running && exit 0
  "$BIN" --listen ":8765" --root "$ROOT" --data-dir "$VAR" \
    --workers "$WORKERS" --buffer-mib "$BUFFER_MIB" --compression "$COMPRESSION" \
    >>"$LOGFILE" 2>&1 &
  echo $! > "$PIDFILE"
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
    kill -9 "$PID" 2>/dev/null || true
  fi
  rm -f "$PIDFILE"
}

case "$1" in
  start) start ;;
  stop) stop ;;
  restart) stop; start ;;
  *) echo "Usage: $0 {start|stop|restart}"; exit 1 ;;
esac

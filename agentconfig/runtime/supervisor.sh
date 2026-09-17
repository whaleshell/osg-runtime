#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026 zorneth
# SPDX-License-Identifier: MIT
#
# Agent supervisor: runs harness cycles in once|watch mode.
# Inspired by OpenShell's agent supervisor role; MIT-owned implementation.
set -euo pipefail

RUNTIME_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD_DIR="$(cd "$RUNTIME_DIR/.." && pwd)"
PROMPT_FILE="${OSG_AGENT_PROMPT:-$PAYLOAD_DIR/agent-prompt.md}"
HARNESS="${OSG_AGENT_HARNESS:-cursor}"
ADAPTER="$RUNTIME_DIR/harnesses/$HARNESS/exec.sh"
RUN_MODE="${OSG_AGENT_RUN_MODE:-once}"
POLL_INTERVAL_SECONDS="${OSG_AGENT_POLL_INTERVAL_SECONDS:-900}"
HEARTBEAT_SECONDS="${OSG_AGENT_HEARTBEAT_SECONDS:-60}"
# OpenShell-compatible HOME for harnesses; falls back to persist home.
export HOME="${OSG_AGENT_HOME:-${HOME:-/sandbox/home}}"
export OSG_AGENT_HOME="$HOME"

[[ -f "$PROMPT_FILE" ]] || { echo "osg-agent: missing prompt: $PROMPT_FILE" >&2; exit 1; }
[[ -x "$ADAPTER" || -f "$ADAPTER" ]] || { echo "osg-agent: missing harness adapter: $ADAPTER" >&2; exit 1; }
chmod +x "$ADAPTER" 2>/dev/null || true

case "$RUN_MODE" in
  once|watch) ;;
  *) echo "osg-agent: unsupported run mode: $RUN_MODE" >&2; exit 2 ;;
esac

json_status() {
  local json="$1"
  printf '%s' "$json" | sed -nE 's/.*"status"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/p'
}

json_next_poll() {
  local json="$1"
  printf '%s' "$json" | sed -nE 's/.*"next_poll_seconds"[[:space:]]*:[[:space:]]*([0-9]+).*/\1/p'
}

run_cycle() {
  local cycle="$1"
  local out
  out="$(mktemp)"
  echo "osg-agent: starting $RUN_MODE cycle $cycle harness=$HARNESS home=$HOME" >&2
  set +e
  bash "$ADAPTER" "$PROMPT_FILE" >"$out" 2>&1
  local st=$?
  set -e
  cat "$out" >&2
  local last
  last="$(tail -n 1 "$out" 2>/dev/null || true)"
  rm -f "$out"
  if [[ "$last" == OSG_AGENT_RESULT\ * ]]; then
    local payload="${last#OSG_AGENT_RESULT }"
    local status
    status="$(json_status "$payload")"
    case "$status" in
      complete|terminal_failure)
        echo "osg-agent: $status" >&2
        return 0
        ;;
      waiting|blocked|transient_failure)
        local sleep_s
        sleep_s="$(json_next_poll "$payload")"
        [[ -n "$sleep_s" ]] || sleep_s="$POLL_INTERVAL_SECONDS"
        echo "osg-agent: $status; sleeping ${sleep_s}s" >&2
        sleep "$sleep_s"
        return 10
        ;;
      *)
        echo "osg-agent: unknown result status: $status" >&2
        return "$st"
        ;;
    esac
  fi
  return "$st"
}

cycle=1
while true; do
  set +e
  run_cycle "$cycle"
  rc=$?
  set -e
  if [[ "$RUN_MODE" == "once" ]]; then
    exit "$rc"
  fi
  if [[ "$rc" -eq 0 ]]; then
    exit 0
  fi
  if [[ "$rc" -ne 10 ]]; then
    echo "osg-agent: harness exit $rc; retrying in ${POLL_INTERVAL_SECONDS}s" >&2
    sleep "$POLL_INTERVAL_SECONDS"
  fi
  cycle=$((cycle + 1))
done

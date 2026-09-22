#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026 whaleshell
# SPDX-License-Identifier: MIT
#
# Cursor Agent harness adapter (OpenShell-style: prepare HOME, then run).
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: exec.sh <prompt-file>" >&2
  exit 2
fi

PROMPT_FILE="$1"
export HOME="${WHALESHELL_AGENT_HOME:-${HOME:-/sandbox/home}}"
cd "${WHALESHELL_AGENT_WORKDIR:-/workspace}"

ADAPTER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Prepare ~/.cursor/cli-config.json from /etc/whaleshell/agent-payload (attribution off).
bash "$ADAPTER_DIR/prepare-home.sh"

AGENT_BIN="${WHALESHELL_AGENT_BIN:-}"
if [[ -z "$AGENT_BIN" ]]; then
  for c in agent cursor-agent; do
    if command -v "$c" >/dev/null 2>&1; then
      AGENT_BIN="$(command -v "$c")"
      break
    fi
  done
fi
[[ -n "$AGENT_BIN" ]] || { echo "whaleshell-agent: cursor agent binary not found" >&2; exit 1; }

echo "whaleshell-agent: cursor harness invoking $AGENT_BIN" >&2
set +e
"$AGENT_BIN" -p "$(cat "$PROMPT_FILE")" --output-format text
st=$?
set -e
echo "WHALESHELL_AGENT_RESULT {\"status\":\"complete\",\"reason\":\"cursor_cycle_done\",\"exit\":$st}"
exit "$st"

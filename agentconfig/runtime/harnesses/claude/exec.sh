#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026 whaleshell
# SPDX-License-Identifier: MIT
#
# Claude Code harness adapter (headless one-shot against agent-prompt.md).
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: exec.sh <prompt-file>" >&2
  exit 2
fi

PROMPT_FILE="$1"
export HOME="${WHALESHELL_AGENT_HOME:-${HOME:-/sandbox/home}}"
cd "${WHALESHELL_AGENT_WORKDIR:-/workspace}"

CLAUDE_BIN="${WHALESHELL_CLAUDE_BIN:-}"
if [[ -z "$CLAUDE_BIN" ]]; then
  if command -v claude >/dev/null 2>&1; then
    CLAUDE_BIN="$(command -v claude)"
  fi
fi
[[ -n "$CLAUDE_BIN" ]] || { echo "whaleshell-agent: claude binary not found" >&2; exit 1; }

echo "whaleshell-agent: claude harness invoking $CLAUDE_BIN" >&2
set +e
"$CLAUDE_BIN" -p "$(cat "$PROMPT_FILE")" --output-format text
st=$?
set -e
echo "WHALESHELL_AGENT_RESULT {\"status\":\"complete\",\"reason\":\"claude_cycle_done\",\"exit\":$st}"
exit "$st"

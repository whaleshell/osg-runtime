#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026 zorneth
# SPDX-License-Identifier: MIT
#
# Claude Code harness adapter (headless one-shot against agent-prompt.md).
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: exec.sh <prompt-file>" >&2
  exit 2
fi

PROMPT_FILE="$1"
export HOME="${OSG_AGENT_HOME:-${HOME:-/sandbox/home}}"
cd "${OSG_AGENT_WORKDIR:-/workspace}"

CLAUDE_BIN="${OSG_CLAUDE_BIN:-}"
if [[ -z "$CLAUDE_BIN" ]]; then
  if command -v claude >/dev/null 2>&1; then
    CLAUDE_BIN="$(command -v claude)"
  fi
fi
[[ -n "$CLAUDE_BIN" ]] || { echo "osg-agent: claude binary not found" >&2; exit 1; }

echo "osg-agent: claude harness invoking $CLAUDE_BIN" >&2
set +e
"$CLAUDE_BIN" -p "$(cat "$PROMPT_FILE")" --output-format text
st=$?
set -e
echo "OSG_AGENT_RESULT {\"status\":\"complete\",\"reason\":\"claude_cycle_done\",\"exit\":$st}"
exit "$st"

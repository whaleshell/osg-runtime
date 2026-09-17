#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026 zorneth
# SPDX-License-Identifier: MIT
#
# OpenShell-style harness home prep: materialize Cursor CLI config from
# agent-payload into $HOME (like Codex harness writes ~/.codex/auth.json).
set -euo pipefail

HOME="${OSG_AGENT_HOME:-${HOME:-/sandbox/home}}"
ADAPTER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD_DIR="$(cd "$ADAPTER_DIR/../../.." && pwd)"
SEED="${OSG_CURSOR_CLI_CONFIG:-$PAYLOAD_DIR/cursor/cli-config.json}"
DEST="$HOME/.cursor/cli-config.json"

mkdir -p "$HOME/.cursor"

if [[ ! -f "$SEED" ]]; then
  echo "osg-agent: no cursor cli-config seed at $SEED (skip)" >&2
  exit 0
fi

if [[ -f "$DEST" ]] && command -v python3 >/dev/null 2>&1; then
  python3 - "$DEST" "$SEED" <<'PY'
import json, sys
dest, seed = sys.argv[1], sys.argv[2]
with open(seed) as f:
    s = json.load(f)
try:
    with open(dest) as f:
        d = json.load(f)
except Exception:
    d = {}
if not isinstance(d, dict):
    d = {}
d["attribution"] = s.get("attribution") or {
    "attributeCommitsToAgent": False,
    "attributePRsToAgent": False,
}
if "version" not in d:
    d["version"] = s.get("version", 1)
with open(dest, "w") as f:
    json.dump(d, f, indent=2)
    f.write("\n")
PY
else
  cp "$SEED" "$DEST"
fi

echo "osg-agent: prepared $DEST from payload seed" >&2

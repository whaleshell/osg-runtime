#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026 whaleshell
# SPDX-License-Identifier: MIT
#
# In-sandbox agent entrypoint (OpenShell-style role; MIT original).
set -euo pipefail

RUNTIME_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec bash "$RUNTIME_DIR/supervisor.sh"

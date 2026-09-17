// SPDX-FileCopyrightText: Copyright (c) 2026 zorneth
// SPDX-License-Identifier: MIT

// Package agentconfig stages and installs agent guidance into sandboxes.
//
// Layout (inspired by OpenShell's /etc/openshell/{skills,agent-payload}, MIT-owned):
//
//	/etc/osg/skills/           — static skills + policy advisor notes
//	/etc/osg/agent/            — optional MCP/config payload
//	$HOME/.cursor/skills/…     — symlinks so Cursor Agent discovers skills
//	$HOME/.claude/skills/…     — symlinks for Claude Code
//	/workspace/AGENTS.md       — pointer (only if missing)
//
// Host secrets and ~/.cursor from the developer machine are never mounted.
// Operators pass an optional manifest or skill dirs via CLI; builtins ship embedded.
package agentconfig

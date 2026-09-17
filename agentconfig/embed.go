// SPDX-FileCopyrightText: Copyright (c) 2026 zorneth
// SPDX-License-Identifier: MIT

package agentconfig

import "embed"

//go:embed skills/policy_advisor.md skills/policy-advisor/SKILL.md
//go:embed runtime/entrypoint.sh runtime/supervisor.sh
//go:embed runtime/harnesses/cursor/exec.sh runtime/harnesses/cursor/prepare-home.sh
//go:embed runtime/harnesses/claude/exec.sh
//go:embed cursor/cli-config.json
var builtinFS embed.FS

const agentsMDName = "AGENTS.md"

// agentsMDBody is written to /AGENTS.md when missing.
const agentsMDBody = "" +
	"# osg Sandbox Guidance\n" +
	"\n" +
	"When you see a policy deny / CONNECT 403, do not treat it as final if the user\n" +
	"task still needs that request. Read `/etc/osg/skills/policy_advisor.md`\n" +
	"and use the `policy-advisor` skill, then ask the operator to apply the\n" +
	"narrowest change (`osg policy set` / provider attach) before retrying.\n" +
	"\n" +
	"Agent payload (prompt, runtime, harness): `/etc/osg/agent-payload/`.\n" +
	"Supervisor entrypoint: `/etc/osg/agent-payload/runtime/entrypoint.sh`.\n" +
	"\n" +
	"## Git / commits\n" +
	"\n" +
	"Do **not** add Cursor co-authorship or trailers (Co-authored-by: Cursor,\n" +
	"Made-with: Cursor, Made with Cursor, --trailer). Commit messages must be\n" +
	"plain subject/body only. Attribution is disabled in ~/.cursor/cli-config.json.\n"

var runtimeFiles = []string{
	"runtime/entrypoint.sh",
	"runtime/supervisor.sh",
	"runtime/harnesses/cursor/exec.sh",
	"runtime/harnesses/cursor/prepare-home.sh",
	"runtime/harnesses/claude/exec.sh",
}

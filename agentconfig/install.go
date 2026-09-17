// SPDX-FileCopyrightText: Copyright (c) 2026 zorneth
// SPDX-License-Identifier: MIT

package agentconfig

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/zorneth/osg-core/defaults"
)

// Guest is the minimal sandbox surface needed to install a staged config.
type Guest interface {
	ExecRaw(argv []string) error
	CopyTo(hostSrc, guestDest string) error
}

// Install copies a staged tree into the sandbox guest.
//
// Layout aligned with OpenShell's roles:
//   - /etc/osg/skills + /etc/osg/agent-payload  (control tree; CopyTo bypasses Landlock)
//   - /AGENTS.md                               (root pointer, like OpenShell)
//   - /sandbox/home → /osg/data/home           (harness HOME alias)
//   - $HOME/.cursor|claude/skills              (discovery links)
func Install(g Guest, st Staged) error {
	if g == nil {
		return fmt.Errorf("agentconfig: nil guest")
	}

	if st.EtcOSGHost != "" {
		if err := g.CopyTo(st.EtcOSGHost, "/etc"); err != nil {
			return fmt.Errorf("agentconfig: /etc/osg: %w", err)
		}
	}

	homePrep := fmt.Sprintf(`
set -e
mkdir -p "%s/.cursor/skills" "%s/.claude/skills" "%s/.cursor" "%s/.claude" "%s"
ln -sfn "%s" "%s"
# Harness HOME alias (OpenShell uses /sandbox/home).
grep -q OSG_AGENT_HOME "%s/.profile" 2>/dev/null || printf '%%s\n' \
  'export OSG_AGENT_HOME=%s' \
  'export HOME="${OSG_AGENT_HOME:-%s}"' >> "%s/.profile"
`,
		defaults.GuestHome, defaults.GuestHome,
		defaults.GuestHome, defaults.GuestHome,
		defaults.GuestSandboxRoot,
		defaults.GuestHome, defaults.GuestSandboxHome,
		defaults.GuestHome,
		defaults.GuestSandboxHome,
		defaults.GuestSandboxHome,
		defaults.GuestHome,
	)
	if err := g.ExecRaw([]string{"/bin/bash", "-c", homePrep}); err != nil {
		return fmt.Errorf("agentconfig: home/sandbox layout: %w", err)
	}

	linkScript := fmt.Sprintf(`
set -e
SKILLS="%s"
for d in "$SKILLS"/*; do
  [ -d "$d" ] || continue
  name="$(basename "$d")"
  ln -sfn "$d" "%s/.cursor/skills/$name"
  ln -sfn "$d" "%s/.claude/skills/$name"
done
# Make runtime scripts executable when present.
if [ -d "%s/runtime" ]; then
  chmod -R a+rX "%s/runtime" || true
  find "%s/runtime" -type f -name '*.sh' -exec chmod a+x {} \;
fi
`,
		defaults.GuestSkills,
		defaults.GuestHome, defaults.GuestHome,
		defaults.GuestAgentPayload, defaults.GuestAgentPayload, defaults.GuestAgentPayload,
	)
	if err := g.ExecRaw([]string{"/bin/bash", "-c", linkScript}); err != nil {
		return fmt.Errorf("agentconfig: link skills: %w", err)
	}

	if st.MCPCursorHost != "" {
		dest := path.Join(defaults.GuestHome, ".cursor", "mcp.json")
		if err := g.CopyTo(st.MCPCursorHost, dest); err != nil {
			return fmt.Errorf("agentconfig: cursor mcp: %w", err)
		}
	}
	if st.MCPClaudeHost != "" {
		dest := path.Join(defaults.GuestHome, ".claude", "mcp.json")
		if err := g.CopyTo(st.MCPClaudeHost, dest); err != nil {
			return fmt.Errorf("agentconfig: claude mcp: %w", err)
		}
	}

	// OpenShell path: prepare HOME from agent-payload (harness does the same each cycle).
	if st.CLIConfigHost != "" {
		prep := path.Join(defaults.GuestAgentPayload, "runtime/harnesses/cursor/prepare-home.sh")
		// If runtime wasn't included, still apply seed via inline prepare using payload path.
		script := fmt.Sprintf(`
set -e
export OSG_AGENT_HOME="%s"
export HOME="%s"
PREP="%s"
SEED="%s"
if [ -x "$PREP" ] || [ -f "$PREP" ]; then
  bash "$PREP"
elif [ -f "$SEED" ]; then
  mkdir -p "$HOME/.cursor"
  cp "$SEED" "$HOME/.cursor/cli-config.json"
fi
`, defaults.GuestHome, defaults.GuestHome, prep,
			path.Join(defaults.GuestAgentPayload, "cursor/cli-config.json"))
		if err := g.ExecRaw([]string{"/bin/bash", "-c", script}); err != nil {
			return fmt.Errorf("agentconfig: prepare cursor home: %w", err)
		}
	}

	for _, h := range st.HomeSeeds {
		rel := filepath.ToSlash(h.RelDest)
		dest := path.Join(defaults.GuestHome, rel)
		parent := path.Dir(dest)
		if err := g.ExecRaw([]string{"/bin/mkdir", "-p", parent}); err != nil {
			return fmt.Errorf("agentconfig: home mkdir %s: %w", parent, err)
		}
		copyDest := dest
		if fi, err := os.Stat(h.HostSrc); err == nil && fi.IsDir() {
			copyDest = parent
		}
		if err := g.CopyTo(h.HostSrc, copyDest); err != nil {
			return fmt.Errorf("agentconfig: home %s: %w", rel, err)
		}
	}

	if st.AgentsMDHost != "" {
		// OpenShell installs /AGENTS.md at container root when missing.
		checkRoot := `test -e /AGENTS.md || test -L /AGENTS.md`
		if err := g.ExecRaw([]string{"/bin/bash", "-c", checkRoot}); err != nil {
			if err := g.CopyTo(st.AgentsMDHost, "/"+agentsMDName); err != nil {
				return fmt.Errorf("agentconfig: /AGENTS.md: %w", err)
			}
		}
		// Also seed workspace copy when missing (agents that only scan workdir).
		checkWS := `test -e /workspace/AGENTS.md || test -L /workspace/AGENTS.md`
		if err := g.ExecRaw([]string{"/bin/bash", "-c", checkWS}); err != nil {
			if err := g.CopyTo(st.AgentsMDHost, "/workspace/"+agentsMDName); err != nil {
				return fmt.Errorf("agentconfig: /workspace/AGENTS.md: %w", err)
			}
		}
		// Readable under Landlock via /etc (always in read_only allowlist).
		_ = g.CopyTo(st.AgentsMDHost, path.Join(defaults.GuestEtcOSG, agentsMDName))
	}
	return nil
}

// DefaultOptions returns create-time defaults (builtin skills + AGENTS.md + runtime).
func DefaultOptions() Options {
	return Options{
		IncludeBuiltin: true,
		WriteAgentsMD:  true,
		IncludeRuntime: true,
		SeedCursorCLI:  true,
		Harness:        "cursor",
		RuntimeMode:    "once",
	}
}

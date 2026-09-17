// SPDX-FileCopyrightText: Copyright (c) 2026 zorneth
// SPDX-License-Identifier: MIT

package agentconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStageBuiltin(t *testing.T) {
	st, err := Stage(DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(st.Dir)

	advisor := filepath.Join(st.SkillsHostDir, "policy_advisor.md")
	b, err := os.ReadFile(advisor)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "osg Policy Advisor") {
		t.Fatalf("missing advisor content")
	}
	skill := filepath.Join(st.SkillsHostDir, "policy-advisor", "SKILL.md")
	if _, err := os.Stat(skill); err != nil {
		t.Fatal(err)
	}
	if st.AgentsMDHost == "" {
		t.Fatal("expected AGENTS.md staged")
	}
	ep := filepath.Join(st.EtcOSGHost, "agent-payload", "runtime", "entrypoint.sh")
	if _, err := os.Stat(ep); err != nil {
		t.Fatal(err)
	}
	if st.Harness != "cursor" || st.RuntimeMode != "once" {
		t.Fatalf("harness=%s mode=%s", st.Harness, st.RuntimeMode)
	}
	if st.CLIConfigHost == "" {
		t.Fatal("expected cli-config seed")
	}
	b2, err := os.ReadFile(st.CLIConfigHost)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b2), `"attributeCommitsToAgent": false`) {
		t.Fatalf("cli-config: %s", b2)
	}
	// OpenShell path: seed lives under agent-payload/cursor/
	if !strings.Contains(st.CLIConfigHost, filepath.Join("agent-payload", "cursor")) {
		t.Fatalf("expected payload bake path, got %s", st.CLIConfigHost)
	}
	prep := filepath.Join(st.EtcOSGHost, "agent-payload", "runtime", "harnesses", "cursor", "prepare-home.sh")
	if _, err := os.Stat(prep); err != nil {
		t.Fatal(err)
	}
}

func TestStageManifestSkillsAndMCP(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my\n---\n# hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mcp := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(mcp, []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	man := filepath.Join(dir, "agent-config.yaml")
	body := "version: 1\nskills:\n  - ./my-skill\nmcp:\n  cursor: ./mcp.json\n"
	if err := os.WriteFile(man, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := Stage(Options{
		ManifestPath:   man,
		IncludeBuiltin: true,
		WriteAgentsMD:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(st.Dir)

	if _, err := os.Stat(filepath.Join(st.SkillsHostDir, "my-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if st.MCPCursorHost == "" {
		t.Fatal("expected cursor mcp")
	}
	if st.AgentsMDHost != "" {
		t.Fatal("agents_md should be off")
	}
}

func TestHomeDestRejectsAbs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x")
	_ = os.WriteFile(src, []byte("x"), 0o644)
	man := filepath.Join(dir, "agent-config.yaml")
	_ = os.WriteFile(man, []byte("version: 1\nhome:\n  - src: ./x\n    dest: /etc/passwd\n"), 0o644)
	_, err := Stage(Options{ManifestPath: man, IncludeBuiltin: false, WriteAgentsMD: false})
	if err == nil {
		t.Fatal("expected reject absolute home dest")
	}
}

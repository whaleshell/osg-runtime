package mounts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkspaceOK(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveWorkspace(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("got %q want %q", got, dir)
	}
}

func TestRefuseHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	if _, err := ResolveWorkspace(home, false); err == nil {
		t.Fatal("expected refuse $HOME")
	}
	if _, err := ResolveWorkspace(home, true); err != nil {
		t.Fatalf("iKnow should allow: %v", err)
	}
}

func TestRefuseSSH(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	ssh := filepath.Join(home, ".ssh")
	if _, err := os.Stat(ssh); err != nil {
		t.Skip("no ~/.ssh")
	}
	if _, err := ResolveWorkspace(ssh, false); err == nil {
		t.Fatal("expected refuse ~/.ssh")
	}
}

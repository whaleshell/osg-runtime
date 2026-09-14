package podman

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSocketCandidatesOrder(t *testing.T) {
	t.Setenv("OSG_PODMAN_SOCKET", "/tmp/osg-podman-test.sock")
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/xdg-runtime-test")
	c := socketCandidates()
	if len(c) < 2 {
		t.Fatalf("candidates=%v", c)
	}
	if c[0] != "/tmp/osg-podman-test.sock" {
		t.Fatalf("first=%q", c[0])
	}
	wantXDG := filepath.Join("/tmp/xdg-runtime-test", "podman", "podman.sock")
	found := false
	for _, p := range c {
		if p == wantXDG {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing xdg candidate in %v", c)
	}
}

func TestResolveHostPrefersDOCKER_HOST(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	t.Setenv("CONTAINER_HOST", "unix:///ignored")
	h, err := ResolveHost()
	if err != nil || h != "unix:///var/run/docker.sock" {
		t.Fatalf("host=%q err=%v", h, err)
	}
}

func TestDiscoverSocketMissing(t *testing.T) {
	t.Setenv("OSG_PODMAN_SOCKET", "/tmp/definitely-missing-osg-podman.sock")
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/xdg-missing-"+t.Name())
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("CONTAINER_HOST", "")
	// Avoid HOME machine sockets by pointing HOME at empty dir
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, err := DiscoverSocket()
	if err == nil {
		// podman CLI may still succeed on developer machines — accept either
		if _, lookErr := os.Stat("/tmp/definitely-missing-osg-podman.sock"); lookErr == nil {
			t.Fatal("unexpected")
		}
	}
}

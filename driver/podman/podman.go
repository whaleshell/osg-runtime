// Package podman discovers a rootless/rootful Podman API socket and speaks
// the Docker-compatible Engine API (same client as driver/docker).
package podman

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/client"

	"github.com/zorneth/osg-runtime/driver/docker"
)

// New returns a compute driver pointed at Podman.
// Honors DOCKER_HOST / CONTAINER_HOST if already set; otherwise discovers a socket.
func New() (*docker.Driver, error) {
	host, err := ResolveHost()
	if err != nil {
		return nil, err
	}
	cli, err := client.NewClientWithOpts(client.WithHost(host), client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("podman driver: %w", err)
	}
	return docker.NewFromClient(cli), nil
}

// ResolveHost returns a docker client host URL (unix://… or tcp://…).
func ResolveHost() (string, error) {
	if h := strings.TrimSpace(os.Getenv("DOCKER_HOST")); h != "" {
		return h, nil
	}
	if h := strings.TrimSpace(os.Getenv("CONTAINER_HOST")); h != "" {
		return h, nil
	}
	sock, err := DiscoverSocket()
	if err != nil {
		return "", err
	}
	return "unix://" + sock, nil
}

// DiscoverSocket finds a usable podman.sock.
// Order: OSG_PODMAN_SOCKET, XDG_RUNTIME_DIR, /run/user/$UID/…,
// machine sock under HOME, then `podman info`.
func DiscoverSocket() (string, error) {
	for _, c := range socketCandidates() {
		if socketAlive(c) {
			return c, nil
		}
	}
	if s, ok := socketFromPodmanInfo(); ok {
		return s, nil
	}
	return "", fmt.Errorf("podman: no API socket found (try: systemctl --user enable --now podman.socket, or set OSG_PODMAN_SOCKET / DOCKER_HOST)")
}

func socketCandidates() []string {
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		for _, e := range out {
			if e == p {
				return
			}
		}
		out = append(out, p)
	}
	add(os.Getenv("OSG_PODMAN_SOCKET"))
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		add(filepath.Join(xdg, "podman", "podman.sock"))
	}
	if u, err := user.Current(); err == nil && u.Uid != "" {
		add(filepath.Join("/run/user", u.Uid, "podman", "podman.sock"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".local/share/containers/podman/machine/podman.sock"))
		add(filepath.Join(home, ".local/share/containers/podman/machine/machine.sock"))
	}
	return out
}

func socketAlive(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.Mode()&os.ModeSocket == 0 {
		// Some platforms report sockets differently; still try dial.
		if err != nil {
			return false
		}
	}
	c, err := net.DialTimeout("unix", path, 400*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func socketFromPodmanInfo() (string, bool) {
	cmd := exec.Command("podman", "info", "--format", "json")
	cmd.Env = os.Environ()
	b, err := cmd.Output()
	if err != nil {
		return "", false
	}
	var info struct {
		Host struct {
			RemoteSocket struct {
				Path   string `json:"path"`
				Exists bool   `json:"exists"`
			} `json:"remoteSocket"`
		} `json:"host"`
	}
	if err := json.Unmarshal(b, &info); err != nil {
		return "", false
	}
	p := strings.TrimSpace(info.Host.RemoteSocket.Path)
	if p == "" {
		return "", false
	}
	// Strip unix:// prefix if present
	p = strings.TrimPrefix(p, "unix://")
	if socketAlive(p) {
		return p, true
	}
	return "", false
}

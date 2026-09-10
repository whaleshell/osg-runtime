// Package sidecar starts the egress proxy container beside a sandbox.
package sidecar

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// EnsureLinuxCLI builds (if needed) a linux/$GOARCH osg binary for mounting into the proxy sidecar.
// Host darwin/windows binaries cannot run inside Linux containers (Docker Desktop).
func EnsureLinuxCLI(ctx context.Context, moduleDir string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	arch := runtime.GOARCH
	out := filepath.Join(cacheDir, "osg", "osg-linux-"+arch)
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/osg")
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH="+arch,
		"CGO_ENABLED=0",
	)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("sidecar: cross-compile linux osg: %w\n%s", err, strings.TrimSpace(string(b)))
	}
	return out, nil
}

// ProxyEnv returns HTTP(S)_PROXY env entries pointing at the sidecar hostname.
func ProxyEnv(proxyHost string, port int) []string {
	if proxyHost == "" {
		proxyHost = "osg-proxy"
	}
	if port <= 0 {
		port = 3128
	}
	u := fmt.Sprintf("http://%s:%d", proxyHost, port)
	return []string{
		"HTTP_PROXY=" + u,
		"HTTPS_PROXY=" + u,
		"http_proxy=" + u,
		"https_proxy=" + u,
		"ALL_PROXY=" + u,
		"NO_PROXY=localhost,127.0.0.1,::1",
		"no_proxy=localhost,127.0.0.1,::1",
	}
}

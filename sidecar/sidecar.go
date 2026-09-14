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

	"github.com/zorneth/osg-core/defaults"
)

// EnsureLinuxCLI builds (if needed) a linux/$GOARCH osg binary for mounting into the proxy sidecar.
// Host darwin/windows binaries cannot run inside Linux containers (Docker Desktop).
// Rebuild when missing, OSG_REBUILD_CLI=1, or go.mod newer than the cached binary.
func EnsureLinuxCLI(ctx context.Context, moduleDir string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	arch := runtime.GOARCH
	out := filepath.Join(cacheDir, "osg", "osg-linux-"+arch)
	if !needsLinuxRebuild(out, moduleDir) {
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

func needsLinuxRebuild(out, moduleDir string) bool {
	if os.Getenv("OSG_REBUILD_CLI") == "1" || os.Getenv("OSG_REBUILD_INIT") == "1" {
		return true
	}
	st, err := os.Stat(out)
	if err != nil || st.Size() == 0 {
		return true
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		mod := filepath.Join(moduleDir, name)
		if ms, err := os.Stat(mod); err == nil && ms.ModTime().After(st.ModTime()) {
			return true
		}
	}
	return false
}

// EnsureLinuxInit builds (if needed) a linux/$GOARCH osg-init for guest harden.
func EnsureLinuxInit(ctx context.Context, runtimeModuleDir string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	arch := runtime.GOARCH
	out := filepath.Join(cacheDir, "osg", "osg-init-linux-"+arch)
	if !needsLinuxRebuild(out, runtimeModuleDir) {
		return out, nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/osg-init")
	cmd.Dir = runtimeModuleDir
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH="+arch,
		"CGO_ENABLED=0",
	)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("sidecar: cross-compile linux osg-init: %w\n%s", err, strings.TrimSpace(string(b)))
	}
	_ = os.Chmod(out, 0o755)
	return out, nil
}

// EnsureLinuxSSHD builds linux osg-sshd for guest SSH connect.
func EnsureLinuxSSHD(ctx context.Context, runtimeModuleDir string) (string, error) {
	return ensureLinuxCmd(ctx, runtimeModuleDir, "osg-sshd", "./cmd/osg-sshd")
}

// EnsureLinuxAgent builds linux osg-agent for outbound gateway relay.
func EnsureLinuxAgent(ctx context.Context, runtimeModuleDir string) (string, error) {
	return ensureLinuxCmd(ctx, runtimeModuleDir, "osg-agent", "./cmd/osg-agent")
}

func ensureLinuxCmd(ctx context.Context, runtimeModuleDir, name, pkg string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	arch := runtime.GOARCH
	out := filepath.Join(cacheDir, "osg", name+"-linux-"+arch)
	if !needsLinuxRebuild(out, runtimeModuleDir) {
		return out, nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, pkg)
	cmd.Dir = runtimeModuleDir
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH="+arch,
		"CGO_ENABLED=0",
	)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("sidecar: cross-compile linux %s: %w\n%s", name, err, strings.TrimSpace(string(b)))
	}
	_ = os.Chmod(out, 0o755)
	return out, nil
}

// ProxyEnv returns HTTP(S)_PROXY env entries pointing at the sidecar hostname.
func ProxyEnv(proxyHost string, port int) []string {
	if proxyHost == "" {
		proxyHost = "osg-proxy"
	}
	if port <= 0 {
		port = defaults.ProxyPort
	}
	u := fmt.Sprintf("http://%s:%d", proxyHost, port)
	return []string{
		"HTTP_PROXY=" + u,
		"HTTPS_PROXY=" + u,
		"http_proxy=" + u,
		"https_proxy=" + u,
		"ALL_PROXY=" + u,
		"NO_PROXY=" + defaults.NoProxyValue,
		"no_proxy=" + defaults.NoProxyValue,
	}
}

// CABundleEnv points TLS clients at the MITM CA mounted into the sandbox.
func CABundleEnv(caFile string) []string {
	if caFile == "" {
		caFile = defaults.GuestCAFile
	}
	return []string{
		"SSL_CERT_FILE=" + caFile,
		"CURL_CA_BUNDLE=" + caFile,
		"REQUESTS_CA_BUNDLE=" + caFile,
		"NODE_EXTRA_CA_CERTS=" + caFile,
	}
}

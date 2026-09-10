// Package driver defines the compute backend interface (Docker, Podman, VM, K8s).
package driver

import (
	"context"

	"github.com/lkmavi/osg-core"
)

// Spec describes a sandbox to create.
type Spec struct {
	Name      string   // human name → container osg-<name>
	Image     string   // default ubuntu:24.04
	Workspace string   // absolute host path → /workspace
	Command   []string // default: sleep infinity
	Env       []string // KEY=VAL (already allowlisted by caller)
	IKnow     bool     // override mount deny-list (logged by caller)

	// Egress sidecar (P3). When ProxyBin is set, network is internal and
	// a dual-homed osg-proxy-<name> container is started beside the sandbox.
	ProxyBin   string // linux osg binary (host path)
	PolicyPath string // policy YAML mounted read-only into the proxy
	ProxyPort  int    // default 3128
}

// Handle is a live sandbox reference.
type Handle struct {
	ID      core.ID
	Name    string
	Network string
	Image   string
}

// Info is a list/status row.
type Info struct {
	ID      core.ID
	Name    string
	Network string
	Image   string
	Status  string
}

// ExecRequest is a one-shot or PTY-backed command.
type ExecRequest struct {
	Argv []string
	TTY  bool
	Env  []string
}

// ExecResult carries exit status.
type ExecResult struct {
	ExitCode int
}

// ComputeDriver is implemented by docker (default), podman, vm, kubernetes.
type ComputeDriver interface {
	Create(ctx context.Context, spec Spec) (Handle, error)
	Start(ctx context.Context, id core.ID) error
	Stop(ctx context.Context, id core.ID) error
	Exec(ctx context.Context, id core.ID, req ExecRequest) (ExecResult, error)
	Delete(ctx context.Context, id core.ID) error
	List(ctx context.Context) ([]Info, error)
	Inspect(ctx context.Context, nameOrID string) (Info, error)
}

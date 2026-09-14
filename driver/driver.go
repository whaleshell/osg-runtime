// Package driver defines the compute backend interface (Docker, Podman, VM, K8s).
package driver

import (
	"context"
	"io"

	"github.com/zorneth/osg-core"
)

// Spec describes a sandbox to create.
type Spec struct {
	Name      string   // human name → container osg-<name>
	Image     string   // default debian:bookworm
	Workspace string   // absolute host path → /workspace
	Command   []string // default: sleep infinity
	Env       []string // KEY=VAL (already allowlisted by caller)
	IKnow     bool     // override mount deny-list (logged by caller)

	// Egress sidecar (P3). When ProxyBin is set, network is internal and
	// a dual-homed osg-proxy-<name> container is started beside the sandbox.
	ProxyBin   string   // linux osg binary (host path)
	PolicyPath string   // policy YAML mounted read-only into the proxy (+ sandbox)
	ProxyPort  int      // default defaults.ProxyPort
	ProxyEnv   []string // real credential KEY=VAL for placeholder rewrite (proxy only)

	// Harden (P4): linux osg-init binary mounted at /osg/osg-init; execs are wrapped.
	InitBin  string
	NoHarden bool

	// Display (P6): noVNC published on host loopback only.
	DisplayMode     string // none | novnc
	DisplayPort     int    // host port; 0 uses defaults.NoVNCPort
	DisplayPassword string // VNC/noVNC password

	// Labels (P8): arbitrary osg.* / user labels on the container.
	Labels map[string]string

	// ExtraHosts entries "host:ip" (Docker ExtraHosts). host.osg.internal added by CLI.
	ExtraHosts []string

	// GatewayURL when set, create registers the sandbox with the control plane.
	GatewayURL string

	// PersistVolume mounts named volume osg-data-<name> at defaults.GuestData (retained across stop/start).
	PersistVolume bool

	// EnableSSH publishes loopback to defaults.GuestSSHPort and expects /osg/osg-sshd (linux binary).
	EnableSSH bool
	SSHBin    string // host path to linux osg-sshd

	// GPU requests NVIDIA CDI devices into the sandbox (Docker DeviceRequests).
	// Default device when CDIDevices empty: nvidia.com/gpu=all (override via OSG_GPU_CDI).
	GPU        bool
	GPUCount   int      // reserved; CDI list takes precedence in MVP
	CDIDevices []string // e.g. nvidia.com/gpu=0
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
	// Logs streams container logs to w (follow optional).
	Logs(ctx context.Context, id core.ID, follow bool, w io.Writer) error
	CopyTo(ctx context.Context, id core.ID, srcHost, destPath string) error
	CopyFrom(ctx context.Context, id core.ID, srcPath, destHost string) error
	SSHPort(ctx context.Context, id core.ID) (int, error)
	EnsureSSHDaemon(ctx context.Context, id core.ID, authorizedKey string) error
}

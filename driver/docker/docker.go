// Package docker implements driver.ComputeDriver via the Docker Engine API.
package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"golang.org/x/term"

	"github.com/lkmavi/osg-core"
	"github.com/lkmavi/osg-runtime/driver"
	"github.com/lkmavi/osg-runtime/mounts"
)

const (
	labelSandbox = "osg.sandbox"
	labelName    = "osg.name"
	labelNetwork = "osg.network"
	labelRole    = "osg.role"
	roleSandbox  = "sandbox"
	roleProxy    = "proxy"
	defaultImage = "ubuntu:24.04"
	defaultProxy = 3128
)

// Driver talks to a local Docker Engine / Desktop daemon.
type Driver struct {
	cli *client.Client
}

// New returns a Docker compute driver using DOCKER_* env (FromEnv).
func New() (*Driver, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker driver: %w", err)
	}
	return &Driver{cli: cli}, nil
}

// NewFromClient wraps an existing client (tests).
func NewFromClient(cli *client.Client) *Driver {
	return &Driver{cli: cli}
}

// Close releases the underlying HTTP client.
func (d *Driver) Close() error {
	if d == nil || d.cli == nil {
		return nil
	}
	return d.cli.Close()
}

// Create ensures network + optional proxy sidecar + sandbox container (not started).
func (d *Driver) Create(ctx context.Context, spec driver.Spec) (driver.Handle, error) {
	if d == nil || d.cli == nil {
		return driver.Handle{}, fmt.Errorf("docker driver: client not initialized")
	}
	name := sanitizeName(spec.Name)
	if name == "" {
		return driver.Handle{}, fmt.Errorf("docker driver: sandbox name required")
	}
	img := strings.TrimSpace(spec.Image)
	if img == "" {
		img = defaultImage
	}
	ws, err := mounts.ResolveWorkspace(spec.Workspace, spec.IKnow)
	if err != nil {
		return driver.Handle{}, err
	}
	netName := "osg-net-" + name
	ctrName := "osg-" + name
	withProxy := strings.TrimSpace(spec.ProxyBin) != ""

	if err := d.ensureImage(ctx, img); err != nil {
		return driver.Handle{}, err
	}
	if err := d.ensureNetwork(ctx, netName, name, withProxy); err != nil {
		return driver.Handle{}, err
	}

	env := append([]string{}, spec.Env...)
	if withProxy {
		port := spec.ProxyPort
		if port <= 0 {
			port = defaultProxy
		}
		proxyHost := "osg-proxy-" + name
		if err := d.createProxySidecar(ctx, name, netName, img, spec.ProxyBin, spec.PolicyPath, port); err != nil {
			_ = d.cli.NetworkRemove(ctx, netName)
			return driver.Handle{}, err
		}
		env = mergeEnv(env, []string{
			fmt.Sprintf("HTTP_PROXY=http://%s:%d", proxyHost, port),
			fmt.Sprintf("HTTPS_PROXY=http://%s:%d", proxyHost, port),
			fmt.Sprintf("http_proxy=http://%s:%d", proxyHost, port),
			fmt.Sprintf("https_proxy=http://%s:%d", proxyHost, port),
			fmt.Sprintf("ALL_PROXY=http://%s:%d", proxyHost, port),
			"NO_PROXY=localhost,127.0.0.1,::1",
			"no_proxy=localhost,127.0.0.1,::1",
		})
	}

	cmd := spec.Command
	if len(cmd) == 0 {
		cmd = []string{"sleep", "infinity"}
	}
	cfg := &container.Config{
		Image: img,
		Cmd:   cmd,
		Env:   env,
		Labels: map[string]string{
			labelSandbox: "1",
			labelName:    name,
			labelNetwork: netName,
			labelRole:    roleSandbox,
		},
		WorkingDir: mounts.WorkdirInContainer,
	}
	host := &container.HostConfig{
		Binds: []string{
			ws + ":" + mounts.WorkdirInContainer + ":rw",
		},
		// Never mount docker.sock into the sandbox.
	}
	networking := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			netName: {},
		},
	}
	resp, err := d.cli.ContainerCreate(ctx, cfg, host, networking, nil, ctrName)
	if err != nil {
		if withProxy {
			_ = d.removeProxySidecar(ctx, name)
		}
		_ = d.cli.NetworkRemove(ctx, netName)
		return driver.Handle{}, fmt.Errorf("docker create %s: %w", ctrName, err)
	}
	return driver.Handle{
		ID:      core.ID(resp.ID),
		Name:    name,
		Network: netName,
		Image:   img,
	}, nil
}

// Start starts a created container.
func (d *Driver) Start(ctx context.Context, id core.ID) error {
	if err := d.cli.ContainerStart(ctx, string(id), container.StartOptions{}); err != nil {
		return fmt.Errorf("docker start: %w", err)
	}
	return nil
}

// Stop stops a running container.
func (d *Driver) Stop(ctx context.Context, id core.ID) error {
	timeout := 10
	if err := d.cli.ContainerStop(ctx, string(id), container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("docker stop: %w", err)
	}
	return nil
}

// Exec runs a command in the container. With TTY, attaches stdin/stdout in raw mode.
func (d *Driver) Exec(ctx context.Context, id core.ID, req driver.ExecRequest) (driver.ExecResult, error) {
	if len(req.Argv) == 0 {
		return driver.ExecResult{}, fmt.Errorf("docker exec: empty argv")
	}
	tty := req.TTY
	execID, err := d.cli.ContainerExecCreate(ctx, string(id), container.ExecOptions{
		Cmd:          req.Argv,
		Env:          req.Env,
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  tty,
		Tty:          tty,
		WorkingDir:   mounts.WorkdirInContainer,
	})
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("docker exec create: %w", err)
	}
	attach, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{Tty: tty})
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("docker exec attach: %w", err)
	}
	defer attach.Close()

	if tty {
		inFd := int(os.Stdin.Fd())
		if term.IsTerminal(inFd) {
			old, err := term.MakeRaw(inFd)
			if err != nil {
				return driver.ExecResult{}, fmt.Errorf("docker exec raw terminal: %w", err)
			}
			defer func() { _ = term.Restore(inFd, old) }()
		}
		errCh := make(chan error, 1)
		go func() {
			_, copyErr := io.Copy(attach.Conn, os.Stdin)
			_ = attach.CloseWrite()
			errCh <- copyErr
		}()
		_, _ = io.Copy(os.Stdout, attach.Reader)
		<-errCh
	} else {
		_, _ = stdcopy.StdCopy(os.Stdout, os.Stderr, attach.Reader)
	}

	inspect, err := d.cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("docker exec inspect: %w", err)
	}
	return driver.ExecResult{ExitCode: inspect.ExitCode}, nil
}

// Delete removes sandbox container, proxy sidecar, and osg network.
func (d *Driver) Delete(ctx context.Context, id core.ID) error {
	info, err := d.cli.ContainerInspect(ctx, string(id))
	if err != nil {
		return fmt.Errorf("docker delete inspect: %w", err)
	}
	netName := info.Config.Labels[labelNetwork]
	name := info.Config.Labels[labelName]
	_ = d.cli.ContainerRemove(ctx, string(id), container.RemoveOptions{Force: true})
	if name != "" {
		_ = d.removeProxySidecar(ctx, name)
	}
	if netName != "" {
		_ = d.cli.NetworkRemove(ctx, netName)
	}
	return nil
}

// List returns osg sandbox containers (excludes proxy sidecars).
func (d *Driver) List(ctx context.Context) ([]driver.Info, error) {
	f := filters.NewArgs()
	f.Add("label", labelSandbox+"=1")
	list, err := d.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, err
	}
	out := make([]driver.Info, 0, len(list))
	for _, c := range list {
		if c.Labels[labelRole] == roleProxy {
			continue
		}
		name := c.Labels[labelName]
		if name == "" && len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		out = append(out, driver.Info{
			ID:      core.ID(c.ID),
			Name:    name,
			Network: c.Labels[labelNetwork],
			Image:   c.Image,
			Status:  c.Status,
		})
	}
	return out, nil
}

// Inspect resolves by sandbox name or container id/prefix.
func (d *Driver) Inspect(ctx context.Context, nameOrID string) (driver.Info, error) {
	nameOrID = strings.TrimSpace(nameOrID)
	if nameOrID == "" {
		return driver.Info{}, fmt.Errorf("docker inspect: empty name")
	}
	// Try container name osg-<name>
	candidates := []string{nameOrID, "osg-" + nameOrID}
	for _, id := range candidates {
		c, err := d.cli.ContainerInspect(ctx, id)
		if err != nil {
			continue
		}
		if c.Config.Labels[labelSandbox] != "1" && !strings.HasPrefix(strings.TrimPrefix(c.Name, "/"), "osg-") {
			continue
		}
		name := c.Config.Labels[labelName]
		if name == "" {
			name = strings.TrimPrefix(c.Name, "/")
			name = strings.TrimPrefix(name, "osg-")
		}
		return driver.Info{
			ID:      core.ID(c.ID),
			Name:    name,
			Network: c.Config.Labels[labelNetwork],
			Image:   c.Config.Image,
			Status:  c.State.Status,
		}, nil
	}
	return driver.Info{}, fmt.Errorf("docker inspect: sandbox %q not found", nameOrID)
}

func (d *Driver) ensureNetwork(ctx context.Context, netName, sandboxName string, internal bool) error {
	_, err := d.cli.NetworkInspect(ctx, netName, network.InspectOptions{})
	if err == nil {
		return nil
	}
	_, err = d.cli.NetworkCreate(ctx, netName, network.CreateOptions{
		Driver:   "bridge",
		Internal: internal, // fail-closed when proxy sidecar is enabled
		Labels: map[string]string{
			labelSandbox: "1",
			labelName:    sandboxName,
		},
	})
	if err != nil {
		return fmt.Errorf("docker network create %s: %w", netName, err)
	}
	return nil
}

func (d *Driver) createProxySidecar(ctx context.Context, name, netName, img, binPath, policyPath string, port int) error {
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("docker proxy bin: %w", err)
	}
	if policyPath == "" {
		return fmt.Errorf("docker proxy: policy path required")
	}
	if _, err := os.Stat(policyPath); err != nil {
		return fmt.Errorf("docker proxy policy: %w", err)
	}
	ctrName := "osg-proxy-" + name
	_ = d.cli.ContainerRemove(ctx, ctrName, container.RemoveOptions{Force: true})

	cfg := &container.Config{
		Image: img,
		Cmd: []string{
			"/osg/osg", "proxy",
			"--listen", fmt.Sprintf("0.0.0.0:%d", port),
			"--policy", "/osg/policy.yaml",
		},
		Labels: map[string]string{
			labelSandbox: "1",
			labelName:    name,
			labelNetwork: netName,
			labelRole:    roleProxy,
		},
		ExposedPorts: nat.PortSet{
			nat.Port(fmt.Sprintf("%d/tcp", port)): {},
		},
	}
	host := &container.HostConfig{
		Binds: []string{
			binPath + ":/osg/osg:ro",
			policyPath + ":/osg/policy.yaml:ro",
		},
	}
	networking := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			netName: {Aliases: []string{"osg-proxy", ctrName}},
		},
	}
	resp, err := d.cli.ContainerCreate(ctx, cfg, host, networking, nil, ctrName)
	if err != nil {
		return fmt.Errorf("docker create proxy %s: %w", ctrName, err)
	}
	// Dual-home onto the default bridge so the proxy can reach the internet
	// while the sandbox stays on an internal network only.
	if err := d.cli.NetworkConnect(ctx, "bridge", resp.ID, nil); err != nil {
		_ = d.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return fmt.Errorf("docker proxy bridge connect: %w", err)
	}
	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = d.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return fmt.Errorf("docker start proxy: %w", err)
	}
	return nil
}

func (d *Driver) removeProxySidecar(ctx context.Context, name string) error {
	ctrName := "osg-proxy-" + name
	_ = d.cli.ContainerRemove(ctx, ctrName, container.RemoveOptions{Force: true})
	return nil
}

func mergeEnv(base, extra []string) []string {
	keys := map[string]int{}
	out := make([]string, 0, len(base)+len(extra))
	add := func(entry string) {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			return
		}
		if i, exists := keys[key]; exists {
			out[i] = entry
			return
		}
		keys[key] = len(out)
		out = append(out, entry)
	}
	for _, e := range base {
		add(e)
	}
	for _, e := range extra {
		add(e)
	}
	return out
}

func (d *Driver) ensureImage(ctx context.Context, ref string) error {
	_, _, err := d.cli.ImageInspectWithRaw(ctx, ref)
	if err == nil {
		return nil
	}
	rc, err := d.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("docker pull %s: %w", ref, err)
	}
	defer rc.Close()
	_, _ = io.Copy(io.Discard, rc)
	return nil
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// Probe is a host-side Docker readiness report for `osg health`.
type Probe struct {
	OK              bool
	ServerVersion   string
	APIVersion      string
	OperatingSystem string
	Architecture    string
	Context         string
	Isolation       string
	HostGOOS        string
	Error           string
}

// Health probes the daemon (Ping + ServerVersion + Info).
func (d *Driver) Health(ctx context.Context) Probe {
	p := Probe{
		Context:  dockerContextName(),
		HostGOOS: runtime.GOOS,
	}
	if d == nil || d.cli == nil {
		p.Error = "docker client not initialized"
		return p
	}
	if _, err := d.cli.Ping(ctx); err != nil {
		p.Error = err.Error()
		return p
	}
	ver, err := d.cli.ServerVersion(ctx)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	p.ServerVersion = ver.Version
	p.APIVersion = ver.APIVersion
	p.OperatingSystem = ver.Os
	p.Architecture = ver.Arch

	info, err := d.cli.Info(ctx)
	if err == nil {
		if info.OperatingSystem != "" {
			p.OperatingSystem = info.OperatingSystem
		}
		if info.Architecture != "" {
			p.Architecture = info.Architecture
		}
		p.Isolation = classifyIsolation(runtime.GOOS, info.OperatingSystem, info.OSType)
	} else {
		p.Isolation = classifyIsolation(runtime.GOOS, p.OperatingSystem, ver.Os)
	}
	p.OK = true
	return p
}

func dockerContextName() string {
	if v := strings.TrimSpace(os.Getenv("DOCKER_CONTEXT")); v != "" {
		return v
	}
	return "default"
}

func classifyIsolation(hostGOOS, operatingSystem, osType string) string {
	blob := strings.ToLower(operatingSystem + " " + osType)
	if strings.Contains(blob, "docker desktop") ||
		strings.Contains(blob, "dockerdesktop") ||
		hostGOOS == "darwin" || hostGOOS == "windows" {
		if hostGOOS == "darwin" || hostGOOS == "windows" {
			return "docker-desktop-vm"
		}
	}
	if hostGOOS == "linux" {
		return "native-linux"
	}
	return "unknown"
}

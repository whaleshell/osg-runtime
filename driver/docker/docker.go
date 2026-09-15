// Package docker implements driver.ComputeDriver via the Docker Engine API.
package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"golang.org/x/term"

	"archive/tar"
	"path/filepath"

	"github.com/zorneth/osg-core"
	"github.com/zorneth/osg-core/defaults"
	"github.com/zorneth/osg-runtime/driver"
	"github.com/zorneth/osg-runtime/mounts"
	"github.com/zorneth/osg-runtime/sidecar"
)

const (
	labelSandbox = "osg.sandbox"
	labelName    = "osg.name"
	labelNetwork = "osg.network"
	labelRole    = "osg.role"
	labelInit    = "osg.init"
	labelVolume  = "osg.volume"
	labelSSH     = "osg.ssh"
	labelPolicy  = "osg.policy_path"
	roleSandbox  = "sandbox"
	roleProxy    = "proxy"
)

// Image and guest layout defaults (aliased from osg-core/defaults).
const (
	defaultImage      = defaults.ImageDebian
	localSandboxImage = defaults.ImageLocal
	guiSandboxImage   = defaults.ImageGUI
	defaultProxy      = defaults.ProxyPort
	defaultNoVNCPort  = defaults.NoVNCPort
	guestSSHPort      = defaults.GuestSSHPort
	guestDataPath     = defaults.GuestData
	guestHomePath     = defaults.GuestHome
	guestBinPath      = defaults.GuestBin
	guestPath         = defaults.GuestPath
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
		img = d.defaultSandboxImage(ctx, spec.DisplayMode, spec.GPU)
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
	var caVol string
	if withProxy {
		port := spec.ProxyPort
		if port <= 0 {
			port = defaultProxy
		}
		proxyHost := "osg-proxy-" + name
		caVol = "osg-ca-" + name
		if err := d.ensureVolume(ctx, caVol); err != nil {
			_ = d.cli.NetworkRemove(ctx, netName)
			return driver.Handle{}, err
		}
		if err := d.createProxySidecar(ctx, name, netName, img, spec.ProxyBin, spec.PolicyPath, port, caVol, spec.ProxyEnv); err != nil {
			_ = d.cli.VolumeRemove(ctx, caVol, true)
			_ = d.cli.NetworkRemove(ctx, netName)
			return driver.Handle{}, err
		}
		env = mergeEnv(env, sidecar.ProxyEnv(proxyHost, port))
		env = mergeEnv(env, sidecar.CABundleEnv(defaults.GuestCAFile))
	}

	binds := []string{
		ws + ":" + mounts.WorkdirInContainer + ":rw",
	}
	labels := map[string]string{
		labelSandbox: "1",
		labelName:    name,
		labelNetwork: netName,
		labelRole:    roleSandbox,
	}
	if caVol != "" {
		binds = append(binds, caVol+":"+defaults.GuestCADir+":ro")
		labels["osg.ca_volume"] = caVol
	}
	for k, v := range spec.Labels {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		labels[k] = v
	}
	if spec.PersistVolume {
		volName := "osg-data-" + name
		if err := d.ensureVolume(ctx, volName); err != nil {
			if withProxy {
				_ = d.removeProxySidecar(ctx, name)
			}
			_ = d.cli.NetworkRemove(ctx, netName)
			return driver.Handle{}, err
		}
		binds = append(binds, volName+":"+guestDataPath+":rw")
		labels[labelVolume] = volName
		env = mergeEnv(env, []string{
			"OSG_DATA=" + guestDataPath,
			"HOME=" + guestHomePath,
			"PATH=" + guestPath,
		})
	}
	if !spec.NoHarden && strings.TrimSpace(spec.InitBin) != "" {
		if _, err := os.Stat(spec.InitBin); err != nil {
			return driver.Handle{}, fmt.Errorf("docker init bin: %w", err)
		}
		binds = append(binds, spec.InitBin+":/osg/osg-init:ro")
		if spec.PolicyPath != "" {
			binds = append(binds, spec.PolicyPath+":/osg/policy.yaml:ro")
			env = mergeEnv(env, []string{"OSG_POLICY=/osg/policy.yaml"})
			if abs, err := filepath.Abs(spec.PolicyPath); err == nil {
				labels[labelPolicy] = abs
			} else {
				labels[labelPolicy] = spec.PolicyPath
			}
		}
		labels[labelInit] = "1"
	}
	if spec.EnableSSH {
		if strings.TrimSpace(spec.SSHBin) == "" {
			return driver.Handle{}, fmt.Errorf("docker ssh: SSHBin required when EnableSSH")
		}
		if _, err := os.Stat(spec.SSHBin); err != nil {
			return driver.Handle{}, fmt.Errorf("docker ssh bin: %w", err)
		}
		binds = append(binds, spec.SSHBin+":/osg/osg-sshd:ro")
		labels[labelSSH] = "1"
		env = mergeEnv(env, []string{"OSG_SSH=1"})
	}

	displayMode := strings.ToLower(strings.TrimSpace(spec.DisplayMode))
	withDisplay := displayMode == "novnc"
	if withDisplay {
		if !imageHasGUI(ctx, d, img) {
			return driver.Handle{}, fmt.Errorf("docker display: image %q missing GUI stack; build with: task runtime:image:gui", img)
		}
		pass := strings.TrimSpace(spec.DisplayPassword)
		if pass == "" {
			pass = "osg"
		}
		hostPort := spec.DisplayPort
		if hostPort <= 0 {
			hostPort = defaultNoVNCPort
		}
		env = mergeEnv(env, []string{
			"DISPLAY_MODE=novnc",
			"DISPLAY=:99",
			"OSG_VNC_PASSWORD=" + pass,
			fmt.Sprintf("OSG_NOVNC_PORT=%d", defaultNoVNCPort),
		})
		labels["osg.display"] = "novnc"
		labels["osg.display.port"] = strconv.Itoa(hostPort)
	}

	cmd := spec.Command
	if len(cmd) == 0 {
		switch {
		case withDisplay && usesEmbeddedInit(img):
			cmd = []string{"--", "/usr/local/bin/osg-gui-boot"}
		case usesEmbeddedInit(img):
			cmd = []string{"--", "sleep", "infinity"}
		case withDisplay:
			cmd = []string{"/usr/local/bin/osg-gui-boot"}
		default:
			cmd = []string{"sleep", "infinity"}
		}
	}
	cfg := &container.Config{
		Image:      img,
		Cmd:        cmd,
		Env:        env,
		Labels:     labels,
		WorkingDir: mounts.WorkdirInContainer,
	}
	host := &container.HostConfig{
		Binds: binds,
		// Never mount docker.sock into the sandbox.
		// Docker default seccomp stays on (do not set seccomp=unconfined).
		SecurityOpt: []string{"no-new-privileges:true"},
		CapDrop:     []string{"NET_RAW"},
		ExtraHosts:  append([]string{}, spec.ExtraHosts...),
	}
	if reqs := DeviceRequestsForGPU(spec); len(reqs) > 0 {
		host.DeviceRequests = reqs
		labels[labelGPU] = "1"
	}
	if cfg.ExposedPorts == nil {
		cfg.ExposedPorts = nat.PortSet{}
	}
	if host.PortBindings == nil {
		host.PortBindings = nat.PortMap{}
	}
	if withDisplay {
		hostPort := spec.DisplayPort
		if hostPort <= 0 {
			hostPort = defaultNoVNCPort
		}
		p := nat.Port(fmt.Sprintf("%d/tcp", defaultNoVNCPort))
		cfg.ExposedPorts[p] = struct{}{}
		host.PortBindings[p] = []nat.PortBinding{{
			HostIP:   "127.0.0.1",
			HostPort: strconv.Itoa(hostPort),
		}}
		// Chromium needs shared memory; default 64MiB is too small in Docker.
		host.ShmSize = 1 << 30 // 1 GiB
	}
	if spec.EnableSSH {
		p := nat.Port(fmt.Sprintf("%d/tcp", guestSSHPort))
		cfg.ExposedPorts[p] = struct{}{}
		host.PortBindings[p] = []nat.PortBinding{{
			HostIP:   "127.0.0.1",
			HostPort: "0", // docker allocates
		}}
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
		if len(host.DeviceRequests) > 0 {
			return driver.Handle{}, fmt.Errorf("docker create %s: %w\nhint: enable NVIDIA CDI / Container Toolkit, or unset --gpu (see docs/GPU.md)", ctrName, err)
		}
		return driver.Handle{}, fmt.Errorf("docker create %s: %w", ctrName, err)
	}
	return driver.Handle{
		ID:      core.ID(resp.ID),
		Name:    name,
		Network: netName,
		Image:   img,
	}, nil
}

// Start starts a created container and seeds persist dirs when applicable.
func (d *Driver) Start(ctx context.Context, id core.ID) error {
	if err := d.cli.ContainerStart(ctx, string(id), container.StartOptions{}); err != nil {
		return fmt.Errorf("docker start: %w", err)
	}
	if err := d.ensureGuestLayout(ctx, string(id)); err != nil {
		// Non-fatal: PATH is also injected via container/exec env.
		fmt.Fprintf(os.Stderr, "docker: guest layout: %v\n", err)
	}
	return nil
}

// ensureGuestLayout creates durable home/bin under GuestData for agent installs.
// Uses a raw docker exec (no osg-init wrap) so layout works before harden paths matter.
func (d *Driver) ensureGuestLayout(ctx context.Context, id string) error {
	info, err := d.cli.ContainerInspect(ctx, id)
	if err != nil || info.Config == nil {
		return err
	}
	if info.Config.Labels[labelRole] == roleProxy {
		return nil
	}
	if info.Config.Labels[labelVolume] == "" {
		return nil
	}
	script := "mkdir -p " + guestHomePath + "/.local/bin " + guestBinPath + " /etc/profile.d && " +
		"printf '%s\\n' 'export PATH=\"" + guestHomePath + "/.local/bin:" + guestBinPath + ":$PATH\"' > " + guestHomePath + "/.profile && " +
		"printf '%s\\n' 'export PATH=\"" + guestHomePath + "/.local/bin:" + guestBinPath + ":$PATH\"' > " + guestHomePath + "/.bashrc && " +
		"printf '%s\\n' 'export PATH=\"" + guestHomePath + "/.local/bin:" + guestBinPath + ":$PATH\"' > /etc/profile.d/osg-path.sh"
	execID, err := d.cli.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd:          []string{"/bin/bash", "-c", script},
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   "/",
		Env:          []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
	})
	if err != nil {
		return fmt.Errorf("guest layout exec create: %w", err)
	}
	attach, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("guest layout exec attach: %w", err)
	}
	defer attach.Close()
	_, _ = io.Copy(io.Discard, attach.Reader)
	insp, err := d.cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return err
	}
	if insp.ExitCode != 0 {
		return fmt.Errorf("guest layout exit %d", insp.ExitCode)
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
// When the sandbox was created with osg-init, argv is wrapped: osg-init -- <cmd>.
func (d *Driver) Exec(ctx context.Context, id core.ID, req driver.ExecRequest) (driver.ExecResult, error) {
	if len(req.Argv) == 0 {
		return driver.ExecResult{}, fmt.Errorf("docker exec: empty argv")
	}
	argv := req.Argv
	workdir := mounts.WorkdirInContainer
	if info, err := d.cli.ContainerInspect(ctx, string(id)); err == nil && info.Config != nil {
		if info.Config.Labels[labelInit] == "1" {
			argv = append([]string{"/osg/osg-init", "--"}, req.Argv...)
		}
		// Proxy sidecar has no /workspace mount; exec must not chdir there.
		if info.Config.Labels[labelRole] == roleProxy {
			workdir = "/"
		}
	}
	tty := req.TTY
	inFd := int(os.Stdin.Fd())
	outFd := int(os.Stdout.Fd())
	hostTTY := tty && term.IsTerminal(inFd)

	// Prefer stdout for size (same as docker CLI Out().GetTtySize).
	sizeFd := outFd
	if !term.IsTerminal(sizeFd) {
		sizeFd = inFd
	}

	var consoleSize *[2]uint
	env := ensureTTYEnv(req.Env)
	// Do not inject LINES/COLUMNS: Node TUIs prefer env over ioctl, and a wrong
	// pair permanently squashes the UI. Size comes from ConsoleSize + resize.
	if hostTTY || term.IsTerminal(sizeFd) {
		if width, height, err := term.GetSize(sizeFd); err == nil && width > 0 && height > 0 {
			// Docker ConsoleSize is [height, width] (docker CLI fillConsoleSize).
			consoleSize = &[2]uint{uint(height), uint(width)}
		}
	}

	execID, err := d.cli.ContainerExecCreate(ctx, string(id), container.ExecOptions{
		Cmd:          argv,
		Env:          env,
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  tty,
		Tty:          tty,
		WorkingDir:   workdir,
		ConsoleSize:  consoleSize,
	})
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("docker exec create: %w", err)
	}
	attach, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{
		Tty:         tty,
		ConsoleSize: consoleSize,
	})
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("docker exec attach: %w", err)
	}
	defer attach.Close()
	setConnNoDelay(attach.Conn)

	if tty {
		if hostTTY {
			old, err := term.MakeRaw(inFd)
			if err != nil {
				return driver.ExecResult{}, fmt.Errorf("docker exec raw terminal: %w", err)
			}
			defer func() { _ = term.Restore(inFd, old) }()
			// Resize before streaming so TUI mouse/scroll sees a real size on first paint.
			resizeExecOnce(ctx, d.cli, execID.ID, sizeFd)
			go resizeExecTTY(ctx, d.cli, execID.ID, sizeFd)
		}
		// When the remote PTY closes (e.g. shell `exit`), stop waiting on stdin —
		// otherwise stdin copy blocks until another keystroke.
		outDone := make(chan struct{})
		go func() {
			defer close(outDone)
			_, _ = io.Copy(os.Stdout, attach.Reader)
		}()
		go func() {
			// Small reads so mouse-wheel CSI sequences are forwarded promptly.
			_, _ = copyStdinTTY(attach.Conn, os.Stdin)
			_ = attach.CloseWrite()
		}()
		<-outDone
		_ = attach.CloseWrite()
	} else {
		_, _ = stdcopy.StdCopy(os.Stdout, os.Stderr, attach.Reader)
	}

	inspect, err := d.cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("docker exec inspect: %w", err)
	}
	return driver.ExecResult{ExitCode: inspect.ExitCode}, nil
}

// ensureTTYEnv forces a guest TERM that Debian ships terminfo for.
// Host values like xterm-kitty / xterm-ghostty break mouse scroll inside the container.
func ensureTTYEnv(base []string) []string {
	env := append([]string{}, base...)
	termName := strings.TrimSpace(os.Getenv("TERM"))
	switch {
	case termName == "" || termName == "dumb" || termName == "unknown":
		termName = "xterm-256color"
	case strings.Contains(termName, "kitty"),
		strings.Contains(termName, "ghostty"),
		strings.Contains(termName, "alacritty"),
		strings.HasPrefix(termName, "tmux"),
		strings.HasPrefix(termName, "screen"):
		// Keep 256color semantics; container rarely has fancy terminfo entries.
		termName = "xterm-256color"
	}
	env = mergeEnv(env, []string{"TERM=" + termName})
	if os.Getenv("COLORTERM") == "" {
		env = mergeEnv(env, []string{"COLORTERM=truecolor"})
	}
	return env
}

func setConnNoDelay(c net.Conn) {
	type noDelayer interface{ SetNoDelay(bool) error }
	if nd, ok := c.(noDelayer); ok {
		_ = nd.SetNoDelay(true)
	}
	if tcp, ok := c.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
}

// copyStdinTTY forwards host key/mouse bytes with a small buffer (not io.Copy's 32KiB).
func copyStdinTTY(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 256)
	var n int64
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			n += int64(nw)
			if ew != nil {
				return n, ew
			}
			if nr != nw {
				return n, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return n, nil
			}
			return n, er
		}
	}
}

func resizeExecOnce(ctx context.Context, cli client.ContainerAPIClient, execID string, inFd int) {
	width, height, err := term.GetSize(inFd)
	if err != nil || width <= 0 || height <= 0 {
		return
	}
	_ = cli.ContainerExecResize(ctx, execID, container.ResizeOptions{
		Height: uint(height),
		Width:  uint(width),
	})
}

// resizeExecTTY sets the guest PTY size to the host terminal and watches SIGWINCH.
// Without this, interactive TUIs often render but ignore keyboard input (size 0×0),
// and mouse-wheel scroll never attaches to the app.
func resizeExecTTY(ctx context.Context, cli client.ContainerAPIClient, execID string, inFd int) {
	doResize := func() {
		resizeExecOnce(ctx, cli, execID, inFd)
	}
	// Retry like docker CLI — resize can race with exec start.
	for i := 0; i < 10; i++ {
		doResize()
		time.Sleep(20 * time.Millisecond)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			doResize()
		}
	}
}

// Delete removes sandbox container, proxy sidecar, osg network, and data volume.
func (d *Driver) Delete(ctx context.Context, id core.ID) error {
	info, err := d.cli.ContainerInspect(ctx, string(id))
	if err != nil {
		return fmt.Errorf("docker delete inspect: %w", err)
	}
	netName := info.Config.Labels[labelNetwork]
	name := info.Config.Labels[labelName]
	volName := info.Config.Labels[labelVolume]
	caVol := info.Config.Labels["osg.ca_volume"]
	_ = d.cli.ContainerRemove(ctx, string(id), container.RemoveOptions{Force: true})
	if name != "" {
		_ = d.removeProxySidecar(ctx, name)
	}
	if netName != "" {
		_ = d.cli.NetworkRemove(ctx, netName)
	}
	if volName != "" {
		_ = d.cli.VolumeRemove(ctx, volName, true)
	}
	if caVol != "" {
		_ = d.cli.VolumeRemove(ctx, caVol, true)
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

// Logs streams stdout/stderr from the sandbox container and, when present, the
// egress proxy sidecar (where OCSF agent-observation events are emitted).
// When follow is true, starts from the last 500 lines per container.
func (d *Driver) Logs(ctx context.Context, id core.ID, follow bool, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: false,
	}
	if follow {
		opts.Tail = "500"
	}

	type src struct {
		id     string
		prefix string
	}
	sources := []src{{id: string(id), prefix: "sandbox"}}

	if info, err := d.cli.ContainerInspect(ctx, string(id)); err == nil && info.Config != nil {
		name := info.Config.Labels[labelName]
		if name == "" {
			name = strings.TrimPrefix(strings.TrimPrefix(info.Name, "/"), "osg-")
		}
		if name != "" {
			proxyName := "osg-proxy-" + name
			if _, err := d.cli.ContainerInspect(ctx, proxyName); err == nil {
				sources = append(sources, src{id: proxyName, prefix: "proxy"})
			}
		}
	}

	if len(sources) == 1 {
		rc, err := d.cli.ContainerLogs(ctx, sources[0].id, opts)
		if err != nil {
			return fmt.Errorf("docker logs: %w", err)
		}
		defer rc.Close()
		_, err = stdcopy.StdCopy(w, w, rc)
		return err
	}

	var mu sync.Mutex
	writeLine := func(prefix, text string) error {
		mu.Lock()
		defer mu.Unlock()
		_, err := fmt.Fprintf(w, "[%s] %s\n", prefix, text)
		return err
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(sources))
	for _, s := range sources {
		rc, err := d.cli.ContainerLogs(ctx, s.id, opts)
		if err != nil {
			if s.prefix == "proxy" {
				continue
			}
			return fmt.Errorf("docker logs %s: %w", s.id, err)
		}
		wg.Add(1)
		go func(prefix string, rc io.ReadCloser) {
			defer wg.Done()
			defer rc.Close()
			pr, pw := io.Pipe()
			go func() {
				_, copyErr := stdcopy.StdCopy(pw, pw, rc)
				_ = pw.CloseWithError(copyErr)
			}()
			sc := bufio.NewScanner(pr)
			sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for sc.Scan() {
				if ctx.Err() != nil {
					errCh <- ctx.Err()
					return
				}
				if err := writeLine(prefix, sc.Text()); err != nil {
					errCh <- err
					return
				}
			}
			if err := sc.Err(); err != nil && ctx.Err() == nil {
				errCh <- err
			}
		}(s.prefix, rc)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		select {
		case err := <-errCh:
			return err
		default:
			return nil
		}
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Driver) ensureVolume(ctx context.Context, name string) error {
	_, err := d.cli.VolumeInspect(ctx, name)
	if err == nil {
		return nil
	}
	_, err = d.cli.VolumeCreate(ctx, volume.CreateOptions{Name: name, Labels: map[string]string{"osg.volume": "1"}})
	if err != nil {
		return fmt.Errorf("docker volume create %s: %w", name, err)
	}
	return nil
}

// CopyTo tars srcHost and extracts at destPath inside the container.
// For a single file, destPath is the full guest path (basename used in the tar).
// For a directory, destPath is the guest parent directory under which src's basename appears.
func (d *Driver) CopyTo(ctx context.Context, id core.ID, srcHost, destPath string) error {
	srcHost = filepath.Clean(srcHost)
	st, err := os.Stat(srcHost)
	if err != nil {
		return err
	}
	destPath = filepath.ToSlash(filepath.Clean(destPath))
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		tw := tar.NewWriter(pw)
		var err error
		if st.IsDir() {
			err = writeTarDir(tw, srcHost, filepath.Base(srcHost))
		} else {
			err = writeTarFile(tw, srcHost, st, filepath.Base(destPath))
		}
		_ = tw.Close()
		_ = pw.CloseWithError(err)
		errCh <- err
	}()
	destDir := destPath
	if !st.IsDir() {
		destDir = filepath.ToSlash(filepath.Dir(destPath))
		if destDir == "." || destDir == "" {
			destDir = "/"
		}
	}
	err = d.cli.CopyToContainer(ctx, string(id), destDir, pr, container.CopyToContainerOptions{AllowOverwriteDirWithFile: true})
	_ = pr.Close()
	if werr := <-errCh; werr != nil && err == nil {
		err = werr
	}
	if err != nil {
		return fmt.Errorf("docker copy to: %w", err)
	}
	return nil
}

func writeTarDir(tw *tar.Writer, src, base string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(base, rel))
		if rel == "." {
			name = base + "/"
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = name
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		_ = f.Close()
		return err
	})
}

func writeTarFile(tw *tar.Writer, src string, st os.FileInfo, archiveName string) error {
	hdr, err := tar.FileInfoHeader(st, "")
	if err != nil {
		return err
	}
	hdr.Name = archiveName
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

// CopyFrom reads a container path as tar and writes into destHost.
// If destHost exists as a directory (or ends with a path separator), entries are
// extracted under it. Otherwise a single regular file is written to destHost.
func (d *Driver) CopyFrom(ctx context.Context, id core.ID, srcPath, destHost string) error {
	rc, _, err := d.cli.CopyFromContainer(ctx, string(id), srcPath)
	if err != nil {
		return fmt.Errorf("docker copy from: %w", err)
	}
	defer rc.Close()

	asDir := strings.HasSuffix(destHost, string(os.PathSeparator)) || strings.HasSuffix(destHost, "/")
	if st, err := os.Stat(destHost); err == nil && st.IsDir() {
		asDir = true
	}

	tr := tar.NewReader(rc)
	wroteFile := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			if !asDir && !wroteFile {
				return fmt.Errorf("docker copy from: no regular file at %s", srcPath)
			}
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if !asDir {
				continue
			}
			if err := os.MkdirAll(filepath.Join(destHost, name), 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			var target string
			if asDir {
				target = filepath.Join(destHost, name)
			} else {
				if wroteFile {
					return fmt.Errorf("docker copy from: dest %s is a file but archive has multiple entries", destHost)
				}
				target = destHost
				wroteFile = true
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			_ = f.Close()
			if err != nil {
				return err
			}
		}
	}
}

// SSHPort returns the host port bound to guest GuestSSHPort.
func (d *Driver) SSHPort(ctx context.Context, id core.ID) (int, error) {
	info, err := d.cli.ContainerInspect(ctx, string(id))
	if err != nil {
		return 0, err
	}
	if info.Config != nil && info.Config.Labels[labelSSH] != "1" {
		return 0, fmt.Errorf("sandbox was not created with --ssh")
	}
	p := nat.Port(fmt.Sprintf("%d/tcp", guestSSHPort))
	if info.NetworkSettings == nil {
		return 0, fmt.Errorf("no network settings")
	}
	bindings := info.NetworkSettings.Ports[p]
	if len(bindings) == 0 || bindings[0].HostPort == "" {
		return 0, fmt.Errorf("ssh port not published")
	}
	port, err := strconv.Atoi(bindings[0].HostPort)
	if err != nil {
		return 0, err
	}
	return port, nil
}

// EnsureSSHDaemon starts /osg/osg-sshd inside the guest if labeled for SSH.
func (d *Driver) EnsureSSHDaemon(ctx context.Context, id core.ID, authorizedKey string) error {
	info, err := d.cli.ContainerInspect(ctx, string(id))
	if err != nil {
		return err
	}
	if info.Config == nil || info.Config.Labels[labelSSH] != "1" {
		return fmt.Errorf("sandbox was not created with --ssh")
	}
	tmp, err := os.MkdirTemp("", "osg-ssh-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	keyFile := filepath.Join(tmp, "authorized_keys")
	if err := os.WriteFile(keyFile, []byte(authorizedKey+"\n"), 0o600); err != nil {
		return err
	}
	if err := d.CopyTo(ctx, id, keyFile, "/osg/ssh/authorized_keys"); err != nil {
		// ensure dir then retry via shell mkdir
		_, _ = d.Exec(ctx, id, driver.ExecRequest{Argv: []string{"mkdir", "-p", "/osg/ssh"}})
		if err := d.CopyTo(ctx, id, keyFile, "/osg/ssh/authorized_keys"); err != nil {
			return err
		}
	}
	_, err = d.Exec(ctx, id, driver.ExecRequest{
		Argv: []string{"sh", "-c", fmt.Sprintf(
			`chmod 600 /osg/ssh/authorized_keys 2>/dev/null; if ! pgrep -f /osg/osg-sshd >/dev/null 2>&1; then /osg/osg-sshd --listen 0.0.0.0:%d --authorized-keys /osg/ssh/authorized_keys --host-key /osg/ssh/host_ed25519 >/osg/ssh/sshd.log 2>&1 & fi; sleep 0.3; pgrep -f /osg/osg-sshd >/dev/null`,
			guestSSHPort)},
	})
	return err
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

func (d *Driver) createProxySidecar(ctx context.Context, name, netName, img, binPath, policyPath string, port int, caVol string, proxyEnv []string) error {
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

	binds := []string{
		binPath + ":/osg/osg:ro",
		policyPath + ":/osg/policy.yaml:ro",
	}
	cmd := []string{
		"/osg/osg", "proxy",
		"--listen", fmt.Sprintf("0.0.0.0:%d", port),
		"--policy", "/osg/policy.yaml",
	}
	if caVol != "" {
		binds = append(binds, caVol+":/osg/ca:rw")
		cmd = append(cmd, "--ca-out", "/osg/ca/ca.pem")
	}

	absPolicy := policyPath
	if a, err := filepath.Abs(policyPath); err == nil {
		absPolicy = a
	}
	cfg := &container.Config{
		Image: img,
		Cmd:   cmd,
		Env:   append([]string{}, proxyEnv...),
		Labels: map[string]string{
			labelSandbox: "1",
			labelName:    name,
			labelNetwork: netName,
			labelRole:    roleProxy,
			labelPolicy:  absPolicy,
		},
		ExposedPorts: nat.PortSet{
			nat.Port(fmt.Sprintf("%d/tcp", port)): {},
		},
	}
	host := &container.HostConfig{Binds: binds}
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
	if caVol != "" {
		if err := d.waitProxyCA(ctx, resp.ID, 5*time.Second); err != nil {
			_ = d.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
			return err
		}
	}
	return nil
}

func (d *Driver) waitProxyCA(ctx context.Context, proxyID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		res, err := d.Exec(ctx, core.ID(proxyID), driver.ExecRequest{
			Argv: []string{"test", "-s", "/osg/ca/ca.pem"},
		})
		if err == nil && res.ExitCode == 0 {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	logs, _ := d.cli.ContainerLogs(ctx, proxyID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: "40"})
	var buf strings.Builder
	if logs != nil {
		_, _ = io.Copy(&buf, logs)
		_ = logs.Close()
	}
	detail := strings.TrimSpace(buf.String())
	if detail != "" {
		return fmt.Errorf("docker proxy: timed out waiting for /osg/ca/ca.pem (last exec: %v)\nproxy logs:\n%s", lastErr, detail)
	}
	return fmt.Errorf("docker proxy: timed out waiting for /osg/ca/ca.pem (last exec: %v)", lastErr)
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
	low := strings.ToLower(ref)
	if low == localSandboxImage || low == guiSandboxImage || low == gpuSandboxImage || strings.HasPrefix(low, "osg-sandbox:") {
		hint := "cli"
		switch {
		case low == guiSandboxImage || strings.Contains(low, "gui"):
			hint = "gui"
		case low == gpuSandboxImage || strings.Contains(low, "gpu"):
			hint = "gpu"
		}
		return fmt.Errorf("docker image %s not found locally; build with: task runtime:image %s", ref, hint)
	}
	rc, err := d.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("docker pull %s: %w", ref, err)
	}
	defer rc.Close()
	_, _ = io.Copy(io.Discard, rc)
	return nil
}

func (d *Driver) defaultSandboxImage(ctx context.Context, displayMode string, gpu bool) string {
	if d == nil || d.cli == nil {
		return defaultImage
	}
	if strings.EqualFold(strings.TrimSpace(displayMode), "novnc") {
		if _, _, err := d.cli.ImageInspectWithRaw(ctx, guiSandboxImage); err == nil {
			return guiSandboxImage
		}
		return guiSandboxImage // ensureImage will fail with a clear pull error; task runtime:image:gui preferred
	}
	if gpu {
		if _, _, err := d.cli.ImageInspectWithRaw(ctx, gpuSandboxImage); err == nil {
			return gpuSandboxImage
		}
		return gpuSandboxImage
	}
	_, _, err := d.cli.ImageInspectWithRaw(ctx, localSandboxImage)
	if err == nil {
		return localSandboxImage
	}
	return defaultImage
}

func usesEmbeddedInit(img string) bool {
	img = strings.TrimSpace(strings.ToLower(img))
	return img == localSandboxImage || img == guiSandboxImage || strings.HasPrefix(img, "osg-sandbox:")
}

func imageHasGUI(ctx context.Context, d *Driver, img string) bool {
	img = strings.TrimSpace(strings.ToLower(img))
	if img == guiSandboxImage || strings.Contains(img, ":gui") {
		return true
	}
	// Heuristic: inspect for osg-gui-boot via missing path is hard; require known tags.
	_ = ctx
	_ = d
	return false
}

// SeccompNote documents the MVP harden posture for health / spike notes.
func SeccompNote() string {
	return "docker-default + no-new-privileges + CapDrop=NET_RAW (in-process filter later)"
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

// RunProbe runs osg-init --probe in a one-shot helper container (Landlock ABI).
func (d *Driver) RunProbe(ctx context.Context, initBin string) (string, error) {
	if d == nil || d.cli == nil {
		return "", fmt.Errorf("docker client not initialized")
	}
	if err := d.ensureImage(ctx, defaultImage); err != nil {
		return "", err
	}
	cfg := &container.Config{
		Image:      defaultImage,
		Cmd:        []string{"/osg/osg-init", "--probe"},
		WorkingDir: "/",
	}
	host := &container.HostConfig{
		Binds: []string{initBin + ":/osg/osg-init:ro"},
		// AutoRemove handled after wait
	}
	resp, err := d.cli.ContainerCreate(ctx, cfg, host, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("probe create: %w", err)
	}
	id := resp.ID
	defer func() { _ = d.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}) }()
	if err := d.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("probe start: %w", err)
	}
	statusCh, errCh := d.cli.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return "", err
		}
	case <-statusCh:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	logs, err := d.cli.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", err
	}
	defer logs.Close()
	var buf strings.Builder
	_, _ = stdcopy.StdCopy(&buf, &buf, logs)
	out := strings.TrimSpace(buf.String())
	if out == "" {
		return "empty probe output", nil
	}
	// Compact one-line summary if JSON
	if strings.Contains(out, "LandlockABI") {
		return summarizeProbeJSON(out), nil
	}
	return out, nil
}

func summarizeProbeJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	// Prefer a one-line human summary when JSON decode works.
	var m struct {
		LandlockABI   int    `json:"LandlockABI"`
		LandlockError string `json:"LandlockError"`
	}
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		if m.LandlockABI > 0 {
			return fmt.Sprintf("abi=%d (probe ok)", m.LandlockABI)
		}
		if m.LandlockError != "" {
			return fmt.Sprintf("abi=0 (%s)", m.LandlockError)
		}
		return "abi=0"
	}
	return strings.ReplaceAll(strings.ReplaceAll(raw, "\n", " "), "  ", " ")
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

<h1 align="center">whaleshell-runtime</h1>

<p align="center">
  <strong>Sandbox glue & images</strong><br>
  Lifecycle helpers, harden, secrets store, whaleshell-init, and GHCR sandbox images.
</p>
<p align="center">
  <a href="https://github.com/whaleshell/whaleshell-runtime/actions/workflows/ci.yml"><img src="https://github.com/whaleshell/whaleshell-runtime/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://pkg.go.dev/github.com/whaleshell/whaleshell-runtime"><img src="https://pkg.go.dev/badge/github.com/whaleshell/whaleshell-runtime.svg" alt="Go Reference"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License"></a>
  <a href="https://github.com/whaleshell/whaleshell-runtime"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go Version"></a>

  <a href="https://github.com/whaleshell/whaleshell-runtime/actions/workflows/images-sandbox.yml"><img src="https://github.com/whaleshell/whaleshell-runtime/actions/workflows/images-sandbox.yml/badge.svg" alt="images-sandbox"></a>
</p>
<p align="center">
  <sub>Part of the <a href="https://github.com/whaleshell">whaleshell / whaleshell</a> ecosystem</sub>
</p>

---

## Overview

**whaleshell-runtime** ties sandbox creation together: guest init (`whaleshell-init`), Landlock/seccomp harden, secrets store, inference snippets, and the Debian-based sandbox image flavors published to GHCR.

### Key Features

| Category | Capabilities |
|----------|--------------|
| **Images** | `sandboxes/{base,gui,gpu}` on `ghcr.io/whaleshell/whaleshell` |
| **Init** | `whaleshell-init` — Landlock, seccomp, workspace mount |
| **Secrets** | Host-side store resolved into guest placeholders |
| **Harden** | Linux sandbox hardening helpers |
| **Inference** | Local host-gateway model URL helpers |

---

## Installation

```bash
go get github.com/whaleshell/whaleshell-runtime@latest
```

**Images (after CI publish):**

```text
ghcr.io/whaleshell/whaleshell/sandboxes/base:latest
ghcr.io/whaleshell/whaleshell/sandboxes/gui:latest
ghcr.io/whaleshell/whaleshell/sandboxes/gpu:latest
```

**Requirements:** Go 1.27+

---

## Quick Start

```bash
# build guest init into the image context (as CI does)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -o images/sandbox/whaleshell-init ./cmd/whaleshell-init

docker build -t whaleshell-sandbox:local --target cli -f images/sandbox/Dockerfile images/sandbox
```

---

## Package Structure

| Path | Purpose |
|------|---------|
| `cmd/whaleshell-init` | Guest entrypoint |
| `sandbox/` | Lifecycle helpers |
| `harden/` | Landlock / seccomp |
| `secrets/` | Secret store |
| `images/sandbox/` | Multi-target Dockerfile |
| `inference/` | Local model policy snippets |
| `agentconfig/` | Agent payload helpers |


---

## Related

| Resource | Link |
|----------|------|
| Roadmap | [ROADMAP.md](./ROADMAP.md) |
| Organization | [https://github.com/whaleshell](https://github.com/whaleshell) |
| Organization overview | [github.com/whaleshell](https://github.com/whaleshell) |
| pkg.go.dev | [`github.com/whaleshell/whaleshell-runtime`](https://pkg.go.dev/github.com/whaleshell/whaleshell-runtime) |

## License

[MIT](./LICENSE) © whaleshell

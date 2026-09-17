<h1 align="center">osg-runtime</h1>

<p align="center">
  <strong>Sandbox glue & images</strong><br>
  Lifecycle helpers, harden, secrets store, osg-init, and GHCR sandbox images.
</p>
<p align="center">
  <a href="https://github.com/zorneth/osg-runtime/actions/workflows/ci.yml"><img src="https://github.com/zorneth/osg-runtime/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://pkg.go.dev/github.com/zorneth/osg-runtime"><img src="https://pkg.go.dev/badge/github.com/zorneth/osg-runtime.svg" alt="Go Reference"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License"></a>
  <a href="https://github.com/zorneth/osg-runtime"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go Version"></a>

  <a href="https://github.com/zorneth/osg-runtime/actions/workflows/images-sandbox.yml"><img src="https://github.com/zorneth/osg-runtime/actions/workflows/images-sandbox.yml/badge.svg" alt="images-sandbox"></a>
</p>
<p align="center">
  <sub>Part of the <a href="https://github.com/zorneth">zorneth / osg</a> ecosystem</sub>
</p>

---

## Overview

**osg-runtime** ties sandbox creation together: guest init (`osg-init`), Landlock/seccomp harden, secrets store, inference snippets, and the Debian-based sandbox image flavors published to GHCR.

### Key Features

| Category | Capabilities |
|----------|--------------|
| **Images** | `sandboxes/{base,gui,gpu}` on `ghcr.io/zorneth/osg` |
| **Init** | `osg-init` — Landlock, seccomp, workspace mount |
| **Secrets** | Host-side store resolved into guest placeholders |
| **Harden** | Linux sandbox hardening helpers |
| **Inference** | Local host-gateway model URL helpers |

---

## Installation

```bash
go get github.com/zorneth/osg-runtime@latest
```

**Images (after CI publish):**

```text
ghcr.io/zorneth/osg/sandboxes/base:latest
ghcr.io/zorneth/osg/sandboxes/gui:latest
ghcr.io/zorneth/osg/sandboxes/gpu:latest
```

**Requirements:** Go 1.27+

---

## Quick Start

```bash
# build guest init into the image context (as CI does)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -o images/sandbox/osg-init ./cmd/osg-init

docker build -t osg-sandbox:local --target cli -f images/sandbox/Dockerfile images/sandbox
```

---

## Package Structure

| Path | Purpose |
|------|---------|
| `cmd/osg-init` | Guest entrypoint |
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
| Organization | [https://github.com/zorneth](https://github.com/zorneth) |
| Organization overview | [github.com/zorneth](https://github.com/zorneth) |
| pkg.go.dev | [`github.com/zorneth/osg-runtime`](https://pkg.go.dev/github.com/zorneth/osg-runtime) |

## License

[MIT](./LICENSE) © zorneth

# osg-runtime

MIT — sandbox lifecycle, Docker driver, CONNECT proxy, env allowlist, `osg-init`.

**Depends on:** `osg-core`  
**Must not depend on:** `osg-cli`, gateway UI

```bash
export GOWORK=/path/to/agent-blocker/go.work
go test -C osg-runtime ./...
go build -C osg-cli -o osg ./cmd/osg && ./osg health
```

See [LICENSING.md](../docs/LICENSING.md).

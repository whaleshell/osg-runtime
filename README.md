# osg-runtime

Apache-2.0 · transitional runtime (sandbox, harden, proxy, drivers, `osg-init`).

**Depends on:** MIT `osg-core` (product policy), `github.com/docker/docker` (client)  
**Must not depend on:** `osg-cli`, gateway UI

Policy YAML is the **osg** schema (`osg-core`); OpenShell-shaped files are imported at parse time.

```bash
task --dir osg-runtime check:quick
go build -C osg-cli -o osg ./cmd/osg && ./osg health
```

See [LICENSING.md](../docs/LICENSING.md).

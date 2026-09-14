package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// PolicyHostPath returns the host path of the sandbox policy bind (label osg.policy_path).
func (d *Driver) PolicyHostPath(ctx context.Context, nameOrID string) (string, error) {
	if d == nil || d.cli == nil {
		return "", fmt.Errorf("docker driver: client not initialized")
	}
	name := sanitizeName(nameOrID)
	for _, ctr := range []string{"osg-proxy-" + name, "osg-" + name} {
		ins, err := d.cli.ContainerInspect(ctx, ctr)
		if err != nil {
			continue
		}
		if p := strings.TrimSpace(ins.Config.Labels[labelPolicy]); p != "" {
			return p, nil
		}
		for _, m := range ins.Mounts {
			if m.Destination == "/osg/policy.yaml" && m.Source != "" {
				return m.Source, nil
			}
		}
	}
	f := filters.NewArgs()
	f.Add("label", labelSandbox+"=1")
	f.Add("label", labelName+"="+name)
	list, err := d.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return "", fmt.Errorf("docker policy path: %w", err)
	}
	for _, c := range list {
		if p := strings.TrimSpace(c.Labels[labelPolicy]); p != "" {
			return p, nil
		}
	}
	return "", fmt.Errorf("docker policy path: no policy bind for sandbox %q (create with --policy)", nameOrID)
}

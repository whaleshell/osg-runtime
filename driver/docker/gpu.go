package docker

import (
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"

	"github.com/zorneth/osg-core/defaults"
	"github.com/zorneth/osg-runtime/driver"
)

const (
	defaultCDIDevice = "nvidia.com/gpu=all"
	labelGPU         = "osg.gpu"
	gpuSandboxImage  = defaults.ImageGPU
)

// DeviceRequestsForGPU builds Docker CDI DeviceRequests for a Spec (nil if GPU off).
func DeviceRequestsForGPU(spec driver.Spec) []container.DeviceRequest {
	if !spec.GPU && len(spec.CDIDevices) == 0 {
		return nil
	}
	ids := make([]string, 0, len(spec.CDIDevices))
	for _, id := range spec.CDIDevices {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		if env := strings.TrimSpace(os.Getenv("OSG_GPU_CDI")); env != "" {
			for _, part := range strings.Split(env, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					ids = append(ids, part)
				}
			}
		}
	}
	if len(ids) == 0 {
		ids = []string{defaultCDIDevice}
	}
	return []container.DeviceRequest{{
		Driver:    "cdi",
		DeviceIDs: ids,
	}}
}

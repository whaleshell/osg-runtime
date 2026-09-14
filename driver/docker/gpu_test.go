package docker

import (
	"testing"

	"github.com/zorneth/osg-runtime/driver"
)

func TestDeviceRequestsForGPU(t *testing.T) {
	if DeviceRequestsForGPU(driver.Spec{}) != nil {
		t.Fatal("expected nil when GPU off")
	}
	reqs := DeviceRequestsForGPU(driver.Spec{GPU: true})
	if len(reqs) != 1 || reqs[0].Driver != "cdi" || len(reqs[0].DeviceIDs) != 1 || reqs[0].DeviceIDs[0] != defaultCDIDevice {
		t.Fatalf("reqs=%+v", reqs)
	}
	reqs = DeviceRequestsForGPU(driver.Spec{CDIDevices: []string{"nvidia.com/gpu=0", "  "}})
	if len(reqs) != 1 || len(reqs[0].DeviceIDs) != 1 || reqs[0].DeviceIDs[0] != "nvidia.com/gpu=0" {
		t.Fatalf("reqs=%+v", reqs)
	}
	t.Setenv("OSG_GPU_CDI", "nvidia.com/gpu=1,nvidia.com/gpu=2")
	reqs = DeviceRequestsForGPU(driver.Spec{GPU: true})
	if len(reqs[0].DeviceIDs) != 2 {
		t.Fatalf("env ids=%v", reqs[0].DeviceIDs)
	}
}

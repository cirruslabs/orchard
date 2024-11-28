package scheduler_test

import (
	"github.com/cirruslabs/orchard/internal/controller/scheduler"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWorkerInfos(t *testing.T) {
	workerInfos := make(scheduler.WorkerInfos)
	require.Len(t, workerInfos, 0)

	workerInfos.AddVM("worker-name", v1.Resources{
		"tart-vms": 1,
	})
	require.Len(t, workerInfos, 1)

	workerInfos.AddVM("worker-name", v1.Resources{
		"tart-vms": 1,
	})
	require.Len(t, workerInfos, 1)
	require.Equal(t, scheduler.WorkerInfo{
		ResourcesUsed: map[string]uint64{
			"tart-vms": 2,
		},
		NumRunningVMs: 2,
	}, workerInfos.Get("worker-name"))
}

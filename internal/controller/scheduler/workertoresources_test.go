package scheduler_test

import (
	"github.com/cirruslabs/orchard/internal/controller/scheduler"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWorkerToResources(t *testing.T) {
	workerToResources := make(scheduler.WorkerToResources)
	require.Len(t, workerToResources, 0)

	workerToResources.Add("worker-name", v1.Resources{
		"tart-vms": 1,
	})
	require.Len(t, workerToResources, 1)

	workerToResources.Add("worker-name", v1.Resources{
		"tart-vms": 1,
	})
	require.Len(t, workerToResources, 1)
	require.Equal(t, v1.Resources{"tart-vms": 2}, workerToResources.Get("worker-name"))
}

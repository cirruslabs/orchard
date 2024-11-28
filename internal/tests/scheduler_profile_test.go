package tests_test

import (
	"context"
	"github.com/cirruslabs/orchard/internal/tests/devcontroller"
	"github.com/cirruslabs/orchard/internal/tests/wait"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSchedulerProfileOptimizeUtilization(t *testing.T) {
	ctx := context.Background()

	// Create a development environment
	devClient, _, _ := devcontroller.StartIntegrationTestEnvironmentWithAdditionalOpts(t,
		false, nil,
		true, nil,
	)

	// Change the scheduler to pack as many VMs as possible on each worker
	clusterSettings, err := devClient.ClusterSettings().Get(ctx)
	require.NoError(t, err)
	clusterSettings.SchedulerProfile = v1.SchedulerProfileOptimizeUtilization
	require.NoError(t, devClient.ClusterSettings().Set(ctx, clusterSettings))

	// Create three workers and three VMs
	threeWorkersThreeVMsScenario(t, devClient)

	ensureAssignment(t, devClient, "test-vm-1", "worker-a")
	ensureAssignment(t, devClient, "test-vm-2", "worker-a")
	ensureAssignment(t, devClient, "test-vm-3", "worker-a")
}

func TestSchedulerProfileDistributeLoad(t *testing.T) {
	ctx := context.Background()

	// Create a development environment
	devClient, _, _ := devcontroller.StartIntegrationTestEnvironmentWithAdditionalOpts(t,
		false, nil,
		true, nil,
	)

	// Change the scheduler to spread VMs as much as possible between workers
	clusterSettings, err := devClient.ClusterSettings().Get(ctx)
	require.NoError(t, err)
	clusterSettings.SchedulerProfile = v1.SchedulerProfileDistributeLoad
	require.NoError(t, devClient.ClusterSettings().Set(ctx, clusterSettings))

	// Create three workers and three VMs
	threeWorkersThreeVMsScenario(t, devClient)

	ensureAssignment(t, devClient, "test-vm-1", "worker-a")
	ensureAssignment(t, devClient, "test-vm-2", "worker-b")
	ensureAssignment(t, devClient, "test-vm-3", "worker-c")
}

func threeWorkersThreeVMsScenario(t *testing.T, devClient *client.Client) {
	ctx := context.Background()

	_, err := devClient.Workers().Create(ctx, v1.Worker{
		Meta: v1.Meta{
			Name: "worker-a",
		},
		Resources: map[string]uint64{
			v1.ResourceTartVMs: 3,
		},
	})
	require.NoError(t, err)
	_, err = devClient.Workers().Create(ctx, v1.Worker{
		Meta: v1.Meta{
			Name: "worker-b",
		},
		Resources: map[string]uint64{
			v1.ResourceTartVMs: 3,
		},
	})
	require.NoError(t, err)
	_, err = devClient.Workers().Create(ctx, v1.Worker{
		Meta: v1.Meta{
			Name: "worker-c",
		},
		Resources: map[string]uint64{
			v1.ResourceTartVMs: 3,
		},
	})
	require.NoError(t, err)

	require.NoError(t, devClient.VMs().Create(ctx, &v1.VM{
		Meta: v1.Meta{
			Name: "test-vm-1",
		},
		Image:  "example.com/doesnt/matter:latest",
		CPU:    4,
		Memory: 8 * 1024,
		Status: v1.VMStatusPending,
	}))
	require.NoError(t, devClient.VMs().Create(ctx, &v1.VM{
		Meta: v1.Meta{
			Name: "test-vm-2",
		},
		Image:  "example.com/doesnt/matter:latest",
		CPU:    4,
		Memory: 8 * 1024,
		Status: v1.VMStatusPending,
	}))
	require.NoError(t, devClient.VMs().Create(ctx, &v1.VM{
		Meta: v1.Meta{
			Name: "test-vm-3",
		},
		Image:  "example.com/doesnt/matter:latest",
		CPU:    4,
		Memory: 8 * 1024,
		Status: v1.VMStatusPending,
	}))
}

func ensureAssignment(t *testing.T, devClient *client.Client, vmName string, workerName string) {
	require.True(t, wait.Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), vmName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM %s to be assigned to a worker", vmName)

		return vm.Worker == workerName
	}), "VM was %s expected to be assigned to the worker %q, but was assigned to the worker %q",
		vmName, workerName)
}

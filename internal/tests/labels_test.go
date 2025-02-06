package tests_test

import (
	"context"
	"github.com/cirruslabs/orchard/internal/tests/devcontroller"
	"github.com/cirruslabs/orchard/internal/tests/wait"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestLabels(t *testing.T) {
	ctx := context.Background()

	// Create a development environment
	devClient, _, _ := devcontroller.StartIntegrationTestEnvironmentWithAdditionalOpts(t,
		false, nil,
		true, nil,
	)

	// Create a worker that doesn't have any labels
	_, err := devClient.Workers().Create(ctx, v1.Worker{
		Meta: v1.Meta{
			Name: "worker-without-labels",
		},
		Resources: map[string]uint64{
			v1.ResourceTartVMs: 2,
		},
	})
	require.NoError(t, err)

	// Create a VM that requests a "role=test" label
	vmName := "test-vm"

	require.NoError(t, devClient.VMs().Create(ctx, &v1.VM{
		Meta: v1.Meta{
			Name: vmName,
		},
		Image:  "example.com/doesnt/matter:latest",
		Labels: map[string]string{"role": "test"},
		Status: v1.VMStatusPending,
	}))

	// Ensure that this VM doesn't get assigned in 30 seconds
	require.False(t, wait.Wait(30*time.Second, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), vmName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM %s to be assigned", vmName)

		return vm.Worker != ""
	}), "VM %s was not expected to be assigned to any worker, but was assigned to some worker", vmName)

	// Now create one more worker that has the required labels
	const workerWithLabelsName = "worker-with-labels"

	_, err = devClient.Workers().Create(ctx, v1.Worker{
		Meta: v1.Meta{
			Name: workerWithLabelsName,
		},
		Resources: map[string]uint64{
			v1.ResourceTartVMs: 2,
		},
		Labels: map[string]string{"role": "test"},
	})
	require.NoError(t, err)

	// Wait for the VM to be assigned to the new worker
	require.True(t, wait.Wait(30*time.Second, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), vmName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM %s to be assigned to a worker", vmName)

		return vm.Worker == workerWithLabelsName
	}), "VM was %s expected to be assigned to the worker %q, but was assigned "+
		"to another worker (or no worker)", vmName, workerWithLabelsName)
}

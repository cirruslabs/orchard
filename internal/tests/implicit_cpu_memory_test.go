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

func TestImplicitCPUMemory(t *testing.T) {
	ctx := context.Background()

	// Create a development environment
	devClient, _, _ := devcontroller.StartIntegrationTestEnvironmentWithAdditionalOpts(t,
		false, nil,
		true, nil,
	)

	// Create a worker with default CPU and memory values
	const workerName = "worker"

	_, err := devClient.Workers().Create(ctx, v1.Worker{
		Meta: v1.Meta{
			Name: workerName,
		},
		Resources: map[string]uint64{
			v1.ResourceTartVMs: 2,
		},
		DefaultCPU:    12,
		DefaultMemory: 3456,
	})
	require.NoError(t, err)

	// Create a VM with implicit CPU and memory
	vmName := "test-vm"

	require.NoError(t, devClient.VMs().Create(ctx, &v1.VM{
		Meta: v1.Meta{
			Name: vmName,
		},
		Image:  "example.com/doesnt/matter:latest",
		Status: v1.VMStatusPending,
	}))

	// Wait for the VM to be assigned
	require.True(t, wait.Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), vmName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM %s to be assigned to a worker", vmName)

		return vm.Worker == workerName
	}), "VM was %s expected to be assigned to the worker %q, but was assigned to the worker %q",
		vmName, workerName)

	// Ensure that the VM is using default CPU and memory values from the worker
	vm, err := devClient.VMs().Get(context.Background(), vmName)
	require.NoError(t, err)
	require.EqualValues(t, 12, vm.AssignedCPU)
	require.EqualValues(t, 3456, vm.AssignedMemory)
}

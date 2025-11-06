package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/imageconstant"
	"github.com/cirruslabs/orchard/internal/tests/devcontroller"
	"github.com/cirruslabs/orchard/internal/tests/wait"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/samber/lo"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

func TestSpecUpdateSoftnet(t *testing.T) {
	devClient, _, _ := devcontroller.StartIntegrationTestEnvironment(t)

	// Create a VM
	vmName := "test"

	err := devClient.VMs().Create(t.Context(), &v1.VM{
		Meta: v1.Meta{
			Name: vmName,
		},
		Image:    imageconstant.DefaultMacosImage,
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
		Status:   v1.VMStatusPending,
	})
	require.NoError(t, err)

	// Wait for the VM to start
	var vm *v1.VM

	require.True(t, wait.Wait(2*time.Minute, func() bool {
		vm, err = devClient.VMs().Get(context.Background(), vmName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM to start. Current status: %s", vm.Status)

		return vm.Status == v1.VMStatusRunning
	}), "failed to start a VM")

	// Ensure that Softnet is not enabled for a VM
	tartVMName := ondiskname.New(vmName, vm.UID, vm.RestartCount).String()

	tartRunCmdline, err := tartRunProcessCmdline(tartVMName)
	require.NoError(t, err)
	require.NotContains(t, tartRunCmdline, "--net-softnet")
	require.NotContains(t, tartRunCmdline, "--net-softnet-allow")
	require.NotContains(t, tartRunCmdline, "--net-softnet-block")

	// Update the VM's specification and enable Softnet
	vm.NetSoftnetAllow = []string{"10.0.0.0/16"}
	vm.NetSoftnetBlock = []string{"0.0.0.0/0"}

	vm, err = devClient.VMs().Update(t.Context(), *vm)
	require.NoError(t, err)
	require.EqualValues(t, 1, vm.Generation)
	require.EqualValues(t, 0, vm.ObservedGeneration)

	require.True(t, wait.Wait(30*time.Second, func() bool {
		vm, err = devClient.VMs().Get(context.Background(), vmName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM's observed generation to be updated...")

		return vm.ObservedGeneration == 1
	}), "failed to update a VM")

	tartRunCmdline, err = tartRunProcessCmdline(tartVMName)
	require.NoError(t, err)
	require.Contains(t, tartRunCmdline, "--net-softnet")
	require.True(t, sliceContainsAnotherSlice(tartRunCmdline, []string{"--net-softnet-allow", "10.0.0.0/16"}))
	require.True(t, sliceContainsAnotherSlice(tartRunCmdline, []string{"--net-softnet-block", "0.0.0.0/0"}))
}

func tartRunProcessCmdline(vmName string) ([]string, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}

	for _, process := range processes {
		name, err := process.Name()
		if err != nil {
			// On macOS, process.Name() returns "invalid argument" for most
			// of the processes likely due to permissions, so just ignore it
			continue
		}

		if name != "tart" {
			continue
		}

		cmdline, err := process.CmdlineSlice()
		if err != nil {
			return nil, err
		}

		if len(cmdline) < 3 {
			continue
		}

		if cmdline[1] != "run" {
			continue
		}

		if lo.Contains(cmdline[2:], vmName) {
			return cmdline, nil
		}
	}

	return nil, fmt.Errorf("failed to find a \"tart run\" process for VM %q", vmName)
}

func sliceContainsAnotherSlice(haystack []string, needle []string) bool {
	if len(needle) == 0 {
		return true
	}

	var needleIdx int

	for _, haystackItem := range haystack {
		if haystackItem == needle[needleIdx] {
			needleIdx++

			if needleIdx == len(needle) {
				return true
			}
		}
	}

	return false
}

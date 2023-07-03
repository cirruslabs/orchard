package tests_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/command/dev"
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/cirruslabs/orchard/internal/worker"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/internal/worker/tart"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/exp/slices"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSingleVM(t *testing.T) {
	devClient := StartIntegrationTestEnvironment(t)

	workers, err := devClient.Workers().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(workers))
	err = devClient.VMs().Create(context.Background(), &v1.VM{
		Meta: v1.Meta{
			Name: "test-vm",
		},
		Image:    "ghcr.io/cirruslabs/macos-ventura-base:latest",
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
		Status:   v1.VMStatusPending,
		StartupScript: &v1.VMScript{
			ScriptContent: "echo \"Hello, $FOO!\"\nfor i in $(seq 1 1000); do echo \"$i\"; done",
			Env:           map[string]string{"FOO": "Bar"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), "test-vm")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Waiting for the VM to start. Current status: %s", vm.Status)
		return vm.Status == v1.VMStatusRunning || vm.Status == v1.VMStatusFailed
	}), "failed to start a VM")
	runningVM, err := devClient.VMs().Get(context.Background(), "test-vm")
	if err != nil {
		t.Fatal(err)
	}
	assert.Empty(t, runningVM.StatusMessage)
	assert.Equal(t, v1.VMStatusRunning, runningVM.Status)
	assert.True(t, Wait(2*time.Minute, func() bool {
		logLines, err := devClient.VMs().Logs(context.Background(), "test-vm")
		if err != nil {
			t.Fatal(err)
		}
		return len(logLines) > 0
	}), "failed to wait for logs to become available")
	logLines, err := devClient.VMs().Logs(context.Background(), "test-vm")
	if err != nil {
		t.Fatal(err)
	}
	expectedLogs := []string{"Hello, Bar!"}
	for i := 1; i <= 1000; i++ {
		expectedLogs = append(expectedLogs, strconv.Itoa(i))
	}
	assert.Equal(t, expectedLogs, logLines)

	// Ensure that the VM exists on disk before deleting it
	require.True(t, hasVMByPredicate(t, func(info tart.VMInfo) bool {
		return strings.Contains(info.Name, runningVM.UID)
	}, nil))

	// Delete the VM from the controller
	require.NoError(t, devClient.VMs().Delete(context.Background(), "test-vm"))

	// Ensure that the worker has deleted this VM from disk
	assert.True(t, Wait(2*time.Minute, func() bool {
		t.Logf("Waiting for the VM to be garbage collected...")

		return !hasVMByPredicate(t, func(info tart.VMInfo) bool {
			return strings.Contains(info.Name, runningVM.UID)
		}, nil)
	}), "VM was not garbage collected in a timely manner")
}

func TestFailedStartupScript(t *testing.T) {
	devClient := StartIntegrationTestEnvironment(t)

	workers, err := devClient.Workers().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(workers))
	err = devClient.VMs().Create(context.Background(), &v1.VM{
		Meta: v1.Meta{
			Name: "test-vm",
		},
		Image:    "ghcr.io/cirruslabs/macos-ventura-base:latest",
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
		Status:   v1.VMStatusPending,
		StartupScript: &v1.VMScript{
			ScriptContent: "exit 123",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), "test-vm")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Waiting for the VM to start. Current status: %s", vm.Status)
		return vm.Status == v1.VMStatusFailed
	}), "failed to start a VM")
	runningVM, err := devClient.VMs().Get(context.Background(), "test-vm")
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, runningVM.StatusMessage,
		"failed to run startup script: Process exited with status 123")
}

func Wait(duration time.Duration, condition func() bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	for {
		if condition() {
			// all good
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(5 * time.Second):
			// try again
			continue
		}
	}
}

func StartIntegrationTestEnvironment(
	t *testing.T,
) *client.Client {
	return StartIntegrationTestEnvironmentWithAdditionalOpts(t, nil, nil)
}

func StartIntegrationTestEnvironmentWithAdditionalOpts(
	t *testing.T,
	additionalControllerOpts []controller.Option,
	additionalWorkerOpts []worker.Option,
) *client.Client {
	t.Setenv("ORCHARD_HOME", t.TempDir())
	devController, devWorker, err := dev.CreateDevControllerAndWorker(t.TempDir(),
		":0", nil, additionalControllerOpts, additionalWorkerOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = devWorker.Close()
	})
	devContext, cancelDevFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelDevFunc)
	go func() {
		err := devController.Run(devContext)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("dev controller failed: %v", err)
		}
	}()
	go func() {
		err := devWorker.Run(devContext)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("dev worker failed: %v", err)
		}
	}()

	time.Sleep(5 * time.Second)

	devClient, err := client.New(client.WithAddress(devController.Address()))
	if err != nil {
		t.Fatal(err)
	}
	return devClient
}

func TestPortForwarding(t *testing.T) {
	ctx := context.Background()

	devClient := StartIntegrationTestEnvironment(t)

	// Create a generic macOS VM
	err := devClient.VMs().Create(ctx, &v1.VM{
		Meta: v1.Meta{
			Name: "test-vm",
		},
		Image:    "ghcr.io/cirruslabs/macos-ventura-base:latest",
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
	})
	require.NoError(t, err)

	// Establish port forwarding to VMs SSH port
	wsConn, err := devClient.VMs().PortForward(ctx, "test-vm", 22, 120)
	require.NoError(t, err)

	vm, err := devClient.VMs().Get(ctx, "test-vm")
	require.NoError(t, err)
	require.Equal(t, v1.VMStatusRunning, vm.Status)
	require.Empty(t, vm.StatusMessage)

	t.Logf("Waiting for the VM to start, current status: %s", vm.Status)

	// Make sure we can connect to the VM over SSH via the forwarded port
	sshConfig := &ssh.ClientConfig{
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		User: "admin",
		Auth: []ssh.AuthMethod{
			ssh.Password("admin"),
		},
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(wsConn, "", sshConfig)
	require.NoError(t, err)

	sshClient := ssh.NewClient(sshConn, chans, reqs)

	sshSession, err := sshClient.NewSession()
	require.NoError(t, err)

	unameOutput, err := sshSession.Output("uname -mo")
	require.NoError(t, err)
	require.Contains(t, string(unameOutput), "Darwin arm64")
}

// TestSchedulerHealthCheckingNonExistentWorker ensures that scheduler
// will eventually fail VMs that are scheduled on a worker that was
// deleted from the API.
func TestSchedulerHealthCheckingNonExistentWorker(t *testing.T) {
	ctx := context.Background()

	devClient := StartIntegrationTestEnvironment(t)

	const (
		dummyWorkerName = "dummy-worker"
		dummyVMName     = "dummy-vm"
	)

	// Create a dummy worker that won't update it's LastSeen
	// timestamp, which will result in scheduler failing VMs
	// scheduled on that worker.
	//
	// We use a special resource "unique-resource" to prevent
	// our dummy VM (see below) from scheduling on any worker
	// other than this one.
	_, err := devClient.Workers().Create(ctx, v1.Worker{
		Meta: v1.Meta{
			Name: dummyWorkerName,
		},
		LastSeen:  time.Now(),
		MachineID: uuid.New().String(),
		Resources: map[string]uint64{
			v1.ResourceTartVMs: 1,
			"unique-resource":  1,
		},
	})
	require.NoError(t, err)

	// Create a dummy VM
	err = devClient.VMs().Create(context.Background(), &v1.VM{
		Meta: v1.Meta{
			Name: dummyVMName,
		},
		Image:    "ghcr.io/cirruslabs/macos-ventura-base:latest",
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
		Resources: map[string]uint64{
			"unique-resource": 1,
		},
	})
	require.NoError(t, err)

	// Wait for the dummy VM to get scheduled to a dummy worker
	require.True(t, Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), dummyVMName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM to be assigned to a dummy worker, current worker: %q", vm.Worker)

		return vm.Worker == dummyWorkerName
	}), "failed to wait for the dummy VM to be assigned to a dummy worker")

	// Delete the dummy worker
	err = devClient.Workers().Delete(ctx, dummyWorkerName)
	require.NoError(t, err)

	// Wait for the scheduler to change the dummy VM's status to "failed"
	require.True(t, Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), dummyVMName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM to be failed by the scheduler")

		return vm.Status == v1.VMStatusFailed
	}), "VM was not marked as failed in time")

	// Double check VM's status and status message
	vm, err := devClient.VMs().Get(context.Background(), dummyVMName)
	require.NoError(t, err)
	require.Equal(t, v1.VMStatusFailed, vm.Status)
	require.Equal(t, "VM is assigned to a worker that doesn't exist anymore", vm.StatusMessage)
}

// TestSchedulerHealthCheckingOfflineWorker ensures that scheduler
// will eventually fail VMs that are scheduled on a worker that had
// gone offline for a long time.
func TestSchedulerHealthCheckingOfflineWorker(t *testing.T) {
	ctx := context.Background()

	devClient := StartIntegrationTestEnvironmentWithAdditionalOpts(t,
		[]controller.Option{controller.WithWorkerOfflineTimeout(1 * time.Minute)}, nil)

	const (
		dummyWorkerName = "dummy-worker"
		dummyVMName     = "dummy-vm"
	)

	// Create a dummy worker that will be eventually marked as offline
	// because we won't update the LastSeen field
	_, err := devClient.Workers().Create(ctx, v1.Worker{
		Meta: v1.Meta{
			Name: dummyWorkerName,
		},
		LastSeen:  time.Now(),
		MachineID: uuid.New().String(),
		Resources: map[string]uint64{
			v1.ResourceTartVMs: 1,
			"unique-resource":  1,
		},
	})
	require.NoError(t, err)

	// Create a dummy VM that will be assigned to our dummy worker
	err = devClient.VMs().Create(context.Background(), &v1.VM{
		Meta: v1.Meta{
			Name: dummyVMName,
		},
		Image:    "ghcr.io/cirruslabs/macos-ventura-base:latest",
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
		Resources: map[string]uint64{
			"unique-resource": 1,
		},
	})
	require.NoError(t, err)

	// Wait for the VM to be marked as failed
	assert.True(t, Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), dummyVMName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM to be marked as failed, current status: %s", vm.Status)

		return vm.Status == v1.VMStatusFailed
	}), "VM wasn't marked as failed in a reasonable time")

	// Double-check the VM's status message
	runningVM, err := devClient.VMs().Get(context.Background(), dummyVMName)
	require.NoError(t, err)
	require.Equal(t, v1.VMStatusFailed, runningVM.Status)
	require.Equal(t, "VM is assigned to a worker that lost connection with the controller",
		runningVM.StatusMessage)
}

func TestVMGarbageCollection(t *testing.T) {
	ctx := context.Background()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	// Create on-disk Tart VM that looks like it's managed by Orchard
	vmName := ondiskname.New("test", uuid.New().String(), 0).String()
	_, _, err = tart.Tart(ctx, logger.Sugar(), "clone",
		"ghcr.io/cirruslabs/macos-ventura-base:latest", vmName)
	require.NoError(t, err)

	// Make sure that this VM exists
	require.True(t, hasVM(t, vmName, logger))

	// Start the Orchard Worker
	_ = StartIntegrationTestEnvironment(t)

	// Wait for the Orchard Worker to garbage-collect this VM
	require.True(t, Wait(2*time.Minute, func() bool {
		t.Logf("Waiting for the on-disk VM to be cleaned up by the worker")

		return !hasVM(t, vmName, logger)
	}), "failed to wait for the VM %s to be garbage-collected", vmName)
}

func TestHostDirs(t *testing.T) {
	devClient := StartIntegrationTestEnvironment(t)

	dirToMount := t.TempDir()

	vmName := "test-host-dirs-" + uuid.NewString()

	err := devClient.ClusterSettings().Set(context.Background(), &v1.ClusterSettings{
		HostDirPolicies: []v1.HostDirPolicy{{PathPrefix: dirToMount}},
	})
	require.NoError(t, err)

	scriptContent, err := os.ReadFile(filepath.Join("testdata", "host-dirs.sh"))
	require.NoError(t, err)

	err = devClient.VMs().Create(context.Background(), &v1.VM{
		Meta: v1.Meta{
			Name: vmName,
		},
		Image:    "ghcr.io/cirruslabs/macos-ventura-base:latest",
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
		Status:   v1.VMStatusPending,
		StartupScript: &v1.VMScript{
			ScriptContent: string(scriptContent),
		},
		HostDirs: []v1.HostDir{
			{Name: "readwrite", Path: dirToMount},
			{Name: "readonly", Path: dirToMount, ReadOnly: true},
		},
	})
	require.NoError(t, err)

	var vm *v1.VM

	require.True(t, Wait(2*time.Minute, func() bool {
		vm, err = devClient.VMs().Get(context.Background(), vmName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM to start. Current status: %s", vm.Status)

		return vm.Status == v1.VMStatusRunning || vm.Status == v1.VMStatusFailed
	}), "failed to start a VM")

	require.Empty(t, vm.StatusMessage)
	require.Equal(t, v1.VMStatusRunning, vm.Status)

	var logLines []string

	require.True(t, Wait(2*time.Minute, func() bool {
		logLines, err = devClient.VMs().Logs(context.Background(), vmName)
		require.NoError(t, err)

		return len(logLines) > 0
	}), "failed to wait for logs to become available")

	fmt.Println(logLines)

	require.EqualValues(t, []string{
		"Read-write mount exists",
		"Read-only mount exists",
		"Failed to create a file in read-only mount",
		"Successfully created a file in read-write mount",
	}, logLines)
	require.FileExists(t, filepath.Join(dirToMount, "test-rw.txt"))
	require.NoFileExists(t, filepath.Join(dirToMount, "test-ro.txt"))
}

func TestHostDirsInvalidPolicy(t *testing.T) {
	devClient := StartIntegrationTestEnvironment(t)

	dirToMount := t.TempDir()

	vmName := "test-host-dirs-" + uuid.NewString()

	// Create a VM without creating any directory policies
	// and make sure we get an error
	vmSpec := &v1.VM{
		Meta: v1.Meta{
			Name: vmName,
		},
		Image:    "ghcr.io/cirruslabs/macos-ventura-base:latest",
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
		Status:   v1.VMStatusPending,
		HostDirs: []v1.HostDir{
			{Name: "test" + uuid.NewString(), Path: dirToMount},
		},
	}

	err := devClient.VMs().Create(context.Background(), vmSpec)
	require.Error(t, err)

	// Create a policy for our directory, but do not allow for writing
	err = devClient.ClusterSettings().Set(context.Background(), &v1.ClusterSettings{
		HostDirPolicies: []v1.HostDirPolicy{{PathPrefix: dirToMount, ReadOnly: true}},
	})
	require.NoError(t, err)

	// Make sure we get error with the same spec
	err = devClient.VMs().Create(context.Background(), vmSpec)
	require.Error(t, err)
}

func hasVM(t *testing.T, name string, logger *zap.Logger) bool {
	return hasVMByPredicate(t, func(vmInfo tart.VMInfo) bool {
		return vmInfo.Name == name
	}, logger)
}

func hasVMByPredicate(t *testing.T, predicate func(tart.VMInfo) bool, logger *zap.Logger) bool {
	if logger == nil {
		logger = zap.Must(zap.NewDevelopment())
	}

	vmInfos, err := tart.List(context.Background(), logger.Sugar())
	require.NoError(t, err)

	return slices.ContainsFunc(vmInfos, predicate)
}

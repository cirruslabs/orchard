package tests_test

import (
	"context"
	"errors"
	"github.com/cirruslabs/orchard/internal/command/dev"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"net"
	"net/http"
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
		Softnet:  false,
		Headless: true,
		Status:   v1.VMStatusPending,
		StartupScript: &v1.VMScript{
			ScriptContent: "echo \"Hello, $FOO!\"",
			Env:           map[string]string{"FOO": "Bar"},
		},
		ShutdownScript: &v1.VMScript{
			ScriptContent: "echo \"Buy!\"",
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
	assert.Equal(t, []string{"Hello, Bar!"}, logLines)

	stoppingVM, err := devClient.VMs().Stop(context.Background(), "test-vm")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, v1.VMStatusStopping, stoppingVM.Status)
	assert.True(t, Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), "test-vm")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Waiting for the VM to stop. Current status: %s", vm.Status)
		return vm.Status == v1.VMStatusStopped
	}), "failed to stop a VM")
	logLines, err = devClient.VMs().Logs(context.Background(), "test-vm")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, []string{"Hello, Bar!", "Buy!"}, logLines)
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
		Softnet:  false,
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
	assert.Equal(t, "failed to run script: Process exited with status 123", runningVM.StatusMessage)
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

func StartIntegrationTestEnvironment(t *testing.T) *client.Client {
	t.Setenv("ORCHARD_HOME", t.TempDir())
	devController, devWorker, err := dev.CreateDevControllerAndWorker(t.TempDir(), ":0", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = devWorker.DeleteAllVMs()
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
		Softnet:  false,
		Headless: true,
	})
	require.NoError(t, err)

	// Wait for the VM to start
	var vm *v1.VM

	require.True(t, Wait(2*time.Minute, func() bool {
		vm, err = devClient.VMs().Get(ctx, "test-vm")
		require.NoError(t, err)

		t.Logf("Waiting for the VM to start, current status: %s", vm.Status)

		return vm.Status == v1.VMStatusRunning || vm.Status == v1.VMStatusFailed
	}), "failed to start a VM")

	require.Equal(t, v1.VMStatusRunning, vm.Status)
	require.Empty(t, vm.StatusMessage)

	// Establish port forwarding to VMs SSH port
	wsConn, err := devClient.VMs().PortForward(ctx, "test-vm", 22)
	require.NoError(t, err)

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

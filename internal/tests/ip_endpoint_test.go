package tests_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/imageconstant"
	"github.com/cirruslabs/orchard/internal/tests/devcontroller"
	"github.com/cirruslabs/orchard/internal/tests/wait"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestIPEndpoint(t *testing.T) {
	// Run the Controller
	devClient, _, _ := devcontroller.StartIntegrationTestEnvironment(t)

	// Create a VM to which we'll connect via Controller's SSH server
	err := devClient.VMs().Create(context.Background(), &v1.VM{
		Meta: v1.Meta{
			Name: "test-vm",
		},
		Image:    imageconstant.DefaultMacosImage,
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
		Status:   v1.VMStatusPending,
	})
	require.NoError(t, err)

	// Wait for the VM to start
	require.True(t, wait.Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), "test-vm")
		require.NoError(t, err)

		return vm.Status == v1.VMStatusRunning
	}), "failed to wait for the VM to start")

	// Retrieve the VM's IP
	ip, err := devClient.VMs().IP(context.Background(), "test-vm", 30)
	require.NoError(t, err)

	// Connect to the VM over SSH to make sure the provided IP is valid
	sshClient, err := ssh.Dial("tcp", ip+":22", &ssh.ClientConfig{
		User: "admin",
		Auth: []ssh.AuthMethod{
			ssh.Password("admin"),
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	})
	require.NoError(t, err)

	sshSession, err := sshClient.NewSession()
	require.NoError(t, err)

	output, err := sshSession.CombinedOutput("uname -a")
	require.NoError(t, err)
	require.Contains(t, string(output), "Darwin")
}

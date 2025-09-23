package tests_test

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/cirruslabs/orchard/internal/tests/devcontroller"
	"github.com/cirruslabs/orchard/internal/tests/wait"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

func TestSSHServer(t *testing.T) {
	// Generate SSH host key for the Controller
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(privateKey)
	require.NoError(t, err)

	// Run the Controller
	devClient, devController, _ := devcontroller.StartIntegrationTestEnvironmentWithAdditionalOpts(t,
		false, []controller.Option{
			controller.WithSSHServer(":0", signer, false),
		},
		false, nil,
	)

	// Create a VM to which we'll connect via Controller's SSH server
	err = devClient.VMs().Create(context.Background(), &v1.VM{
		Meta: v1.Meta{
			Name: "test-vm",
		},
		Image:    defaultMacosImage,
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
		Status:   v1.VMStatusPending,
	})
	require.NoError(t, err)

	// Wait for the VM to start
	assert.True(t, wait.Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), "test-vm")
		require.NoError(t, err)

		return vm.Status == v1.VMStatusRunning
	}), "failed to wait for the VM to start")

	// Create a service account whose credentials we'll use to connect to the Controller's SSH server
	require.NoError(t, devClient.ServiceAccounts().Create(context.Background(), &v1.ServiceAccount{
		Meta: v1.Meta{
			Name: "ssh-user",
		},
		Token: "ssh-password",
		Roles: []v1.ServiceAccountRole{
			v1.ServiceAccountRoleComputeWrite,
		},
	}))

	// Connect to the VM over Orchard Controller's SSH server
	sshAddress, ok := devController.SSHAddress()
	require.True(t, ok)

	sshClientController, err := ssh.Dial("tcp", sshAddress, &ssh.ClientConfig{
		User: "ssh-user",
		Auth: []ssh.AuthMethod{
			ssh.Password("ssh-password"),
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			if subtle.ConstantTimeCompare(sshPublicKey.Marshal(), key.Marshal()) != 1 {
				return fmt.Errorf("untrustred public key was presented by the Controller")
			}

			return nil
		},
	})
	require.NoError(t, err)

	netConnVM, err := sshClientController.Dial("tcp", "test-vm:22")
	require.NoError(t, err)

	sshConnVM, sshChansVM, sshReqsVM, err := ssh.NewClientConn(netConnVM, "test-vm:22", &ssh.ClientConfig{
		User: "admin",
		Auth: []ssh.AuthMethod{
			ssh.Password("admin"),
		},
		HostKeyCallback: func(_ string, _ net.Addr, _ ssh.PublicKey) error {
			return nil
		},
	})
	require.NoError(t, err)

	sshClientVM := ssh.NewClient(sshConnVM, sshChansVM, sshReqsVM)

	sshSessVM, err := sshClientVM.NewSession()
	require.NoError(t, err)

	unameBytes, err := sshSessVM.Output("uname -a")
	require.NoError(t, err)
	require.Contains(t, string(unameBytes), "Darwin")
}

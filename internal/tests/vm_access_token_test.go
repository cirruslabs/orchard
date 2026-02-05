package tests_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/cirruslabs/orchard/internal/echoserver"
	"github.com/cirruslabs/orchard/internal/imageconstant"
	"github.com/cirruslabs/orchard/internal/tests/wait"
	"github.com/cirruslabs/orchard/internal/vmtempauth"
	workerpkg "github.com/cirruslabs/orchard/internal/worker"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

type authenticatedTestEnvironment struct {
	adminClient *client.Client
	controller  *controller.Controller
	worker      *workerpkg.Worker
}

func TestVMAccessTokenAPI(t *testing.T) {
	env := startAuthenticatedTestEnvironment(t, false)
	defer func() {
		_ = env.worker.Close()
	}()

	vmA := createRunningSyntheticVM(t, env.adminClient, "test-vm-a")
	vmB := createRunningSyntheticVM(t, env.adminClient, "test-vm-b")

	echoServer, err := echoserver.New()
	require.NoError(t, err)

	echoContext, echoCancel := context.WithCancel(context.Background())
	defer echoCancel()

	go func() {
		_ = echoServer.Run(echoContext)
	}()

	echoPort := parsePort(t, echoServer.Addr())

	tokenA := issueAccessToken(t, env.adminClient, vmA.Name, nil)

	tokenClient, err := client.New(client.WithAddress(env.controller.Address()), client.WithBearerToken(tokenA.Token))
	require.NoError(t, err)

	ip, err := tokenClient.VMs().IP(context.Background(), vmA.Name, 30)
	require.NoError(t, err)
	require.NotEmpty(t, ip)

	vmAConn, err := tokenClient.VMs().PortForward(context.Background(), vmA.Name, echoPort, 30)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = vmAConn.Close()
	})

	require.NoError(t, vmAConn.SetDeadline(time.Now().Add(10*time.Second)))

	_, err = vmAConn.Write([]byte("hello"))
	require.NoError(t, err)

	result := make([]byte, len("hello"))
	_, err = io.ReadFull(vmAConn, result)
	require.NoError(t, err)
	require.Equal(t, "hello", string(result))

	_, err = tokenClient.VMs().IP(context.Background(), vmB.Name, 30)
	require.Error(t, err)
	require.ErrorIs(t, err, client.ErrAPI)
	require.Contains(t, err.Error(), "403")

	_, err = tokenClient.VMs().List(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, client.ErrAPI)
	require.Contains(t, err.Error(), "401")
}

func TestVMAccessTokenVMRecreation(t *testing.T) {
	env := startAuthenticatedTestEnvironment(t, false)
	defer func() {
		_ = env.worker.Close()
	}()

	vm := createRunningSyntheticVM(t, env.adminClient, "test-vm")

	token := issueAccessToken(t, env.adminClient, vm.Name, nil)

	tokenClient, err := client.New(client.WithAddress(env.controller.Address()), client.WithBearerToken(token.Token))
	require.NoError(t, err)

	require.NoError(t, env.adminClient.VMs().Delete(context.Background(), vm.Name))

	createRunningSyntheticVM(t, env.adminClient, vm.Name)

	_, err = tokenClient.VMs().IP(context.Background(), vm.Name, 30)
	require.Error(t, err)
	require.ErrorIs(t, err, client.ErrAPI)
	require.Contains(t, err.Error(), "403")
}

func TestVMAccessTokenSSHServer(t *testing.T) {
	env := startAuthenticatedTestEnvironment(t, true)
	defer func() {
		_ = env.worker.Close()
	}()

	vmA := createRunningSyntheticVM(t, env.adminClient, "test-vm-a")
	vmB := createRunningSyntheticVM(t, env.adminClient, "test-vm-b")

	echoServer, err := echoserver.New()
	require.NoError(t, err)

	echoContext, echoCancel := context.WithCancel(context.Background())
	defer echoCancel()

	go func() {
		_ = echoServer.Run(echoContext)
	}()

	echoPort := parsePort(t, echoServer.Addr())

	tokenA := issueAccessToken(t, env.adminClient, vmA.Name, nil)

	sshAddress, ok := env.controller.SSHAddress()
	require.True(t, ok)

	sshClientController, err := ssh.Dial("tcp", sshAddress, &ssh.ClientConfig{
		User: vmtempauth.SSHUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(tokenA.Token),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sshClientController.Close()
	})

	vmAConn, err := sshClientController.Dial("tcp", fmt.Sprintf("%s:%d", vmA.Name, echoPort))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = vmAConn.Close()
	})

	_, err = vmAConn.Write([]byte("jumpbox"))
	require.NoError(t, err)

	vmAResult := make([]byte, len("jumpbox"))
	_, err = io.ReadFull(vmAConn, vmAResult)
	require.NoError(t, err)
	require.Equal(t, "jumpbox", string(vmAResult))

	_, err = sshClientController.Dial("tcp", fmt.Sprintf("%s:%d", vmB.Name, echoPort))
	require.Error(t, err)

	shortTTL := uint64(1)
	shortToken := issueAccessToken(t, env.adminClient, vmA.Name, &shortTTL)

	time.Sleep(2 * time.Second)

	_, err = ssh.Dial("tcp", sshAddress, &ssh.ClientConfig{
		User: vmtempauth.SSHUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(shortToken.Token),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	require.Error(t, err)
}

func startAuthenticatedTestEnvironment(
	t *testing.T,
	withSSHServer bool,
) *authenticatedTestEnvironment {
	t.Helper()

	dataDir, err := controller.NewDataDir(t.TempDir())
	require.NoError(t, err)

	controllerOpts := []controller.Option{
		controller.WithDataDir(dataDir),
		controller.WithListenAddr(":0"),
		controller.WithExperimentalRPCV2(),
	}

	if withSSHServer {
		_, privateKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		signer, err := ssh.NewSignerFromKey(privateKey)
		require.NoError(t, err)

		controllerOpts = append(controllerOpts, controller.WithSSHServer(":0", signer, false))
	}

	controllerInstance, err := controller.New(controllerOpts...)
	require.NoError(t, err)

	require.NoError(t, controllerInstance.EnsureServiceAccount(&v1.ServiceAccount{
		Meta: v1.Meta{
			Name: "admin",
		},
		Token: "admin-token",
		Roles: v1.AllServiceAccountRoles(),
	}))
	require.NoError(t, controllerInstance.EnsureServiceAccount(&v1.ServiceAccount{
		Meta: v1.Meta{
			Name: "worker",
		},
		Token: "worker-token",
		Roles: []v1.ServiceAccountRole{
			v1.ServiceAccountRoleComputeRead,
			v1.ServiceAccountRoleComputeWrite,
		},
	}))

	adminClient, err := client.New(
		client.WithAddress(controllerInstance.Address()),
		client.WithCredentials("admin", "admin-token"),
	)
	require.NoError(t, err)

	workerClient, err := client.New(
		client.WithAddress(controllerInstance.Address()),
		client.WithCredentials("worker", "worker-token"),
	)
	require.NoError(t, err)

	workerInstance, err := workerpkg.New(workerClient, workerpkg.WithName("worker-a"), workerpkg.WithSynthetic())
	require.NoError(t, err)

	testContext, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		runErr := controllerInstance.Run(testContext)
		if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, http.ErrServerClosed) {
			t.Errorf("controller failed: %v", runErr)
		}
	}()

	go func() {
		runErr := workerInstance.Run(testContext)
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			t.Errorf("worker failed: %v", runErr)
		}
	}()

	assert.True(t, wait.Wait(30*time.Second, func() bool {
		workers, err := adminClient.Workers().List(context.Background())
		if err != nil {
			return false
		}

		return len(workers) == 1
	}), "failed to wait for worker to register")

	return &authenticatedTestEnvironment{
		adminClient: adminClient,
		controller:  controllerInstance,
		worker:      workerInstance,
	}
}

func createRunningSyntheticVM(t *testing.T, apiClient *client.Client, name string) *v1.VM {
	t.Helper()

	err := apiClient.VMs().Create(context.Background(), &v1.VM{
		Meta: v1.Meta{
			Name: name,
		},
		Image:    imageconstant.DefaultMacosImage,
		CPU:      1,
		Memory:   512,
		Headless: true,
		Status:   v1.VMStatusPending,
	})
	require.NoError(t, err)

	require.Truef(t, wait.Wait(60*time.Second, func() bool {
		vm, err := apiClient.VMs().Get(context.Background(), name)
		if err != nil {
			return false
		}

		return vm.Status == v1.VMStatusRunning
	}), "failed to wait for VM %q to reach running state", name)

	vm, err := apiClient.VMs().Get(context.Background(), name)
	require.NoError(t, err)

	return vm
}

func issueAccessToken(
	t *testing.T,
	apiClient *client.Client,
	vmName string,
	ttlSeconds *uint64,
) *v1.VMAccessToken {
	t.Helper()

	token, err := apiClient.VMs().IssueAccessToken(context.Background(), vmName, client.IssueAccessTokenOptions{
		TTLSeconds: ttlSeconds,
	})
	require.NoError(t, err)
	require.NotEmpty(t, token.Token)

	return token
}

func parsePort(t *testing.T, addr string) uint16 {
	t.Helper()

	_, portRaw, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	port, err := strconv.ParseUint(portRaw, 10, 16)
	require.NoError(t, err)

	return uint16(port)
}

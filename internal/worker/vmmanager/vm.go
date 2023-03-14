package vmmanager

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/avast/retry-go"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

var ErrVMFailed = errors.New("VM errored")

type VM struct {
	id       string
	Resource v1.VM
	logger   *zap.SugaredLogger
	RunError error

	ctx    context.Context
	cancel context.CancelFunc

	wg *sync.WaitGroup
}

func NewVM(ctx context.Context, vmResource v1.VM, logger *zap.SugaredLogger) (*VM, error) {
	vmContext, vmContextCancel := context.WithCancel(context.Background())

	vm := &VM{
		id:       fmt.Sprintf("orchard-%s-%s", vmResource.Name, vmResource.UID),
		Resource: vmResource,
		logger:   logger,

		ctx:    vmContext,
		cancel: vmContextCancel,

		wg: &sync.WaitGroup{},
	}

	// Clone the VM so `run` and `ip` are not racing
	if err := vm.cloneAndConfigure(ctx); err != nil {
		return nil, fmt.Errorf("failed to clone the VM: %w", err)
	}

	vm.wg.Add(1)

	go func() {
		defer vm.wg.Done()

		if err := vm.run(vm.ctx); err != nil {
			logger.Errorf("VM %s failed: %v", vm.id, err)
			vm.RunError = err
		}
	}()

	return vm, nil
}

func (vm *VM) cloneAndConfigure(ctx context.Context) error {
	_, _, err := vm.tart(ctx, "clone", vm.Resource.Image, vm.id)
	if err != nil {
		return err
	}

	if vm.Resource.Memory != 0 {
		_, _, err = vm.tart(ctx, "set", "--memory", strconv.FormatUint(vm.Resource.Memory, 10), vm.id)
		if err != nil {
			return err
		}
	}

	if vm.Resource.CPU != 0 {
		_, _, err = vm.tart(ctx, "set", "--cpu", strconv.FormatUint(vm.Resource.CPU, 10), vm.id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (vm *VM) run(ctx context.Context) error {
	var runArgs = []string{"run"}

	if vm.Resource.Softnet {
		runArgs = append(runArgs, "--net-softnet")
	}

	if vm.Resource.Headless {
		runArgs = append(runArgs, "--no-graphics")
	}

	runArgs = append(runArgs, vm.id)
	_, _, err := vm.tart(ctx, runArgs...)
	if err != nil {
		return err
	}

	return nil
}

func (vm *VM) IP(ctx context.Context) (string, error) {
	stdout, _, err := vm.tart(ctx, "ip", "--wait", "60", vm.id)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (vm *VM) Stop() error {
	_, _, _ = vm.tart(context.Background(), "stop", vm.id)

	vm.cancel()

	vm.wg.Wait()

	return nil
}

func (vm *VM) Delete() error {
	_, _, err := vm.tart(context.Background(), "delete", vm.id)
	if err != nil {
		return fmt.Errorf("%w: failed to delete VM %s: %v", ErrFailed, vm.id, err)
	}

	return nil
}

func (vm *VM) Shell(
	ctx context.Context,
	sshUser string,
	sshPassword string,
	script string,
	env map[string]string,
	consumeLine func(line string),
) error {
	ip, err := vm.IP(ctx)
	if err != nil {
		return fmt.Errorf("%w to get IP", ErrVMFailed)
	}

	var netConn net.Conn

	addr := ip + ":22"

	if err := retry.Do(func() error {
		dialer := net.Dialer{}

		netConn, err = dialer.DialContext(ctx, "tcp", addr)

		return err
	}, retry.Context(ctx)); err != nil {
		return fmt.Errorf("%w to dial: %v", ErrVMFailed, err)
	}

	// set default user and password if not provided
	if sshUser == "" && sshPassword == "" {
		sshUser = "admin"
		sshPassword = "admin"
	}

	sshConfig := &ssh.ClientConfig{
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPassword),
		},
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, addr, sshConfig)
	if err != nil {
		return fmt.Errorf("%w to connect via SSH: %v", ErrVMFailed, err)
	}
	cli := ssh.NewClient(sshConn, chans, reqs)

	sess, err := cli.NewSession()
	if err != nil {
		return fmt.Errorf("%w: failed to open SSH session: %v", ErrFailed, err)
	}

	// Log output from the virtual machine
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: while opening stdout pipe: %v", ErrFailed, err)
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		return fmt.Errorf("%w: while opening stderr pipe: %v", ErrFailed, err)
	}
	var outputReaderWG sync.WaitGroup
	outputReaderWG.Add(1)
	go func() {
		output := io.MultiReader(stdout, stderr)

		scanner := bufio.NewScanner(output)

		for scanner.Scan() {
			consumeLine(scanner.Text())
		}
		outputReaderWG.Done()
	}()

	stdinBuf, err := sess.StdinPipe()
	if err != nil {
		return fmt.Errorf("%w: while opening stdin pipe: %v", ErrFailed, err)
	}

	// start a login shell so all the customization from ~/.zprofile will be picked up
	err = sess.Shell()
	if err != nil {
		return fmt.Errorf("%w: failed to start a shell: %v", ErrFailed, err)
	}

	var scriptBuilder strings.Builder

	scriptBuilder.WriteString("set -e\n")
	// don't use sess.Setenv since it requires non-default SSH server configuration
	for key, value := range env {
		scriptBuilder.WriteString("export " + key + "=\"" + value + "\"\n")
	}
	scriptBuilder.WriteString(script)
	scriptBuilder.WriteString("\nexit\n")

	_, err = stdinBuf.Write([]byte(scriptBuilder.String()))
	if err != nil {
		return fmt.Errorf("%w: failed to start agent: %v", ErrFailed, err)
	}
	outputReaderWG.Wait()
	return sess.Wait()
}

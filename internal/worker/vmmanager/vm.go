package vmmanager

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/avast/retry-go"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/internal/worker/tart"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrVMFailed = errors.New("VM failed")

type VM struct {
	onDiskName ondiskname.OnDiskName
	Resource   v1.VM
	logger     *zap.SugaredLogger

	err atomic.Pointer[error]

	ctx    context.Context
	cancel context.CancelFunc

	wg *sync.WaitGroup
}

func NewVM(
	ctx context.Context,
	vmResource v1.VM,
	eventStreamer *client.EventStreamer,
	logger *zap.SugaredLogger,
) *VM {
	vmContext, vmContextCancel := context.WithCancel(context.Background())

	vm := &VM{
		onDiskName: ondiskname.NewFromResource(vmResource),
		Resource:   vmResource,
		logger: logger.With(
			"vm_uid", vmResource.UID,
			"vm_name", vmResource.Name,
			"vm_restart_count", vmResource.RestartCount,
		),

		ctx:    vmContext,
		cancel: vmContextCancel,

		wg: &sync.WaitGroup{},
	}

	// Optionally pull and clone the VM so that `run` and `ip` will not be racing
	vm.logger.Debugf("creating VM")

	if vmResource.ImagePullPolicy == v1.ImagePullPolicyAlways {
		_, _, err := tart.Tart(ctx, vm.logger, "pull", vm.Resource.Image)
		if err != nil {
			vm.setErr(fmt.Errorf("failed to pull the VM: %w", err))

			return vm
		}
	}

	if err := vm.cloneAndConfigure(ctx); err != nil {
		vm.setErr(fmt.Errorf("failed to clone the VM: %w", err))

		return vm
	}

	vm.wg.Add(1)

	go func() {
		defer vm.wg.Done()

		vm.logger.Debugf("spawned VM")

		if err := vm.run(vm.ctx); err != nil {
			vm.setErr(fmt.Errorf("%w: %v", ErrVMFailed, err))
		}

		vm.setErr(fmt.Errorf("%w: VM exited unexpectedly", ErrVMFailed))
	}()

	if vm.Resource.StartupScript != nil {
		go vm.runScript(vm.Resource.StartupScript, eventStreamer)
	}

	return vm
}

func (vm *VM) OnDiskName() ondiskname.OnDiskName {
	return vm.onDiskName
}

func (vm *VM) id() string {
	return vm.onDiskName.String()
}

func (vm *VM) Err() error {
	if err := vm.err.Load(); err != nil {
		return *err
	}

	return nil
}

func (vm *VM) setErr(err error) {
	if vm.err.CompareAndSwap(nil, &err) {
		vm.logger.Error(err)
	}
}

func (vm *VM) cloneAndConfigure(ctx context.Context) error {
	_, _, err := tart.Tart(ctx, vm.logger, "clone", vm.Resource.Image, vm.id())
	if err != nil {
		return err
	}

	if vm.Resource.Memory != 0 {
		_, _, err = tart.Tart(ctx, vm.logger, "set", "--memory",
			strconv.FormatUint(vm.Resource.Memory, 10), vm.id())
		if err != nil {
			return err
		}
	}

	if vm.Resource.CPU != 0 {
		_, _, err = tart.Tart(ctx, vm.logger, "set", "--cpu",
			strconv.FormatUint(vm.Resource.CPU, 10), vm.id())
		if err != nil {
			return err
		}
	}
	return nil
}

func (vm *VM) run(ctx context.Context) error {
	var runArgs = []string{"run"}

	if vm.Resource.NetSoftnet {
		runArgs = append(runArgs, "--net-softnet")
	}
	if vm.Resource.NetBridged != "" {
		runArgs = append(runArgs, fmt.Sprintf("--net-bridged=%s", vm.Resource.NetBridged))
	}

	if vm.Resource.Headless {
		runArgs = append(runArgs, "--no-graphics")
	}

	for _, hostDir := range vm.Resource.HostDirs {
		runArgs = append(runArgs, fmt.Sprintf("--dir=%s", hostDir.String()))
	}

	runArgs = append(runArgs, vm.id())
	_, _, err := tart.Tart(ctx, vm.logger, runArgs...)
	if err != nil {
		return err
	}

	return nil
}

func (vm *VM) IP(ctx context.Context) (string, error) {
	stdout, _, err := tart.Tart(ctx, vm.logger, "ip", "--wait", "60", vm.id())
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (vm *VM) Stop() error {
	vm.logger.Debugf("stopping VM")

	_, _, _ = tart.Tart(context.Background(), vm.logger, "stop", vm.id())

	vm.logger.Debugf("VM stopped")

	vm.cancel()

	vm.wg.Wait()

	return nil
}

func (vm *VM) Delete() error {
	vm.logger.Debugf("deleting VM")

	_, _, err := tart.Tart(context.Background(), vm.logger, "delete", vm.id())
	if err != nil {
		return fmt.Errorf("%w: failed to delete VM: %v", ErrVMFailed, err)
	}

	vm.logger.Debugf("deleted VM")

	return nil
}

func (vm *VM) shell(
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
		return fmt.Errorf("%w: failed to open SSH session: %v", ErrVMFailed, err)
	}

	// Log output from the virtual machine
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: while opening stdout pipe: %v", ErrVMFailed, err)
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		return fmt.Errorf("%w: while opening stderr pipe: %v", ErrVMFailed, err)
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
		return fmt.Errorf("%w: while opening stdin pipe: %v", ErrVMFailed, err)
	}

	// start a login shell so all the customization from ~/.zprofile will be picked up
	err = sess.Shell()
	if err != nil {
		return fmt.Errorf("%w: failed to start a shell: %v", ErrVMFailed, err)
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
		return fmt.Errorf("%w: failed to start script: %v", ErrVMFailed, err)
	}
	outputReaderWG.Wait()
	return sess.Wait()
}

func (vm *VM) runScript(script *v1.VMScript, eventStreamer *client.EventStreamer) {
	if eventStreamer != nil {
		defer func() {
			if err := eventStreamer.Close(); err != nil {
				vm.logger.Errorf("errored during streaming events for startup script: %v", err)
			}
		}()
	}

	consumeLine := func(line string) {
		if eventStreamer == nil {
			return
		}

		eventStreamer.Stream(v1.Event{
			Kind:      v1.EventKindLogLine,
			Timestamp: time.Now().Unix(),
			Payload:   line,
		})
	}

	err := vm.shell(context.Background(), vm.Resource.Username, vm.Resource.Password,
		script.ScriptContent, script.Env, consumeLine)
	if err != nil {
		vm.setErr(fmt.Errorf("%w: failed to run startup script: %v", ErrVMFailed, err))
	}
}

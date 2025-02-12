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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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

	// cloned state allows us to prevent a superfluous "tart delete" of a non-existent VM
	cloned atomic.Bool

	// started state allows us to only set VM's "status" field in the API
	// to "running" when we're indeed about to start the VM
	started atomic.Bool

	// stopping state allows us to catch unexpected
	// "tart run" terminations more correctly
	stopping atomic.Bool

	// Image FQN feature, see https://github.com/cirruslabs/orchard/issues/164
	imageFQN atomic.Pointer[string]

	err atomic.Pointer[error]

	ctx    context.Context
	cancel context.CancelFunc

	wg *sync.WaitGroup
}

func NewVM(
	vmResource v1.VM,
	eventStreamer *client.EventStreamer,
	vmPullTimeHistogram metric.Float64Histogram,
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

	vm.wg.Add(1)

	go func() {
		defer vm.wg.Done()

		if vmResource.ImagePullPolicy == v1.ImagePullPolicyAlways {
			vm.logger.Debugf("pulling VM")

			pullStartedAt := time.Now()

			_, _, err := tart.Tart(vm.ctx, vm.logger, "pull", vm.Resource.Image)
			if err != nil {
				select {
				case <-vm.ctx.Done():
					// Do not return an error because it's the user's intent to cancel this VM operation
				default:
					vm.setErr(fmt.Errorf("failed to pull the VM: %w", err))
				}

				return
			}

			vmPullTimeHistogram.Record(vm.ctx, time.Since(pullStartedAt).Seconds(), metric.WithAttributes(
				attribute.String("worker", vm.Resource.Worker),
				attribute.String("image", vm.Resource.Image),
			))
		}

		vm.logger.Debugf("creating VM")

		if err := vm.cloneAndConfigure(vm.ctx); err != nil {
			select {
			case <-vm.ctx.Done():
				// Do not return an error because it's the user's intent to cancel this VM operation
			default:
				vm.setErr(fmt.Errorf("failed to clone the VM: %w", err))
			}

			return
		}

		vm.cloned.Store(true)

		vm.logger.Debugf("spawned VM")

		// Launch the startup script goroutine as close as possible
		// to the VM startup (below) to avoid "tart ip" timing out
		if vm.Resource.StartupScript != nil {
			go vm.runScript(vm.Resource.StartupScript, eventStreamer)
		}

		vm.started.Store(true)

		if err := vm.run(vm.ctx); err != nil {
			select {
			case <-vm.ctx.Done():
				// Do not return an error because it's the user's intent to cancel this VM
			default:
				vm.setErr(fmt.Errorf("%w: %v", ErrVMFailed, err))
			}

			return
		}

		select {
		case <-vm.ctx.Done():
			// Do not return an error because it's the user's intent to cancel this VM
		default:
			if !vm.stopping.Load() {
				vm.setErr(fmt.Errorf("%w: VM exited unexpectedly", ErrVMFailed))
			}
		}
	}()

	return vm
}

func (vm *VM) OnDiskName() ondiskname.OnDiskName {
	return vm.onDiskName
}

func (vm *VM) Started() bool {
	return vm.started.Load()
}

func (vm *VM) ImageFQN() *string {
	return vm.imageFQN.Load()
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

	// Image FQN feature, see https://github.com/cirruslabs/orchard/issues/164
	fqnRaw, _, err := tart.Tart(ctx, vm.logger, "fqn", vm.Resource.Image)
	if err == nil {
		fqn := strings.TrimSpace(fqnRaw)
		vm.imageFQN.Store(&fqn)
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

	// Randomize the VM's MAC-address when using bridged networking
	// to avoid collisions when cloning from an OCI image on multiple hosts
	//
	// See https://github.com/cirruslabs/orchard/issues/181 for more details.
	if vm.Resource.NetBridged != "" {
		_, _, err = tart.Tart(ctx, vm.logger, "set", "--random-mac", vm.id())
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
	args := []string{"ip", "--wait", "60"}

	if vm.Resource.NetBridged != "" {
		args = append(args, "--resolver", "arp")
	}

	args = append(args, vm.id())

	stdout, _, err := tart.Tart(ctx, vm.logger, args...)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (vm *VM) Stop() {
	vm.logger.Debugf("stopping VM")

	vm.stopping.Store(true)

	// Try to gracefully terminate the VM
	_, _, _ = tart.Tart(context.Background(), zap.NewNop().Sugar(), "stop", "--timeout", "5", vm.id())

	// Terminate the VM goroutine ("tart pull", "tart clone", "tart run", etc.) via the context
	vm.cancel()
	vm.wg.Wait()

	vm.logger.Debugf("VM stopped")
}

func (vm *VM) Delete() error {
	if !vm.cloned.Load() {
		return nil
	}

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
	var sess *ssh.Session

	// Set default user and password if not provided
	if sshUser == "" && sshPassword == "" {
		sshUser = "admin"
		sshPassword = "admin"
	}

	// Configure SSH client
	sshConfig := &ssh.ClientConfig{
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPassword),
		},
	}

	if err := retry.Do(func() error {
		ip, err := vm.IP(ctx)
		if err != nil {
			return fmt.Errorf("failed to get VM's IP: %w", err)
		}

		addr := ip + ":22"

		dialer := net.Dialer{
			Timeout: 5 * time.Second,
		}

		netConn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to dial %s: %w", addr, err)
		}

		sshConn, chans, reqs, err := ssh.NewClientConn(netConn, addr, sshConfig)
		if err != nil {
			return fmt.Errorf("SSH handshake with %s failed: %w", addr, err)
		}

		sshClient := ssh.NewClient(sshConn, chans, reqs)

		sess, err = sshClient.NewSession()
		if err != nil {
			return fmt.Errorf("failed to open an SSH session on %s: %w", addr, err)
		}

		return nil
	}, retry.Context(ctx), retry.OnRetry(func(n uint, err error) {
		consumeLine(fmt.Sprintf("attempt %d to establish SSH connection failed: %v", n, err))
	})); err != nil {
		return fmt.Errorf("failed to establish SSH connection: %w", err)
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

	err := vm.shell(vm.ctx, vm.Resource.Username, vm.Resource.Password,
		script.ScriptContent, script.Env, consumeLine)
	if err != nil {
		vm.setErr(fmt.Errorf("%w: failed to run startup script: %v", ErrVMFailed, err))
	}
}

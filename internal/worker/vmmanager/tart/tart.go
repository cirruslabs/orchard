package tart

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go"
	"github.com/cirruslabs/orchard/internal/dialer"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/internal/worker/vmmanager/base"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

var ErrVMFailed = errors.New("VM failed")

type VM struct {
	onDiskName ondiskname.OnDiskName
	resource   v1.VM
	logger     *zap.SugaredLogger

	// Image FQN feature, see https://github.com/cirruslabs/orchard/issues/164
	imageFQN atomic.Pointer[string]

	ctx    context.Context
	cancel context.CancelFunc

	wg *sync.WaitGroup

	dialer dialer.Dialer

	*base.VM
}

func NewVM(
	vmResource v1.VM,
	eventStreamer *client.EventStreamer,
	vmPullTimeHistogram metric.Float64Histogram,
	dialer dialer.Dialer,
	logger *zap.SugaredLogger,
) *VM {
	vmContext, vmContextCancel := context.WithCancel(context.Background())

	vm := &VM{
		onDiskName: ondiskname.NewFromResource(vmResource),
		resource:   vmResource,
		logger: logger.With(
			"vm_uid", vmResource.UID,
			"vm_name", vmResource.Name,
			"vm_restart_count", vmResource.RestartCount,
		),

		ctx:    vmContext,
		cancel: vmContextCancel,

		wg: &sync.WaitGroup{},

		dialer: dialer,

		VM: base.NewVM(logger),
	}

	vm.wg.Add(1)

	go func() {
		defer vm.wg.Done()

		if vmResource.ImagePullPolicy == v1.ImagePullPolicyAlways {
			vm.SetStatusMessage("pulling VM image...")

			pullStartedAt := time.Now()

			_, _, err := Tart(vm.ctx, vm.logger, "pull", vm.resource.Image)
			if err != nil {
				select {
				case <-vm.ctx.Done():
					// Do not return an error because it's the user's intent to cancel this VM operation
				default:
					vm.SetErr(fmt.Errorf("failed to pull the VM: %w", err))
				}

				return
			}

			vmPullTimeHistogram.Record(vm.ctx, time.Since(pullStartedAt).Seconds(), metric.WithAttributes(
				attribute.String("worker", vm.resource.Worker),
				attribute.String("image", vm.resource.Image),
			))
		}

		if err := vm.cloneAndConfigure(vm.ctx); err != nil {
			select {
			case <-vm.ctx.Done():
				// Do not return an error because it's the user's intent to cancel this VM operation
			default:
				vm.SetErr(fmt.Errorf("failed to clone the VM: %w", err))
			}

			return
		}

		// Backward compatibility with v1.VM specification's "Status" field
		vm.SetStarted(true)

		vm.ConditionsSet().Add(v1.ConditionTypeRunning)

		vm.run(vm.ctx, eventStreamer)
	}()

	return vm
}

func (vm *VM) Resource() v1.VM {
	return vm.resource
}

func (vm *VM) SetResource(vmResource v1.VM) {
	vm.resource = vmResource
	vm.resource.ObservedGeneration = vmResource.Generation
}

func (vm *VM) OnDiskName() ondiskname.OnDiskName {
	return vm.onDiskName
}

func (vm *VM) ImageFQN() *string {
	return vm.imageFQN.Load()
}

func (vm *VM) id() string {
	return vm.onDiskName.String()
}

func (vm *VM) cloneAndConfigure(ctx context.Context) error {
	vm.SetStatusMessage("cloning VM...")

	_, _, err := Tart(ctx, vm.logger, "clone", vm.resource.Image, vm.id())
	if err != nil {
		return err
	}

	vm.ConditionsSet().Remove(v1.ConditionTypeCloning)

	// Image FQN feature, see https://github.com/cirruslabs/orchard/issues/164
	fqnRaw, _, err := Tart(ctx, vm.logger, "fqn", vm.resource.Image)
	if err == nil {
		fqn := strings.TrimSpace(fqnRaw)
		vm.imageFQN.Store(&fqn)
	}

	// Set memory
	vm.SetStatusMessage("configuring VM...")

	memory := vm.resource.AssignedMemory

	if memory == 0 {
		memory = vm.resource.Memory
	}

	if memory != 0 {
		_, _, err = Tart(ctx, vm.logger, "set", "--memory",
			strconv.FormatUint(memory, 10), vm.id())
		if err != nil {
			return err
		}
	}

	// Set CPU
	cpu := vm.resource.AssignedCPU

	if cpu == 0 {
		cpu = vm.resource.CPU
	}

	if cpu != 0 {
		_, _, err = Tart(ctx, vm.logger, "set", "--cpu",
			strconv.FormatUint(cpu, 10), vm.id())
		if err != nil {
			return err
		}
	}

	if diskSize := vm.resource.DiskSize; diskSize != 0 {
		_, _, err = Tart(ctx, vm.logger, "set", "--disk-size",
			strconv.FormatUint(diskSize, 10), vm.id())
		if err != nil {
			return err
		}
	}

	// Randomize VM's MAC-address, this is important when using shared (NAT) networking
	// with full /var/db/dhcpd_leases file (e.g. 256 entries) having an expired entry
	// for a MAC address used by some OCI image, for example:
	//
	// {
	//	name=adminsVlMachine
	//	ip_address=192.168.64.2
	//	hw_address=1,11:11:11:11:11:11
	//	identifier=1,11:11:11:11:11:11
	//	lease=0x1234
	//}
	//
	// The next VM to start with a MAC address 22:22:22:22:22:22 will assume that
	// 192.168.64.2 is free (because its lease expired a long time ago) and will
	// add a new entry using its MAC address and 192.168.64.2 to the
	// /var/db/dhcpd_leases and won't delete the old entry:
	//
	// {
	//	name=adminsVlMachine
	//	ip_address=192.168.64.2
	//	hw_address=1,11:11:11:11:11:11
	//	identifier=1,11:11:11:11:11:11
	//	lease=0x1234
	// }
	// {
	//	name=adminsVlMachine
	//	ip_address=192.168.64.2
	//	hw_address=1,22:22:22:22:22:22
	//	identifier=1,22:22:22:22:22:22
	//	lease=0x67ade532
	// }
	//
	// Afterward, when an OCI VM with MAC address 11:11:11:11:11:11 is cloned and run,
	// it will re-use the 192.168.64.2 entry instead of creating a new one, even through
	// its lease had already expired. The resulting /var/db/dhcpd_leases will look like this:
	//
	// {
	//	name=adminsVlMachine
	//	ip_address=192.168.64.2
	//	hw_address=1,11:11:11:11:11:11
	//	identifier=1,11:11:11:11:11:11
	//	lease=0x67ade5c6
	// }
	// {
	//	name=adminsVlMachine
	//	ip_address=192.168.64.2
	//	hw_address=1,22:22:22:22:22:22
	//	identifier=1,22:22:22:22:22:22
	//	lease=0x67ade532
	// }
	//
	// As a result, you will see two VMs with different MAC address using an identical
	// IP address 192.168.64.2.
	//
	// Another scenarion when this is important is when using bridged networking
	// to avoid collisions when cloning from an OCI image on multiple hosts[1].
	//
	// [1]: https://github.com/cirruslabs/orchard/issues/181
	_, _, err = Tart(ctx, vm.logger, "set", "--random-mac", vm.id())
	if err != nil {
		return err
	}

	if vm.resource.RandomSerial {
		_, _, err = Tart(ctx, vm.logger, "set", "--random-serial", vm.id())
		if err != nil {
			return err
		}
	}

	return nil
}

func (vm *VM) run(ctx context.Context, eventStreamer *client.EventStreamer) {
	defer vm.ConditionsSet().RemoveAll(v1.ConditionTypeRunning, v1.ConditionTypeSuspending, v1.ConditionTypeStopping)

	// Launch the startup script goroutine as close as possible
	// to the VM startup (below) to avoid "tart ip" timing out
	if vm.resource.StartupScript != nil {
		vm.SetStatusMessage("VM started, running startup script...")

		go vm.runScript(vm.resource.StartupScript, eventStreamer)
	} else {
		vm.SetStatusMessage("VM started")
	}

	var runArgs = []string{"run"}

	if vm.resource.NetSoftnetDeprecated || vm.resource.NetSoftnet {
		runArgs = append(runArgs, "--net-softnet")
	}
	if len(vm.resource.NetSoftnetAllow) != 0 {
		runArgs = append(runArgs, "--net-softnet-allow", strings.Join(vm.resource.NetSoftnetAllow, ","))
	}
	if len(vm.resource.NetSoftnetBlock) != 0 {
		runArgs = append(runArgs, "--net-softnet-block", strings.Join(vm.resource.NetSoftnetBlock, ","))
	}
	if vm.resource.NetBridged != "" {
		runArgs = append(runArgs, fmt.Sprintf("--net-bridged=%s", vm.resource.NetBridged))
	}

	if vm.resource.Headless {
		runArgs = append(runArgs, "--no-graphics")
	}

	if vm.resource.Nested {
		runArgs = append(runArgs, "--nested")
	}

	if vm.resource.Suspendable {
		runArgs = append(runArgs, "--suspendable")
	}

	for _, hostDir := range vm.resource.HostDirs {
		runArgs = append(runArgs, fmt.Sprintf("--dir=%s", hostDir.String()))
	}

	runArgs = append(runArgs, vm.id())
	_, _, err := Tart(ctx, vm.logger, runArgs...)
	if err != nil {
		select {
		case <-vm.ctx.Done():
			// Do not return an error because it's the user's intent to cancel this VM
		default:
			vm.SetErr(fmt.Errorf("%w: %v", ErrVMFailed, err))
		}

		return
	}

	select {
	case <-vm.ctx.Done():
		// Do not return an error because it's the user's intent to cancel this VM
	default:
		if !vm.ConditionsSet().ContainsAny(v1.ConditionTypeSuspending, v1.ConditionTypeStopping) {
			vm.SetErr(fmt.Errorf("%w: VM exited unexpectedly", ErrVMFailed))
		}
	}
}

func (vm *VM) IP(ctx context.Context) (string, error) {
	// Bridged networking is problematic, so try with
	// the agent resolver first using a small timeout
	if vm.resource.NetBridged != "" {
		stdout, _, err := Tart(ctx, vm.logger, "ip", "--wait", "5",
			"--resolver", "agent", vm.id())
		if err == nil {
			return strings.TrimSpace(stdout), nil
		}
	}

	args := []string{"ip", "--wait", "60"}

	if vm.resource.NetBridged != "" {
		args = append(args, "--resolver", "arp")
	}

	args = append(args, vm.id())

	stdout, _, err := Tart(ctx, vm.logger, args...)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (vm *VM) Suspend() <-chan error {
	errCh := make(chan error, 1)

	select {
	case <-vm.ctx.Done():
		// VM is already suspended/stopped
		errCh <- nil

		return errCh
	default:
		// VM is still running
	}

	vm.SetStatusMessage("Suspending VM")
	vm.ConditionsSet().Add(v1.ConditionTypeSuspending)

	go func() {
		_, _, err := Tart(context.Background(), zap.NewNop().Sugar(), "suspend", vm.id())
		if err != nil {
			err := fmt.Errorf("failed to suspend VM: %w", err)
			vm.SetErr(err)
			errCh <- err

			return
		}

		errCh <- nil
	}()

	return errCh
}

func (vm *VM) Stop() <-chan error {
	errCh := make(chan error, 1)

	select {
	case <-vm.ctx.Done():
		// VM is already suspended/stopped
		errCh <- nil

		return errCh
	default:
		// VM is still running
	}

	vm.SetStatusMessage("Stopping VM")
	vm.ConditionsSet().Add(v1.ConditionTypeStopping)

	go func() {
		// Try to gracefully terminate the VM
		_, _, _ = Tart(context.Background(), zap.NewNop().Sugar(), "stop", "--timeout", "5", vm.id())

		// Terminate the VM goroutine ("tart pull", "tart clone", "tart run", etc.) via the context
		vm.cancel()
		vm.wg.Wait()

		// We don't return an error because we always terminate a VM
		errCh <- nil
	}()

	return errCh
}

func (vm *VM) Start(eventStreamer *client.EventStreamer) {
	vm.SetStatusMessage("Starting VM")
	vm.ConditionsSet().Add(v1.ConditionTypeRunning)

	vm.cancel()

	vm.ctx, vm.cancel = context.WithCancel(context.Background())
	vm.wg.Add(1)

	go func() {
		defer vm.wg.Done()

		vm.run(vm.ctx, eventStreamer)
	}()
}

func (vm *VM) Delete() error {
	// Cancel all currently running Tart invocations
	// (e.g. "tart clone", "tart run", etc.)
	vm.cancel()

	if vm.ConditionsSet().Contains(v1.ConditionTypeCloning) {
		// Not cloned yet, nothing to delete
		return nil
	}

	_, _, err := Tart(context.Background(), vm.logger, "delete", vm.id())
	if err != nil {
		return fmt.Errorf("%w: failed to delete VM: %v", ErrVMFailed, err)
	}

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

		dialCtx, dialCtxCancel := context.WithTimeout(ctx, 5*time.Second)
		defer dialCtxCancel()

		var netConn net.Conn

		if vm.dialer != nil {
			netConn, err = vm.dialer.DialContext(dialCtx, "tcp", addr)
		} else {
			dialer := net.Dialer{}

			netConn, err = dialer.DialContext(dialCtx, "tcp", addr)
		}
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

	err := vm.shell(vm.ctx, vm.resource.Username, vm.resource.Password,
		script.ScriptContent, script.Env, consumeLine)
	if err != nil {
		vm.SetErr(fmt.Errorf("%w: failed to run startup script: %v", ErrVMFailed, err))
	}
}

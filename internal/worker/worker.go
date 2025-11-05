package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/cirruslabs/chacha/pkg/localnetworkhelper"
	"github.com/cirruslabs/orchard/internal/opentelemetry"
	"github.com/cirruslabs/orchard/internal/worker/dhcpleasetime"
	"github.com/cirruslabs/orchard/internal/worker/iokitregistry"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/internal/worker/tart"
	"github.com/cirruslabs/orchard/internal/worker/vmmanager"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/dustin/go-humanize"
	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
	"github.com/samber/mo"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

const pollInterval = 5 * time.Second

var ErrPollFailed = errors.New("failed to poll controller")

type Worker struct {
	name          string
	syncRequested chan bool
	vmm           *vmmanager.VMManager
	client        *client.Client
	pollTicker    *time.Ticker
	resources     v1.Resources
	labels        v1.Labels

	defaultCPU    uint64
	defaultMemory uint64

	vmPullTimeHistogram metric.Float64Histogram

	localNetworkHelper *localnetworkhelper.LocalNetworkHelper

	logger *zap.SugaredLogger
}

func New(client *client.Client, opts ...Option) (*Worker, error) {
	worker := &Worker{
		client:        client,
		pollTicker:    time.NewTicker(pollInterval),
		vmm:           vmmanager.New(),
		syncRequested: make(chan bool, 1),
	}

	// Apply options
	for _, opt := range opts {
		opt(worker)
	}

	// Apply defaults
	if worker.name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}

		worker.name = hostname
	}

	defaultResources := v1.Resources{
		v1.ResourceTartVMs: 2,
	}

	// Determine the number of the host's logical CPU cores
	numLogicalCPUs, err := cpu.Counts(true)
	if err != nil {
		worker.logger.Warnf("cannot determine the number of host's logical CPU cores, "+
			"%s resource will not be available: %v", v1.ResourceLogicalCores, err)
	} else {
		defaultResources[v1.ResourceLogicalCores] = uint64(numLogicalCPUs)
	}

	// Determine the size of the host's memory
	virtualMemoryStat, err := mem.VirtualMemory()
	if err != nil {
		worker.logger.Warnf("cannot determine the size of the host's memory, "+
			"%s resource will not be available: %v", v1.ResourceMemoryMiB, err)
	} else {
		defaultResources[v1.ResourceMemoryMiB] = virtualMemoryStat.Total / humanize.MiByte
	}

	worker.resources = defaultResources.Merged(worker.resources)

	// Worker, VMs and images-related metrics
	worker.vmPullTimeHistogram, err = opentelemetry.DefaultMeter.Float64Histogram(
		"org.cirruslabs.orchard.worker.vm.pull_time",
	)
	if err != nil {
		return nil, err
	}

	if worker.logger == nil {
		worker.logger = zap.NewNop().Sugar()
	}

	return worker, nil
}

func (worker *Worker) Run(ctx context.Context) error {
	if err := dhcpleasetime.Check(); err != nil {
		worker.logger.Warnf("%v", err)
	}

	for {
		if err := worker.runNewSession(ctx); err != nil {
			return err
		}

		select {
		case <-worker.pollTicker.C:
			// continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (worker *Worker) Close() error {
	var result error
	for _, vm := range worker.vmm.List() {
		vm.Stop()
	}
	for _, vm := range worker.vmm.List() {
		err := vm.Delete()
		if err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (worker *Worker) runNewSession(ctx context.Context) error {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := worker.registerWorker(subCtx); err != nil {
		worker.logger.Warnf("failed to register worker: %v", err)

		return nil
	}

	info, err := worker.client.Controller().Info(ctx)
	if err != nil {
		worker.logger.Warnf("failed to retrieve controller info: %v", err)

		return nil
	}

	if info.Capabilities.Has(v1.ControllerCapabilityRPCV2) {
		worker.logger.Infof("using WebSocket-based v2 RPC")

		go func() {
			_ = retry.Do(func() error {
				return worker.watchRPCV2(subCtx)
			}, retry.OnRetry(func(n uint, err error) {
				worker.logger.Warnf("failed to watch RPC v2: %v", err)
			}), retry.Context(subCtx), retry.Attempts(0))
		}()
	} else {
		worker.logger.Infof("using gRPC-based v1 RPC")

		go func() {
			_ = retry.Do(func() error {
				return worker.watchRPC(subCtx)
			}, retry.OnRetry(func(n uint, err error) {
				worker.logger.Warnf("failed to watch RPC v1: %v", err)
			}), retry.Context(subCtx), retry.Attempts(0))
		}()
	}

	// Backward compatibility with for older Orchard Controllers
	updateFunc := worker.client.VMs().UpdateState

	if !info.Capabilities.Has(v1.ControllerCapabilityVMStateEndpoint) {
		updateFunc = worker.client.VMs().Update
	}

	// Sync on-disk VMs
	if err := worker.syncOnDiskVMs(ctx, updateFunc); err != nil {
		worker.logger.Errorf("failed to sync on-disk VMs: %v", err)

		return nil
	}

	for {
		if err := worker.updateWorker(ctx); err != nil {
			worker.logger.Errorf("failed to update worker resource: %v", err)

			return nil
		}

		if err := worker.syncVMs(subCtx, updateFunc); err != nil {
			worker.logger.Warnf("failed to sync VMs: %v", err)

			return nil
		}

		select {
		case <-worker.syncRequested:
		case <-worker.pollTicker.C:
			// continue
		case <-subCtx.Done():
			return subCtx.Err()
		}
	}
}

func (worker *Worker) registerWorker(ctx context.Context) error {
	platformUUID, err := iokitregistry.PlatformUUID()
	if err != nil {
		return err
	}

	_, err = worker.client.Workers().Create(ctx, v1.Worker{
		Meta: v1.Meta{
			Name: worker.name,
		},
		Resources:     worker.resources,
		Labels:        worker.labels,
		LastSeen:      time.Now(),
		MachineID:     platformUUID,
		DefaultCPU:    worker.defaultCPU,
		DefaultMemory: worker.defaultMemory,
	})
	if err != nil {
		return err
	}

	worker.logger.Infof("registered worker %s", worker.name)

	return nil
}

func (worker *Worker) updateWorker(ctx context.Context) error {
	workerResource, err := worker.client.Workers().Get(ctx, worker.name)
	if err != nil {
		return fmt.Errorf("%w: failed to retrieve worker from the API: %v", ErrPollFailed, err)
	}

	worker.logger.Debugf("got worker from the API")

	workerResource.LastSeen = time.Now()

	if _, err := worker.client.Workers().Update(ctx, *workerResource); err != nil {
		return fmt.Errorf("%w: failed to update worker in the API: %v", ErrPollFailed, err)
	}

	worker.logger.Debugf("updated worker in the API")

	return nil
}

//nolint:nestif,gocognit // nested "if" and cognitive complexity is tolerable for now
func (worker *Worker) syncVMs(ctx context.Context, updateVM func(context.Context, v1.VM) (*v1.VM, error)) error {
	allKeys := mapset.NewSet[ondiskname.OnDiskName]()

	remoteVMs, err := worker.client.VMs().FindForWorker(ctx, worker.name)
	if err != nil {
		return err
	}
	remoteVMsIndex := map[ondiskname.OnDiskName]*v1.VM{}
	for _, remoteVM := range remoteVMs {
		onDiskName := ondiskname.NewFromResource(remoteVM)
		allKeys.Add(onDiskName)
		remoteVMsIndex[onDiskName] = &remoteVM
	}

	localVMsIndex := map[ondiskname.OnDiskName]*vmmanager.VM{}
	for _, vm := range worker.vmm.List() {
		onDiskName := vm.OnDiskName()
		allKeys.Add(onDiskName)
		localVMsIndex[onDiskName] = vm
	}

	worker.logger.Infof("syncing %d local VMs against %d remote VMs...",
		len(localVMsIndex), len(remoteVMsIndex))

	var pairs []lo.Tuple3[ondiskname.OnDiskName, *v1.VM, *vmmanager.VM]

	for onDiskName := range allKeys.Iter() {
		vmResource := remoteVMsIndex[onDiskName]
		vm := localVMsIndex[onDiskName]

		pairs = append(pairs, lo.T3(onDiskName, vmResource, vm))
	}

	// It's important to process the remote VMs in failed state
	// and local VMs that ceased to exist remotely first, otherwise
	// we risk violating the scheduler resource assumptions
	sortNonExistentAndFailedFirst(pairs)

	for _, tuple := range pairs {
		onDiskName, vmResource, vm := lo.Unpack3(tuple)

		remoteState := mo.None[v1.VMStatus]()
		if vmResource != nil {
			remoteState = mo.Some(vmResource.Status)
		}

		localState := mo.None[v1.VMStatus]()
		if vm != nil {
			localState = mo.Some(vm.Status())
		}

		action := transitions[remoteState][localState]

		worker.logger.Debugf("processing VM: %s, remote: %v, local: %v, action: %v\n", onDiskName,
			optionToString(remoteState), optionToString(localState), action)

		switch action {
		case ActionCreate:
			// Remote VM was created, but not the local VM
			worker.createVM(onDiskName, *vmResource)
		case ActionMonitorPending:
			if vmResource.StatusMessage != vm.StatusMessage() {
				vmResource.StatusMessage = vm.StatusMessage()

				if _, err := updateVM(ctx, *vmResource); err != nil {
					return err
				}
			}
		case ActionReportRunning:
			// Remote VM was created, and the local VM too,
			// check if the local VM had already started
			// and update the remote VM as accordingly

			// Image FQN feature, see https://github.com/cirruslabs/orchard/issues/164
			if imageFQN := vm.ImageFQN(); imageFQN != nil {
				vmResource.ImageFQN = *imageFQN
			}

			// Mark the remote VM as started
			vmResource.Status = v1.VMStatusRunning
			vmResource.StatusMessage = vm.StatusMessage()

			if _, err := updateVM(ctx, *vmResource); err != nil {
				return err
			}
		case ActionMonitorRunning:
			if vmResource.StatusMessage != vm.StatusMessage() {
				vmResource.StatusMessage = vm.StatusMessage()

				if _, err := updateVM(ctx, *vmResource); err != nil {
					return err
				}
			}

			if vmResource.Generation != vm.Resource.Generation {
				// Something changed, reboot the VM for the changes to take effect
				vm.Resource = *vmResource

				eventStreamer := worker.client.VMs().StreamEvents(vmResource.Name)

				vm.Reboot(eventStreamer)

				vmResource.ObservedGeneration = vm.Resource.Generation

				if _, err := updateVM(ctx, *vmResource); err != nil {
					return err
				}
			}
		case ActionStop:
			// VM has failed on the remote side, stop it locally to prevent incorrect
			// worker's resources calculation in the Controller's scheduler
			vm.Stop()
		case ActionFail, ActionLostTrack, ActionImpossible:
			// VM has failed on the local side, stop it before reporting as failed to prevent incorrect
			// worker's resources calculation in the Controller's scheduler
			if vm != nil {
				vm.Stop()
			}

			var statusMessage string

			switch action {
			case ActionFail:
				statusMessage = vm.Err().Error()
			case ActionLostTrack:
				statusMessage = "Worker lost track of VM"
			case ActionImpossible:
				statusMessage = "Encountered an impossible transition"
			}

			vmResource.Status = v1.VMStatusFailed
			vmResource.StatusMessage = statusMessage
			if _, err := updateVM(ctx, *vmResource); err != nil {
				return err
			}
		case ActionDelete:
			// Remote VM was deleted, delete local VM
			//
			// Note: this check needs to run for each VM
			// before we attempt to create any VMs below.
			if err := worker.deleteVM(vm); err != nil {
				return err
			}
		}
	}

	return nil
}

//nolint:nestif,gocognit // complexity is tolerable for now
func (worker *Worker) syncOnDiskVMs(ctx context.Context, updateVM func(context.Context, v1.VM) (*v1.VM, error)) error {
	remoteVMs, err := worker.client.VMs().FindForWorker(ctx, worker.name)
	if err != nil {
		return err
	}
	remoteVMsIndex := map[ondiskname.OnDiskName]v1.VM{}
	for _, remoteVM := range remoteVMs {
		remoteVMsIndex[ondiskname.NewFromResource(remoteVM)] = remoteVM
	}

	worker.logger.Infof("syncing on-disk VMs...")

	vmInfos, err := tart.List(ctx, worker.logger)
	if err != nil {
		return err
	}

	for _, vmInfo := range vmInfos {
		onDiskName, err := ondiskname.Parse(vmInfo.Name)
		if err != nil {
			if errors.Is(err, ondiskname.ErrNotManagedByOrchard) {
				continue
			}

			return err
		}

		// VMs that exist in the Worker's VM manager will be handled in the syncVMs()
		if worker.vmm.Exists(onDiskName) {
			continue
		}

		remoteVM, ok := remoteVMsIndex[onDiskName]
		if !ok {
			// On-disk VM doesn't exist on the controller nor in the Worker's VM manager,
			// stop it (if applicable) and delete it
			if vmInfo.Running {
				_, _, err := tart.Tart(ctx, worker.logger, "stop", vmInfo.Name)
				if err != nil {
					worker.logger.Warnf("failed to stop")
				}
			}

			_, _, err := tart.Tart(ctx, worker.logger, "delete", vmInfo.Name)
			if err != nil {
				return err
			}
		} else if remoteVM.Status != v1.VMStatusPending {
			// On-disk VM exists on the controller and was acted upon,
			// but we've lost track of it, so shut it down (if applicable)
			// and report the error (if not failed yet)
			if vmInfo.Running {
				_, _, err := tart.Tart(ctx, worker.logger, "stop", vmInfo.Name)
				if err != nil {
					worker.logger.Warnf("failed to stop")
				}
			}
		}
	}

	return nil
}

func (worker *Worker) deleteVM(vm *vmmanager.VM) error {
	vm.Stop()

	if err := vm.Delete(); err != nil {
		return err
	}

	worker.vmm.Delete(vm.OnDiskName())

	return nil
}

func (worker *Worker) createVM(odn ondiskname.OnDiskName, vmResource v1.VM) {
	eventStreamer := worker.client.VMs().StreamEvents(vmResource.Name)

	vm := vmmanager.NewVM(vmResource, eventStreamer, worker.vmPullTimeHistogram,
		worker.localNetworkHelper, worker.logger)

	worker.vmm.Put(odn, vm)
}

func (worker *Worker) grpcMetadata() metadata.MD {
	return metadata.Join(
		worker.client.GPRCMetadata(),
		metadata.Pairs(rpc.MetadataWorkerNameKey, worker.name),
	)
}

func (worker *Worker) requestVMSyncing() {
	select {
	case worker.syncRequested <- true:
		worker.logger.Debugf("Successfully requested syncing")
	default:
		worker.logger.Debugf("There's already a syncing request in the queue, skipping")
	}
}

func sortNonExistentAndFailedFirst(input []lo.Tuple3[ondiskname.OnDiskName, *v1.VM, *vmmanager.VM]) {
	slices.SortStableFunc(input, func(left, right lo.Tuple3[ondiskname.OnDiskName, *v1.VM, *vmmanager.VM]) int {
		_, leftVM, _ := lo.Unpack3(left)
		_, rightVM, _ := lo.Unpack3(right)

		leftNonExistent := leftVM == nil
		rightNonExistent := rightVM == nil

		switch {
		case leftNonExistent && rightNonExistent:
			return 0
		case leftNonExistent:
			return -1
		case rightNonExistent:
			return 1
		}

		leftFailed := leftVM != nil && leftVM.Status == v1.VMStatusFailed
		rightFailed := rightVM != nil && rightVM.Status == v1.VMStatusFailed

		switch {
		case leftFailed && rightFailed:
			return 0
		case leftFailed:
			return -1
		case rightFailed:
			return 1
		default:
			return 0
		}
	})
}

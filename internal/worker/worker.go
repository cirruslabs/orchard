package worker

import (
	"context"
	"errors"
	"fmt"
	"github.com/avast/retry-go/v4"
	"github.com/cirruslabs/orchard/internal/worker/iokitregistry"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/internal/worker/tart"
	"github.com/cirruslabs/orchard/internal/worker/vmmanager"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
	"os"
	"time"
)

const pollInterval = 5 * time.Second

var ErrPollFailed = errors.New("failed to poll controller")
var ErrRegistrationFailed = errors.New("failed to register worker on the controller")

type Worker struct {
	name          string
	syncRequested chan bool
	vmm           *vmmanager.VMManager
	client        *client.Client
	resources     v1.Resources
	logger        *zap.SugaredLogger
}

func New(client *client.Client, opts ...Option) (*Worker, error) {
	worker := &Worker{
		client:        client,
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
	worker.resources = defaultResources.Merged(worker.resources)

	if worker.logger == nil {
		worker.logger = zap.NewNop().Sugar()
	}

	return worker, nil
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		if err := worker.runNewSession(ctx); err != nil {
			return err
		}
	}
}

func (worker *Worker) runNewSession(ctx context.Context) error {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := worker.registerWorker(subCtx); err != nil {
		worker.logger.Warnf("failed to register worker: %v", err)

		return ErrRegistrationFailed
	}

	go func() {
		_ = retry.Do(func() error {
			return worker.watchRPC(subCtx)
		}, retry.OnRetry(func(n uint, err error) {
			worker.logger.Warnf("failed to watch RPC: %v", err)
		}), retry.Context(subCtx), retry.Attempts(0))
	}()

	for {
		if err := worker.updateWorker(ctx); err != nil {
			worker.logger.Errorf("failed to update worker resource: %v", err)

			return nil
		}

		if err := worker.syncVMs(subCtx); err != nil {
			worker.logger.Warnf("failed to sync VMs: %v", err)

			return nil
		}

		select {
		case <-worker.syncRequested:
		case <-time.After(pollInterval):
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
		Resources: worker.resources,
		LastSeen:  time.Now(),
		MachineID: platformUUID,
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

func (worker *Worker) syncVMs(ctx context.Context) error {
	remoteVMs, err := worker.client.VMs().FindForWorker(ctx, worker.name)
	if err != nil {
		return err
	}

	worker.logger.Infof("syncing %d VMs...", len(remoteVMs))

	// Check if we need to stop any of the VMs
	for _, vmResource := range remoteVMs {
		if vmResource.Status == v1.VMStatusStopping && worker.vmm.Exists(vmResource) {
			if err := worker.stopVM(vmResource); err != nil {
				return err
			}
		}
	}

	// Handle pending VMs
	for _, vmResource := range remoteVMs {
		// handle pending VMs
		if vmResource.Status == v1.VMStatusPending && !worker.vmm.Exists(vmResource) {
			if err := worker.createVM(ctx, vmResource); err != nil {
				return err
			}
		}
	}

	// Sync in-memory VMs
	for _, vm := range worker.vmm.List() {
		remoteVM, ok := remoteVMs[vm.Resource.UID]
		if !ok {
			if err := worker.deleteVM(vm.Resource); err != nil {
				return err
			}
		} else if remoteVM.Status != v1.VMStatusFailed && vm.RunError != nil {
			remoteVM.Status = v1.VMStatusFailed
			remoteVM.StatusMessage = fmt.Sprintf("failed to run VM: %v", vm.RunError)
			updatedVM, err := worker.client.VMs().Update(ctx, vm.Resource)
			if err != nil {
				return err
			}
			vm.Resource = *updatedVM
		}
	}

	// Sync on-disk VMs
	if err := worker.syncOnDiskVMs(ctx, remoteVMs); err != nil {
		return err
	}

	return nil
}

func (worker *Worker) syncOnDiskVMs(ctx context.Context, remoteVMs map[string]v1.VM) error {
	vmInfos, err := tart.List(ctx, worker.logger)
	if err != nil {
		return err
	}

	for _, vmInfo := range vmInfos {
		if vmInfo.Running {
			continue
		}

		onDiskName, err := ondiskname.Parse(vmInfo.Name)
		if err != nil {
			if errors.Is(err, ondiskname.ErrNotManagedByOrchard) {
				continue
			}

			return err
		}

		remoteVM, ok := remoteVMs[onDiskName.UID]
		if !ok {
			// On-disk VM doesn't exist on the controller, delete it
			_, _, err := tart.Tart(ctx, worker.logger, "delete", vmInfo.Name)
			if err != nil {
				return err
			}
		} else if remoteVM.Status == v1.VMStatusRunning && !worker.vmm.Exists(v1.VM{UID: onDiskName.UID}) {
			// On-disk VM exist on the controller,
			// but we don't know about it, so
			// mark it as failed
			remoteVM.Status = v1.VMStatusFailed
			remoteVM.StatusMessage = "Worker lost track of VM"
			_, err := worker.client.VMs().Update(ctx, remoteVM)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (worker *Worker) deleteVM(vmResource v1.VM) error {
	worker.logger.Debugf("deleting VM %s (%s)", vmResource.Name, vmResource.UID)

	if !vmResource.TerminalState() {
		if err := worker.stopVM(vmResource); err != nil {
			return err
		}
	}

	// Delete VM locally, report to the controller
	if worker.vmm.Exists(vmResource) {
		if err := worker.vmm.Delete(vmResource); err != nil {
			return err
		}
	}

	worker.logger.Infof("deleted VM %s (%s)", vmResource.Name, vmResource.UID)

	return nil
}

func (worker *Worker) createVM(ctx context.Context, vmResource v1.VM) error {
	worker.logger.Debugf("creating VM %s (%s)", vmResource.Name, vmResource.UID)

	// Create or update VM locally
	vm, err := worker.vmm.Create(ctx, vmResource, worker.logger)
	if err != nil {
		vmResource.Status = v1.VMStatusFailed
		vmResource.StatusMessage = fmt.Sprintf("VM creation failed: %v", err)
		_, updateErr := worker.client.VMs().Update(context.Background(), vmResource)
		if updateErr != nil {
			worker.logger.Errorf("failed to update VM %s (%s) remotely: %s", vmResource.Name, vmResource.UID, updateErr.Error())
		}
		return err
	}

	worker.logger.Infof("spawned VM %s (%s)", vmResource.Name, vmResource.UID)

	vmResource.Status = v1.VMStatusRunning
	_, updateErr := worker.client.VMs().Update(context.Background(), vmResource)
	if updateErr != nil {
		worker.logger.Errorf("failed to update VM %s (%s) remotely: %s", vmResource.Name, vmResource.UID, updateErr.Error())
	}

	go func() {
		err := worker.execScript(vmResource, vm.Resource.StartupScript)
		if err != nil {
			vmResource.Status = v1.VMStatusFailed
			vmResource.StatusMessage = fmt.Sprintf("failed to run script: %v", err)
			_, updateErr := worker.client.VMs().Update(context.Background(), vmResource)
			if updateErr != nil {
				worker.logger.Errorf("failed to update VM %s (%s) remotely: %s", vmResource.Name, vmResource.UID, updateErr.Error())
			}
		}
	}()

	return nil
}

func (worker *Worker) execScript(vmResource v1.VM, script *v1.VMScript) error {
	if script == nil {
		return nil
	}
	vm, err := worker.vmm.Get(vmResource)
	if err != nil {
		return nil
	}

	eventsStreamer := worker.client.VMs().StreamEvents(vmResource.Name)
	defer func() {
		err := eventsStreamer.Close()
		if err != nil {
			worker.logger.Errorf("errored during streaming events for %s (%s): %w", vmResource.Name, vmResource.UID, err)
		}
	}()
	err = vm.Shell(context.Background(), vmResource.Username, vmResource.Password,
		script.ScriptContent, script.Env,
		func(line string) {
			eventsStreamer.Stream(v1.Event{
				Kind:      v1.EventKindLogLine,
				Timestamp: time.Now().Unix(),
				Payload:   line,
			})
		})
	if err != nil {
		worker.logger.Errorf("failed to run script for VM %s (%s): %s", vmResource.Name, vmResource.UID, err.Error())
	}
	return err
}

func (worker *Worker) stopVM(vmResource v1.VM) error {
	worker.logger.Debugf("stopping VM %s (%s)", vmResource.Name, vmResource.UID)

	// Create or update VM locally
	if !worker.vmm.Exists(vmResource) {
		return nil
	}

	shutdownScriptErr := worker.execScript(vmResource, vmResource.ShutdownScript)
	stopErr := worker.vmm.Stop(vmResource)
	vmResource.Status = v1.VMStatusStopped
	if stopErr != nil {
		vmResource.Status = v1.VMStatusFailed
		vmResource.StatusMessage = fmt.Sprintf("failed to stop vm: %v", stopErr)
	}
	if shutdownScriptErr != nil {
		vmResource.Status = v1.VMStatusFailed
		vmResource.StatusMessage = fmt.Sprintf("failed to run shutdown script: %v", shutdownScriptErr)
	}

	_, err := worker.client.VMs().Update(context.Background(), vmResource)
	if err != nil {
		worker.logger.Errorf("failed to update VM %s (%s) remotely: %s", vmResource.Name, vmResource.UID, err.Error())
	}
	return stopErr
}

func (worker *Worker) DeleteAllVMs() error {
	var result error
	for _, vm := range worker.vmm.List() {
		err := vm.Stop()
		if err != nil {
			result = multierror.Append(result, err)
		}
	}
	for _, vm := range worker.vmm.List() {
		err := vm.Delete()
		if err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (worker *Worker) GPRCMetadata() metadata.MD {
	return metadata.Join(
		worker.client.GPRCMetadata(),
		metadata.Pairs(rpc.MetadataWorkerNameKey, worker.name),
	)
}

func (worker *Worker) RequestVMSyncing() {
	select {
	case worker.syncRequested <- true:
		worker.logger.Debugf("Successfully requested syncing")
	default:
		worker.logger.Debugf("There's already a syncing request in the queue, skipping")
	}
}

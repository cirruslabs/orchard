package scheduler

import (
	"context"
	"github.com/cirruslabs/orchard/internal/controller/notifier"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"go.uber.org/zap"
	"sort"
	"time"
)

const schedulerInterval = 5 * time.Second

type Scheduler struct {
	store                storepkg.Store
	notifier             *notifier.Notifier
	workerOfflineTimeout time.Duration
	logger               *zap.SugaredLogger
	schedulingRequested  chan bool
}

func NewScheduler(
	store storepkg.Store,
	notifier *notifier.Notifier,
	workerOfflineTimeout time.Duration,
	logger *zap.SugaredLogger,
) *Scheduler {
	return &Scheduler{
		store:                store,
		notifier:             notifier,
		workerOfflineTimeout: workerOfflineTimeout,
		logger:               logger,
		schedulingRequested:  make(chan bool, 1),
	}
}

func (scheduler *Scheduler) Run() {
	for {
		// wait either the scheduling interval or a request to schedule
		select {
		case <-scheduler.schedulingRequested:
		case <-time.After(schedulerInterval):
		}

		if err := scheduler.healthCheckingLoopIteration(); err != nil {
			scheduler.logger.Errorf("Failed to health-check VMs: %v", err)
		}
		if err := scheduler.schedulingLoopIteration(); err != nil {
			scheduler.logger.Errorf("Failed to schedule VMs: %v", err)
		}
	}
}

func (scheduler *Scheduler) RequestScheduling() {
	select {
	case scheduler.schedulingRequested <- true:
		scheduler.logger.Debugf("Successfully requested scheduling")
	default:
		scheduler.logger.Debugf("There's already a scheduling request in the queue, skipping")
	}
}

func (scheduler *Scheduler) schedulingLoopIteration() error {
	affectedWorkers := map[string]bool{}

	err := scheduler.store.Update(func(txn storepkg.Transaction) error {
		vms, err := txn.ListVMs()
		if err != nil {
			return err
		}
		unscheduledVMs, workerToResources := processVMs(vms)

		workers, err := txn.ListWorkers()
		if err != nil {
			return err
		}

		for _, unscheduledVM := range unscheduledVMs {
			// Find a worker that can run this VM
			for _, worker := range workers {
				resourcesUsed := workerToResources.Get(worker.Name)
				resourcesRemaining := worker.Resources.Subtracted(resourcesUsed)

				if resourcesRemaining.CanFit(unscheduledVM.Resources) &&
					!worker.Offline(scheduler.workerOfflineTimeout) &&
					!worker.Unschedulable {
					unscheduledVM.Worker = worker.Name

					if err := txn.SetVM(unscheduledVM); err != nil {
						return err
					}
					affectedWorkers[worker.Name] = true

					workerToResources.Add(worker.Name, unscheduledVM.Resources)
				}
			}
		}

		return nil
	})

	syncVMsInstruction := rpc.WatchInstruction{
		Action: &rpc.WatchInstruction_SyncVmsAction{},
	}
	for workerToPoke := range affectedWorkers {
		// it's fine to ignore the error here, since the worker will sync the VMs on the next cycle
		notifyErr := scheduler.notifier.Notify(context.Background(), workerToPoke, &syncVMsInstruction)
		if notifyErr != nil {
			scheduler.logger.Errorf("Failed to reactively sync VMs on worker %s: %v", workerToPoke, notifyErr)
		}
	}

	return err
}

func processVMs(vms []v1.VM) ([]v1.VM, WorkerToResources) {
	var unscheduledVMs []v1.VM
	workerToResources := make(WorkerToResources)

	for _, vm := range vms {
		if vm.Worker == "" {
			unscheduledVMs = append(unscheduledVMs, vm)
		} else if !vm.TerminalState() {
			workerToResources.Add(vm.Worker, vm.Resources)
		}
	}

	// Sort unscheduled VMs by the date of creation
	sort.Slice(unscheduledVMs, func(i, j int) bool {
		return unscheduledVMs[i].CreatedAt.Before(unscheduledVMs[j].CreatedAt)
	})

	return unscheduledVMs, workerToResources
}

func (scheduler *Scheduler) healthCheckingLoopIteration() error {
	return scheduler.store.Update(func(txn storepkg.Transaction) error {
		// Retrieve scheduled VMs
		vms, err := txn.ListVMs()
		if err != nil {
			return err
		}

		var scheduledVMs []v1.VM

		for _, vm := range vms {
			if vm.Worker != "" {
				scheduledVMs = append(scheduledVMs, vm)
			}
		}

		// Retrieve and index workers by name
		workers, err := txn.ListWorkers()
		if err != nil {
			return err
		}

		nameToWorker := map[string]v1.Worker{}
		for _, worker := range workers {
			nameToWorker[worker.Name] = worker
		}

		// Process scheduled VMs
		for _, scheduledVM := range scheduledVMs {
			if err := scheduler.healthCheckVM(txn, nameToWorker, scheduledVM); err != nil {
				return err
			}
		}

		return nil
	})
}

func (scheduler *Scheduler) healthCheckVM(txn storepkg.Transaction, nameToWorker map[string]v1.Worker, vm v1.VM) error {
	worker, ok := nameToWorker[vm.Worker]
	if !ok {
		vm.Status = v1.VMStatusFailed
		vm.StatusMessage = "VM is assigned to a worker that " +
			"doesn't exist anymore"

		return txn.SetVM(vm)
	}

	if worker.Offline(scheduler.workerOfflineTimeout) && !vm.TerminalState() {
		vm.Status = v1.VMStatusFailed
		vm.StatusMessage = "VM is assigned to a worker that " +
			"lost connection with the controller"

		return txn.SetVM(vm)
	}

	return nil
}

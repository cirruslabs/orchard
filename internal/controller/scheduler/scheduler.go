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
	store               storepkg.Store
	notifier            *notifier.Notifier
	logger              *zap.SugaredLogger
	schedulingRequested chan bool
}

func NewScheduler(store storepkg.Store, notifier *notifier.Notifier, logger *zap.SugaredLogger) *Scheduler {
	return &Scheduler{
		store:               store,
		notifier:            notifier,
		logger:              logger,
		schedulingRequested: make(chan bool, 1),
	}
}

func (scheduler *Scheduler) Run() {
	for {
		// wait either the scheduling interval or a request to schedule
		select {
		case <-scheduler.schedulingRequested:
		case <-time.After(schedulerInterval):
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
		scheduledVMs, unscheduledVMs, workerToResources := processVMs(vms)

		workers, err := txn.ListWorkers()
		if err != nil {
			return err
		}

		for _, unscheduledVM := range unscheduledVMs {
			// Find a worker that can run this VM
			for _, worker := range workers {
				resourcesUsed := workerToResources.Get(worker.Name)
				resourcesRemaining := worker.Resources.Subtracted(resourcesUsed)

				if resourcesRemaining.CanFit(unscheduledVM.Resources) && !worker.Offline() {
					unscheduledVM.Worker = worker.Name

					if err := txn.SetVM(unscheduledVM); err != nil {
						return err
					}
					affectedWorkers[worker.Name] = true

					workerToResources.Add(worker.Name, unscheduledVM.Resources)
				}
			}
		}

		// Process scheduled VMs
		nameToWorker := map[string]v1.Worker{}
		for _, worker := range workers {
			nameToWorker[worker.Name] = worker
		}

		for _, scheduledVM := range scheduledVMs {
			if err := processScheduledVM(txn, nameToWorker, scheduledVM); err != nil {
				return err
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

func processScheduledVM(txn storepkg.Transaction, nameToWorker map[string]v1.Worker, scheduledVM v1.VM) error {
	worker, ok := nameToWorker[scheduledVM.Worker]
	if !ok {
		scheduledVM.Status = v1.VMStatusFailed
		scheduledVM.StatusMessage = "VM is assigned to a worker that " +
			"doesn't exist anymore"

		return txn.SetVM(scheduledVM)
	}

	if worker.Offline() {
		scheduledVM.Status = v1.VMStatusFailed
		scheduledVM.StatusMessage = "VM is assigned to a worker that " +
			"lost connection with the controller"

		return txn.SetVM(scheduledVM)
	}

	return nil
}

func processVMs(vms []v1.VM) ([]v1.VM, []v1.VM, WorkerToResources) {
	var scheduledVMs []v1.VM
	var unscheduledVMs []v1.VM
	workerToResources := make(WorkerToResources)

	for _, vm := range vms {
		if vm.Worker == "" {
			unscheduledVMs = append(unscheduledVMs, vm)
		} else if !vm.TerminalState() {
			scheduledVMs = append(scheduledVMs, vm)
			workerToResources.Add(vm.Worker, vm.Resources)
		}
	}

	// Sort unscheduled VMs by the date of creation
	sort.Slice(unscheduledVMs, func(i, j int) bool {
		return unscheduledVMs[i].CreatedAt.Before(unscheduledVMs[j].CreatedAt)
	})

	return scheduledVMs, unscheduledVMs, workerToResources
}

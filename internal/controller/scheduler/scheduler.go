package scheduler

import (
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"sort"
	"time"
)

const schedulerInterval = 5 * time.Second

func Run(store storepkg.Store) error {
	ticker := time.NewTicker(schedulerInterval)

	for {
		if err := runInner(store); err != nil {
			return err
		}

		<-ticker.C
	}
}

func runInner(store storepkg.Store) error {
	return store.Update(func(txn storepkg.Transaction) error {
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

				if resourcesRemaining.CanFit(unscheduledVM.Resources) {
					unscheduledVM.Worker = worker.Name

					if err := txn.SetVM(unscheduledVM); err != nil {
						return err
					}

					workerToResources.Add(worker.Name, unscheduledVM.Resources)
				}
			}
		}

		return nil
	})
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

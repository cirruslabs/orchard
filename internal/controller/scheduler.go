package controller

import (
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"sort"
	"time"
)

const schedulerInterval = 5 * time.Second

func runScheduler(store *storepkg.Store) error {
	ticker := time.NewTicker(schedulerInterval)

	for {
		if err := runSchedulerInner(store); err != nil {
			return err
		}

		<-ticker.C
	}
}

func runSchedulerInner(store *storepkg.Store) error {
	var vms []*v1.VM
	var workers []*v1.Worker
	var err error

	err = store.View(func(txn *storepkg.Txn) error {
		vms, err = txn.ListVMs()
		if err != nil {
			return err
		}

		workers, err = txn.ListWorkers()
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Sort VMs by date of creation
	sort.Slice(vms, func(i, j int) bool {
		return vms[i].CreatedAt.Before(vms[j].CreatedAt)
	})

	for _, vm := range vms {
		vm := vm

		if vm.Worker != "" {
			continue
		}

		// Find an appropriate worker to run this VM on
		for _, worker := range workers {
			worker := worker

			vm.Worker = worker.Name

			err := store.Update(func(txn *storepkg.Txn) error {
				return txn.SetVM(vm)
			})
			if err != nil {
				return err
			}

			break
		}
	}

	return nil
}

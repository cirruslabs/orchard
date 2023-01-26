package controller

import (
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"time"
)

const janitorInterval = 5 * time.Second

func (controller *Controller) runJanitor(store *storepkg.Store) error {
	ticker := time.Tick(janitorInterval)

	for {
		if err := controller.runJanitorInner(store); err != nil {
			return err
		}

		<-ticker
	}
}

func (controller *Controller) runJanitorInner(store *storepkg.Store) error {
	var workers []*v1.Worker
	var err error

	err = store.View(func(txn *storepkg.Txn) error {
		workers, err = txn.ListWorkers()

		return err
	})
	if err != nil {
		return err
	}

	for _, worker := range workers {
		if time.Now().Sub(worker.LastSeen).Minutes() > 1 {
			controller.logger.Debugf("removing outdated worker %s", worker.Name)

			err := store.Update(func(txn *storepkg.Txn) error {
				return txn.DeleteWorker(worker.Name)
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

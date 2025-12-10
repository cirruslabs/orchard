package scheduler

import (
	"errors"
	"fmt"
	"time"

	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

func (scheduler *Scheduler) imagePullLoopIteration() error {
	// Get a lagging view of image pulls, image pull jobs and workers
	var imagePulls []v1.ImagePull
	var imagePullJobs []v1.ImagePullJob
	var workers []v1.Worker

	if err := scheduler.store.View(func(txn storepkg.Transaction) error {
		var err error

		imagePulls, err = txn.ListImagePulls()
		if err != nil {
			return err
		}

		imagePullJobs, err = txn.ListImagePullJobs()
		if err != nil {
			return err
		}

		workers, err = txn.ListWorkers()
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	scheduler.logger.Debugf("processing %d image pull jobs, %d image pulls and %d workers",
		len(imagePullJobs), len(imagePulls), len(workers))

	// Schedule new image pulls and update image pull job states
	imagePullJobIndex := map[string]v1.ImagePullJob{}

	for _, imagePullJob := range imagePullJobs {
		imagePullJobIndex[imagePullJob.Name] = imagePullJob

		// Schedule new image pulls
		existingImagePulls := lo.Filter(imagePulls, func(imagePull v1.ImagePull, _ int) bool {
			return lo.ContainsBy(imagePull.OwnerReferences, func(ownerReference v1.OwnerReference) bool {
				return ownerReference == imagePullJob.OwnerReference()
			})
		})

		for _, worker := range workers {
			// Should we create an image pull for this worker?
			if !worker.Labels.Contains(imagePullJob.Labels) {
				continue
			}

			// Have we already created an image pull for this worker?
			if _, ok := lo.Find(existingImagePulls, func(imagePull v1.ImagePull) bool {
				return imagePull.Worker == worker.Name
			}); ok {
				continue
			}

			// Create an image pull for this worker
			scheduler.logger.Debugf("creating image pull for job %s and worker %s",
				imagePullJob.Name, worker.Name)

			newImagePull, err := scheduler.createImagePull(imagePullJob, worker)
			if err != nil {
				return err
			}

			existingImagePulls = append(existingImagePulls, *newImagePull)
		}

		// Craft the current image pull job state
		newImagePullJobState := v1.ImagePullJobState{
			Total: int64(len(existingImagePulls)),
			Progressing: int64(lo.CountBy(existingImagePulls, func(imagePull v1.ImagePull) bool {
				return v1.ConditionIsTrue(imagePull.Conditions, v1.ConditionTypeProgressing)
			})),
			Succeeded: int64(lo.CountBy(existingImagePulls, func(imagePull v1.ImagePull) bool {
				return v1.ConditionIsTrue(imagePull.Conditions, v1.ConditionTypeCompleted)
			})),
			Failed: int64(lo.CountBy(existingImagePulls, func(imagePull v1.ImagePull) bool {
				return v1.ConditionIsTrue(imagePull.Conditions, v1.ConditionTypeFailed)
			})),
		}

		if newImagePullJobState.Progressing != 0 {
			newImagePullJobState.Conditions = append(newImagePullJobState.Conditions, v1.Condition{
				Type:  v1.ConditionTypeProgressing,
				State: v1.ConditionStateTrue,
			})
		}

		if newImagePullJobState.Failed != 0 {
			newImagePullJobState.Conditions = append(newImagePullJobState.Conditions, v1.Condition{
				Type:  v1.ConditionTypeFailed,
				State: v1.ConditionStateTrue,
			})
		} else if (newImagePullJobState.Progressing + newImagePullJobState.Succeeded + newImagePullJobState.Failed) == newImagePullJobState.Total {
			newImagePullJobState.Conditions = append(newImagePullJobState.Conditions, v1.Condition{
				Type:  v1.ConditionTypeCompleted,
				State: v1.ConditionStateTrue,
			})
		}

		// Is the current image pull job state any different?
		if cmp.Equal(imagePullJob.ImagePullJobState, newImagePullJobState) {
			continue
		}

		// Update image pull job state
		if err := scheduler.store.Update(func(txn storepkg.Transaction) error {
			dbPullJob, err := txn.GetImagePullJob(imagePullJob.Name)
			if err != nil {
				// Is the image pull job still exists?
				if errors.Is(err, storepkg.ErrNotFound) {
					return nil
				}

				return err
			}

			// Is it the same image pull job?
			if dbPullJob.UID != imagePullJob.UID {
				return nil
			}

			dbPullJob.ImagePullJobState = newImagePullJobState

			return txn.SetImagePullJob(*dbPullJob)
		}); err != nil {
			return err
		}
	}

	// Garbage collect orphaned image pulls
	for _, imagePull := range imagePulls {
		// Is this image pull controlled by an image pull job?
		imagePullJobOwnerReferences := lo.Filter(imagePull.OwnerReferences, func(ownerReference v1.OwnerReference, _ int) bool {
			return ownerReference.Kind == v1.KindImagePullJob
		})

		if len(imagePullJobOwnerReferences) == 0 {
			continue
		}

		// Does this image pull has any invalid references?
		hasOrphanedPullJobOwnerReferences := lo.ContainsBy(imagePullJobOwnerReferences, func(ownerReference v1.OwnerReference) bool {
			imagePullJob, ok := imagePullJobIndex[ownerReference.Name]
			if !ok {
				return true
			}

			return ownerReference != imagePullJob.OwnerReference()
		})

		if !hasOrphanedPullJobOwnerReferences {
			continue
		}

		scheduler.logger.Debugf("removing image pull %s with non-existent owner reference", imagePull.Name)

		if err := scheduler.store.Update(func(txn storepkg.Transaction) error {
			// Is this image pull still exists?
			dbImagePull, err := txn.GetImagePull(imagePull.Name)
			if err != nil {
				if errors.Is(err, storepkg.ErrNotFound) {
					scheduler.logger.Warnf("image pull %s is gone, perhaps the user "+
						"manually deleted it?", imagePull.Name)
					return nil
				}

				return err
			}

			// Is this the same image pull?
			if imagePull.UID != dbImagePull.UID {
				return nil
			}

			return txn.DeleteImagePull(imagePull.Name)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (scheduler *Scheduler) createImagePull(imagePullJob v1.ImagePullJob, worker v1.Worker) (*v1.ImagePull, error) {
	var imagePull v1.ImagePull

	if err := scheduler.store.Update(func(txn storepkg.Transaction) error {
		dbPullJob, err := txn.GetImagePullJob(imagePullJob.Name)
		if err != nil {
			// Is the image pull job still exists?
			if errors.Is(err, storepkg.ErrNotFound) {
				return nil
			}

			return err
		}

		// Is it the same pull job?
		if dbPullJob.UID != imagePullJob.UID {
			return nil
		}

		dbWorker, err := txn.GetWorker(worker.Name)
		if err != nil {
			// Is the worker still exists?
			if errors.Is(err, storepkg.ErrNotFound) {
				return nil
			}

			return err
		}

		// Do the worker labels still match?
		if !dbWorker.Labels.Contains(dbPullJob.Labels) {
			return nil
		}

		imagePull = v1.ImagePull{
			Meta: v1.Meta{
				Name:      fmt.Sprintf("%s-%s", dbPullJob.Name, dbWorker.Name),
				CreatedAt: time.Now(),
			},
			UID: uuid.NewString(),
			OwnerReferences: []v1.OwnerReference{
				dbPullJob.OwnerReference(),
			},
			Image:  dbPullJob.Image,
			Worker: worker.Name,
		}

		_, err = txn.GetImagePull(imagePull.Name)
		if err != nil && !errors.Is(err, storepkg.ErrNotFound) {
			return err
		}
		if !errors.Is(err, storepkg.ErrNotFound) {
			scheduler.logger.Warnf("image pull %s already exists, perhaps the user "+
				"manually created it?", imagePull.Name)

			return nil
		}

		if err := txn.SetImagePull(imagePull); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &imagePull, nil
}

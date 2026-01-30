package worker

import (
	"context"
	"errors"

	"github.com/cirruslabs/orchard/internal/worker/tart"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/samber/mo"
)

type ImagePull struct {
	Cancel context.CancelFunc
}

func (worker *Worker) syncPulls(ctx context.Context) error {
	allKeys := mapset.NewSet[string]()

	remotePulls, err := worker.client.ImagePulls().FindForWorker(ctx, worker.name)
	if err != nil {
		return err
	}

	remotePullsIndex := map[string]*v1.ImagePull{}
	for _, remotePull := range remotePulls {
		// A copy is needed to not reference the loop variable
		remotePullCopy := remotePull

		remotePullsIndex[remotePull.Name] = &remotePullCopy
		allKeys.Add(remotePull.Name)
	}

	localPullsIndex := map[string]*ImagePull{}
	for key, localPull := range worker.imagePulls {
		localPullsIndex[key] = localPull
		allKeys.Add(key)
	}

	worker.logger.Infof("syncing %d local image pulls against %d remote image pulls...",
		len(localPullsIndex), len(remotePullsIndex))

	for key := range allKeys.Iter() {
		remotePull := mo.PointerToOption(remotePullsIndex[key])
		localPull := mo.PointerToOption(localPullsIndex[key])

		switch {
		case remotePull.IsSome() && localPull.IsNone():
			// No need to do anything about the image pull in terminal state
			remotePullConditions := remotePull.MustGet().Conditions

			if v1.ConditionIsTrue(remotePullConditions, v1.ConditionTypeCompleted) ||
				v1.ConditionIsTrue(remotePullConditions, v1.ConditionTypeFailed) {
				continue
			}

			// Image pull exists remotely, but not locally,
			// create and start a new local pull
			pullCtx, pullCtxCancel := context.WithCancel(ctx)

			newLocalPull := &ImagePull{
				Cancel: pullCtxCancel,
			}

			go func() {
				defer pullCtxCancel()

				worker.performPull(pullCtx, key, remotePull.MustGet().Image)
			}()

			worker.imagePulls[key] = newLocalPull
		case remotePull.IsNone() && localPull.IsSome():
			// Pull exists locally, but not remotely,
			// terminate and delete the local pull
			localPull.MustGet().Cancel()
			delete(worker.imagePulls, key)
		case remotePull.IsSome() && localPull.IsSome():
			// Terminate local pull when remote pull enters terminal state
			remotePullConditions := remotePull.MustGet().Conditions

			if !v1.ConditionIsTrue(remotePullConditions, v1.ConditionTypeCompleted) &&
				!v1.ConditionIsTrue(remotePullConditions, v1.ConditionTypeFailed) {
				continue
			}

			localPull.MustGet().Cancel()
			delete(worker.imagePulls, key)
		}
	}

	return nil
}

func (worker *Worker) performPull(ctx context.Context, name string, image string) {
	_, err := worker.client.ImagePulls().UpdateState(ctx, v1.ImagePull{
		Meta: v1.Meta{
			Name: name,
		},
		PullState: v1.PullState{
			Conditions: []v1.Condition{
				{
					Type:  v1.ConditionTypeProgressing,
					State: v1.ConditionStateTrue,
				},
			},
		},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		worker.logger.Errorf("failed to update image pull state: %v", err)

		return
	}

	_, _, err = tart.Tart(ctx, worker.logger, "pull", image)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		worker.logger.Errorf("failed to pull image %s: %v", image, err)

		_, err := worker.client.ImagePulls().UpdateState(ctx, v1.ImagePull{
			Meta: v1.Meta{
				Name: name,
			},
			PullState: v1.PullState{
				Conditions: []v1.Condition{
					{
						Type:  v1.ConditionTypeFailed,
						State: v1.ConditionStateTrue,
					},
				},
			},
		})
		if err != nil {
			worker.logger.Errorf("failed to update image pull state: %v", err)
		}

		return
	}

	if _, err := worker.client.ImagePulls().UpdateState(ctx, v1.ImagePull{
		Meta: v1.Meta{
			Name: name,
		},
		PullState: v1.PullState{
			Conditions: []v1.Condition{
				{
					Type:  v1.ConditionTypeCompleted,
					State: v1.ConditionStateTrue,
				},
			},
		},
	}); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		worker.logger.Errorf("failed to update image pull state: %v", err)

		return
	}
}

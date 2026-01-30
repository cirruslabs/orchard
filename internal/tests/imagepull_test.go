package tests

import (
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/imageconstant"
	"github.com/cirruslabs/orchard/internal/tests/devcontroller"
	"github.com/cirruslabs/orchard/internal/tests/wait"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
)

func TestImagePull(t *testing.T) {
	devClient, _, _ := devcontroller.StartIntegrationTestEnvironment(t)

	// Determine the worker name that we'll target
	workers, err := devClient.Workers().List(t.Context())
	require.NoError(t, err)
	require.Len(t, workers, 1)

	// Create an image pull
	imagePullName := "test"

	err = devClient.ImagePulls().Create(t.Context(), &v1.ImagePull{
		Meta: v1.Meta{
			Name: imagePullName,
		},
		Image:  imageconstant.DefaultMacosImage,
		Worker: workers[0].Name,
	})
	require.NoError(t, err)

	// Wait for the image pull to enter terminal state
	var imagePull *v1.ImagePull

	require.True(t, wait.Wait(2*time.Minute, func() bool {
		imagePull, err = devClient.ImagePulls().Get(t.Context(), imagePullName)
		require.NoError(t, err)

		t.Logf("Waiting for the image pull to enter terminal state. Current conditions: %s.",
			v1.ConditionsHumanize(imagePull.Conditions))

		return v1.ConditionIsTrue(imagePull.Conditions, v1.ConditionTypeCompleted) ||
			v1.ConditionIsTrue(imagePull.Conditions, v1.ConditionTypeFailed)
	}), "failed to wait for image pull to enter terminal state")

	// Ensure that image pull succeeded
	require.True(t, v1.ConditionIsTrue(imagePull.Conditions, v1.ConditionTypeCompleted))
}

func TestImagePullJob(t *testing.T) {
	devClient, _, _ := devcontroller.StartIntegrationTestEnvironment(t)

	// Determine the worker name that we'll target
	workers, err := devClient.Workers().List(t.Context())
	require.NoError(t, err)
	require.Len(t, workers, 1)

	// Create an image pull job
	imagePullJobName := "test"

	err = devClient.ImagePullJobs().Create(t.Context(), &v1.ImagePullJob{
		Meta: v1.Meta{
			Name: imagePullJobName,
		},
		Image: imageconstant.DefaultMacosImage,
	})
	require.NoError(t, err)

	// Wait for the image pull job to be completed
	var imagePullJob *v1.ImagePullJob

	require.True(t, wait.Wait(2*time.Minute, func() bool {
		imagePullJob, err = devClient.ImagePullJobs().Get(t.Context(), imagePullJobName)
		require.NoError(t, err)

		t.Logf("Waiting for the image pull job to enter terminal state. Current conditions: %s.",
			v1.ConditionsHumanize(imagePullJob.Conditions))

		return v1.ConditionIsTrue(imagePullJob.Conditions, v1.ConditionTypeCompleted) ||
			v1.ConditionIsTrue(imagePullJob.Conditions, v1.ConditionTypeFailed)
	}), "failed to wait for image pull to enter terminal state")

	// Ensure that image pull had succeeded
	require.Equal(t, []v1.Condition{
		{
			Type:  v1.ConditionTypeCompleted,
			State: v1.ConditionStateTrue,
		},
	}, imagePullJob.Conditions)
	require.EqualValues(t, 0, imagePullJob.Progressing)
	require.EqualValues(t, 1, imagePullJob.Succeeded)
	require.EqualValues(t, 0, imagePullJob.Failed)
	require.EqualValues(t, 1, imagePullJob.Total)
}

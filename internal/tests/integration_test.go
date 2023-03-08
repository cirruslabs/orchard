package tests

import (
	"context"
	"github.com/cirruslabs/orchard/internal/command/dev"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
	"time"
)

func TestSingleVM(t *testing.T) {
	devClient := StartIntegrationTestEnvironment(t)

	workers, err := devClient.Workers().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(workers))
}

func StartIntegrationTestEnvironment(t *testing.T) *client.Client {
	t.Setenv("ORCHARD_HOME", t.TempDir())
	devController, devWorker, err := dev.CreateDevControllerAndWorker(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	devContext, cancelDevFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelDevFunc)
	go func() {
		err := devController.Run(devContext)
		if err != nil && err != context.Canceled && err != http.ErrServerClosed {
			t.Errorf("dev controller failed: %v", err)
		}
	}()
	go func() {
		err := devWorker.Run(devContext)
		if err != nil && err != context.Canceled {
			t.Errorf("dev worker failed: %v", err)
		}
	}()

	// todo: find a better way to wait for the controller to start and a worker to register
	time.Sleep(5 * time.Second)

	devClient, err := client.New()
	if err != nil {
		t.Fatal(err)
	}
	return devClient
}

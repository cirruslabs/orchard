package tests_test

import (
	"context"
	"errors"
	"github.com/cirruslabs/orchard/internal/command/dev"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
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
	err = devClient.VMs().Create(context.Background(), &v1.VM{
		Meta: v1.Meta{
			Name: "test-vm",
		},
		Image:    "ghcr.io/cirruslabs/macos-ventura-base:latest",
		CPU:      4,
		Memory:   8 * 1024,
		Softnet:  false,
		Headless: true,
		Status:   v1.VMStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), "test-vm")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Waiting for the VM to start. Current status: %s", vm.Status)
		return vm.Status == v1.VMStatusRunning
	}), "failed to start a VM")
	stoppingVM, err := devClient.VMs().Stop(context.Background(), "test-vm")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, v1.VMStatusStopping, stoppingVM.Status)
	assert.True(t, Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(context.Background(), "test-vm")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Waiting for the VM to stop. Current status: %s", vm.Status)
		return vm.Status == v1.VMStatusStopped
	}), "failed to stop a VM")
}

func Wait(duration time.Duration, condition func() bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	for {
		if condition() {
			// all good
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(5 * time.Second):
			// try again
			continue
		}
	}
}

func StartIntegrationTestEnvironment(t *testing.T) *client.Client {
	t.Setenv("ORCHARD_HOME", t.TempDir())
	devController, devWorker, err := dev.CreateDevControllerAndWorker(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = devWorker.DeleteAllVMs()
	})
	devContext, cancelDevFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelDevFunc)
	go func() {
		err := devController.Run(devContext)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("dev controller failed: %v", err)
		}
	}()
	go func() {
		err := devWorker.Run(devContext)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("dev worker failed: %v", err)
		}
	}()

	time.Sleep(5 * time.Second)

	devClient, err := client.New()
	if err != nil {
		t.Fatal(err)
	}
	return devClient
}

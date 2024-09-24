package lifecycle

import (
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"go.uber.org/zap"
	"time"
)

func Report(vm *v1.VM, message string, logger *zap.SugaredLogger) {
	args := []interface{}{
		"component", "lifecycle",
		"vm_uid", vm.UID,
		"vm_name", vm.Name,
		"vm_restart_count", vm.RestartCount,
		"vm_image", vm.Image,
		"vm_status", vm.Status,
	}

	if vm.ScheduledAt.IsZero() {
		// VM was never scheduled
		args = append(args, "vm_scheduling_duration", time.Since(vm.CreatedAt))
	} else {
		args = append(args, "vm_scheduling_duration", vm.ScheduledAt.Sub(vm.CreatedAt))
	}

	if vm.StartedAt.IsZero() {
		// VM was never started
		args = append(args, "vm_run_duration", time.Duration(0))
	} else {
		args = append(args, "vm_run_duration", time.Since(vm.StartedAt))
	}

	logger.With(args...).Info(message)
}

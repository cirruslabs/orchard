package worker

import (
	"fmt"

	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/samber/mo"
)

type Action string

const (
	ActionIgnore         Action = "ignore"
	ActionCreate         Action = "create"
	ActionMonitorPending Action = "monitor-pending"
	ActionReportRunning  Action = "report-running"
	ActionMonitorRunning Action = "monitor-running"
	ActionStop           Action = "stop"
	ActionFail           Action = "fail"
	ActionLostTrack      Action = "lost-track"
	ActionImpossible     Action = "impossible"
	ActionDelete         Action = "delete"
)

/**
 * Mapping of actions on how to act for a remote VM state based on a local VM state.
 */
var transitions = map[mo.Option[v1.VMStatus]]map[mo.Option[v1.VMStatus]]Action{
	mo.None[v1.VMStatus](): {
		mo.None[v1.VMStatus]():      ActionIgnore,
		mo.Some(v1.VMStatusPending): ActionDelete,
		mo.Some(v1.VMStatusRunning): ActionDelete,
		mo.Some(v1.VMStatusFailed):  ActionDelete,
	},
	mo.Some(v1.VMStatusPending): {
		mo.None[v1.VMStatus]():      ActionCreate,
		mo.Some(v1.VMStatusPending): ActionMonitorPending,
		mo.Some(v1.VMStatusRunning): ActionReportRunning, // example, remote state is pending and local state is running
		mo.Some(v1.VMStatusFailed):  ActionFail,
	},
	mo.Some(v1.VMStatusRunning): {
		mo.None[v1.VMStatus]():      ActionLostTrack,
		mo.Some(v1.VMStatusPending): ActionImpossible,
		mo.Some(v1.VMStatusRunning): ActionMonitorRunning,
		mo.Some(v1.VMStatusFailed):  ActionFail,
	},
	mo.Some(v1.VMStatusFailed): {
		mo.None[v1.VMStatus]():      ActionIgnore,
		mo.Some(v1.VMStatusPending): ActionStop,
		mo.Some(v1.VMStatusRunning): ActionStop,
		mo.Some(v1.VMStatusFailed):  ActionIgnore,
	},
}

func optionToString[T any](option mo.Option[T]) string {
	if option.IsNone() {
		return "None"
	}

	return fmt.Sprintf("Some(%v)", option.MustGet())
}

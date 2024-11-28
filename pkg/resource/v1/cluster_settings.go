package v1

import "fmt"

type SchedulerProfile string

const (
	SchedulerProfileOptimizeUtilization SchedulerProfile = "optimize-utilization"
	SchedulerProfileDistributeLoad      SchedulerProfile = "distribute-load"
)

type ClusterSettings struct {
	HostDirPolicies  []HostDirPolicy  `json:"hostDirPolicies,omitempty"`
	SchedulerProfile SchedulerProfile `json:"schedulerProfile,omitempty"`
}

func NewSchedulerProfile(value string) (SchedulerProfile, error) {
	switch value {
	case string(SchedulerProfileOptimizeUtilization):
		return SchedulerProfileOptimizeUtilization, nil
	case string(SchedulerProfileDistributeLoad):
		return SchedulerProfileDistributeLoad, nil
	default:
		return "", fmt.Errorf("unsupported scheduler profile: %q", value)
	}
}

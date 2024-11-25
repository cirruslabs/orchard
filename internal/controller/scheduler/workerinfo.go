package scheduler

import v1 "github.com/cirruslabs/orchard/pkg/resource/v1"

type WorkerInfo struct {
	ResourcesUsed v1.Resources
	NumRunningVMs int
}

type WorkerInfos map[string]WorkerInfo

func (workerInfos WorkerInfos) AddVM(name string, resourcesUsed v1.Resources) {
	workerInfo, ok := workerInfos[name]
	if !ok {
		workerInfo = WorkerInfo{
			ResourcesUsed: v1.Resources{},
		}
	}

	workerInfo.ResourcesUsed.Add(resourcesUsed)
	workerInfo.NumRunningVMs++

	workerInfos[name] = workerInfo
}

func (workerInfos WorkerInfos) Get(name string) WorkerInfo {
	workerInfo, ok := workerInfos[name]
	if !ok {
		workerInfo = WorkerInfo{
			ResourcesUsed: v1.Resources{},
		}

		workerInfos[name] = workerInfo
	}

	return workerInfo
}

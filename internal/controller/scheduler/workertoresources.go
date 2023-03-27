package scheduler

import v1 "github.com/cirruslabs/orchard/pkg/resource/v1"

type WorkerToResources map[string]v1.Resources

func (workerToResources WorkerToResources) Add(name string, other v1.Resources) {
	workerResources, ok := workerToResources[name]
	if !ok {
		workerResources = make(v1.Resources)
	}

	workerResources.Add(other)

	workerToResources[name] = workerResources
}

func (workerToResources WorkerToResources) Get(name string) v1.Resources {
	workerResources, ok := workerToResources[name]
	if !ok {
		workerResources = make(v1.Resources)
		workerToResources[name] = workerResources
	}

	return workerResources
}

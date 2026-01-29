package store

import v1 "github.com/cirruslabs/orchard/pkg/resource/v1"

type ListInput struct {
	Filters []v1.Filter
}

type ListOption func(listInput *ListInput)

func WithListFilters(filters ...v1.Filter) ListOption {
	return func(listInput *ListInput) {
		listInput.Filters = filters
	}
}

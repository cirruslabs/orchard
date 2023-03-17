package concurrentmap

import (
	"sync"
)

type ConcurrentMap[T any] struct {
	nonConcurrentMap map[any]T
	mtx              sync.Mutex
}

func NewConcurrentMap[T any]() *ConcurrentMap[T] {
	return &ConcurrentMap[T]{
		nonConcurrentMap: map[any]T{},
	}
}

func (cmap *ConcurrentMap[T]) Load(key any) (T, bool) {
	cmap.mtx.Lock()
	defer cmap.mtx.Unlock()

	result, ok := cmap.nonConcurrentMap[key]

	return result, ok
}

func (cmap *ConcurrentMap[T]) Store(id any, value T) {
	cmap.mtx.Lock()
	defer cmap.mtx.Unlock()

	cmap.nonConcurrentMap[id] = value
}

func (cmap *ConcurrentMap[T]) Delete(key any) {
	cmap.mtx.Lock()
	defer cmap.mtx.Unlock()

	delete(cmap.nonConcurrentMap, key)
}

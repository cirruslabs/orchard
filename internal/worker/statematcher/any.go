package statematcher

type anyOperator[T Element] struct{}

func (anyOperator[T]) Matches(_ *T) bool {
	return true
}

func Any[T Element]() State[T] {
	return anyOperator[T]{}
}

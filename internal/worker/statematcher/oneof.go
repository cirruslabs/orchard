package statematcher

type oneOfOperator[T Element] struct {
	operators []State[T]
}

func (op oneOfOperator[T]) Matches(other *T) bool {
	for _, operator := range op.operators {
		if operator.Matches(other) {
			return true
		}
	}

	return false
}

func OneOf[T Element](operators ...State[T]) State[T] {
	return &oneOfOperator[T]{operators: operators}
}

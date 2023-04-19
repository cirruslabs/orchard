package statematcher

type exactOperator[T Element] struct {
	element T
}

func (op exactOperator[T]) Matches(otherElement *T) bool {
	if otherElement == nil {
		return false
	}

	return op.element == *otherElement
}

func Exact[T Element](element T) State[T] {
	return &exactOperator[T]{element: element}
}

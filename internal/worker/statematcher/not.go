package statematcher

type notOperator[T Element] struct {
	element T
}

func (op notOperator[T]) Matches(otherElement *T) bool {
	if otherElement == nil {
		return false
	}

	return op.element != *otherElement
}

func Not[T Element](element T) State[T] {
	return &notOperator[T]{element: element}
}

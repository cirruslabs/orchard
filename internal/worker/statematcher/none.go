package statematcher

type noneOperator[T Element] struct{}

func (noneOperator[T]) Matches(otherElement *T) bool {
	return otherElement == nil
}

func None[T Element]() State[T] {
	return &noneOperator[T]{}
}

package statematcher

type someOperator[T Element] struct{}

func (someOperator[T]) Matches(otherElement *T) bool {
	return otherElement != nil
}

func Some[T Element]() State[T] {
	return someOperator[T]{}
}

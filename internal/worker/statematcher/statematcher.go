package statematcher

type Element interface {
	comparable
}

type State[T Element] interface {
	Matches(other *T) bool
}

type Rule[R Element, L Element] struct {
	RemoteState State[R]
	LocalState  State[L]
	Action      func()
}

func Match[R Element, L Element](rules []Rule[R, L], remoteState *R, localState *L) *Rule[R, L] {
	for _, rule := range rules {
		rule := rule

		if rule.RemoteState.Matches(remoteState) && rule.LocalState.Matches(localState) {
			return &rule
		}
	}

	return nil
}

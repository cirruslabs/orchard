package statematcher_test

import (
	"github.com/cirruslabs/orchard/internal/worker/statematcher"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestAny makes sure Any operator matches anything.
func TestAny(t *testing.T) {
	require.True(t, statematcher.Any[int]().Matches(nil))

	zero := 0
	require.True(t, statematcher.Any[int]().Matches(&zero))

	one := 1
	require.True(t, statematcher.Any[int]().Matches(&one))
}

// TestExact makes sure Exact operator matches only specific values.
func TestExact(t *testing.T) {
	require.False(t, statematcher.Exact[int](42).Matches(nil))
	zero := 0
	require.False(t, statematcher.Exact[int](42).Matches(&zero))
	one := 1
	require.False(t, statematcher.Exact[int](42).Matches(&one))

	fortyTwo := 42
	require.True(t, statematcher.Exact[int](42).Matches(&fortyTwo))
}

// TestOneOf makes sure OneOf operator matches at least one of its compound operators.
func TestOneOf(t *testing.T) {
	require.False(t, statematcher.OneOf[int](
	/* nothing */
	).Matches(nil))
	require.False(t, statematcher.OneOf[int](
		statematcher.Exact(0),
		statematcher.Exact(1),
		statematcher.Exact(42),
	).Matches(nil))

	fortyTwo := 42
	require.False(t, statematcher.OneOf[int]().Matches(&fortyTwo))
	require.False(t, statematcher.OneOf[int](
		statematcher.Exact(0),
		statematcher.Exact(1),
	).Matches(&fortyTwo))
	require.True(t, statematcher.OneOf[int](
		statematcher.Exact(0),
		statematcher.Exact(1),
		statematcher.Exact(42),
	).Matches(&fortyTwo))
}

// TestSome makes sure Some operator matches any non-nil value.
func TestSome(t *testing.T) {
	require.False(t, statematcher.Some[int]().Matches(nil))

	zero := 0
	require.True(t, statematcher.Some[int]().Matches(&zero))
	one := 1
	require.True(t, statematcher.Some[int]().Matches(&one))
}

// TestStateMatcherFirstMatchIsReturned makes sure statematcher.Match() always returns a first match.
func TestStateMatcherFirstMatchIsReturned(t *testing.T) {
	// Set-up
	expectedRule := statematcher.Rule[int, string]{
		RemoteState: statematcher.Exact(1),
		LocalState:  statematcher.Exact("magic"),

		// Note: non-nil action
		Action: func() {},
	}

	unexpectedRule := statematcher.Rule[int, string]{
		RemoteState: statematcher.Exact(1),
		LocalState:  statematcher.Exact("magic"),

		// Note: nil action
		Action: nil,
	}

	rules := []statematcher.Rule[int, string]{
		expectedRule,
		unexpectedRule,
	}

	// Ensure that the first match is returned
	expectedRuleRemoteState := 1
	expectedRuleLocalState := "magic"

	match := statematcher.Match(rules, &expectedRuleRemoteState, &expectedRuleLocalState)
	require.NotNil(t, match)
	require.NotNil(t, match.Action)

	// Ensure that the first match is returned (reversed rules)
	match = statematcher.Match(lo.Reverse(rules), &expectedRuleRemoteState, &expectedRuleLocalState)
	require.NotNil(t, match)
	require.Nil(t, match.Action)
}

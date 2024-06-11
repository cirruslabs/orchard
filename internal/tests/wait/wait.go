package wait

import (
	"context"
	"time"
)

func Wait(duration time.Duration, predicate func() bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	for {
		if predicate() {
			return true
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(1 * time.Second):
			continue
		}
	}
}

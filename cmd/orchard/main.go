package main

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/internal/command"
	"os"
	"os/signal"
)

func main() {
	// Set up a signal-interruptible context
	ctx, cancel := context.WithCancel(context.Background())

	interruptCh := make(chan os.Signal, 1)
	signal.Notify(interruptCh, os.Interrupt)

	go func() {
		select {
		case <-interruptCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Run the command
	if err := command.NewRootCmd().ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}

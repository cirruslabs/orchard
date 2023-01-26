package main

import (
	"context"
	"github.com/cirruslabs/orchard/internal/command"
	"log"
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
		log.Fatal(err)
	}
}

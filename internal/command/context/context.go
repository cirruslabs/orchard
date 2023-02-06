package context

import (
	"github.com/spf13/cobra"
)

var contextName string

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "context",
		Short: "Manage client/worker → controller contexts",
	}

	command.AddCommand(
		newCreateCommand(),
		newListCommand(),
		newDefaultCommand(),
		newDeleteCommand(),
	)

	return command
}

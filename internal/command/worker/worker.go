//go:build unix

package worker

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "worker",
		Short: "Run a worker on the local machine",
	}

	command.AddCommand(newRunCommand())

	return command
}

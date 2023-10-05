package vnc

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vnc",
		Short: "Open VNC session with the resource",
	}

	command.AddCommand(newVNCVMCommand(), newVNCWorkerCommand())

	return command
}

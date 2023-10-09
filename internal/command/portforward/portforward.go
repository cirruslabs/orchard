package portforward

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "port-forward",
		Short: "Forward TCP port to the resources",
	}

	command.AddCommand(newPortForwardVMCommand(), newPortForwardWorkerCommand())

	return command
}

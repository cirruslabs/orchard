package ssh

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "ssh",
		Short: "SSH into resources",
	}

	command.AddCommand(newSSHVMCommand())

	return command
}

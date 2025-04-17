package controller

import (
	"github.com/spf13/cobra"
)

var dataDirPath string

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "controller",
		Short: "Run a controller on the local machine",
	}

	command.AddCommand(newRunCommand())

	command.PersistentFlags().StringVar(&dataDirPath, "data-dir", "",
		"path to the data controller's directory (defaults to $HOME/.orchard/controller)")

	return command
}

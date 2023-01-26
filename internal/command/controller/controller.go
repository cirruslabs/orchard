package controller

import (
	"github.com/cirruslabs/orchard/internal/orchardhome"
	"github.com/spf13/cobra"
	"log"
	"path/filepath"
)

var dataDir string

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "controller",
		Short: "Initialize and run a controller on the local machine",
	}

	command.AddCommand(newRunCommand())

	orchardHome, err := orchardhome.Path()
	if err != nil {
		log.Fatal(err)
	}

	command.PersistentFlags().StringVar(&dataDir, "data-dir", filepath.Join(orchardHome, "controller"),
		"path to the data directory")

	return command
}

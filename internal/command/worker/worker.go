package worker

import (
	"github.com/cirruslabs/orchard/internal/orchardhome"
	"github.com/spf13/cobra"
	"log"
	"path/filepath"
)

var dataDir string

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "worker",
		Short: "Run a worker on the local machine",
	}

	command.AddCommand(newRunCommand())

	orchardHome, err := orchardhome.Path()
	if err != nil {
		log.Fatal(err)
	}

	command.PersistentFlags().StringVar(&dataDir, "data-dir", filepath.Join(orchardHome, "worker"),
		"path to the data directory")

	return command
}

//go:build unix

package worker

import (
	"github.com/cirruslabs/orchard/internal/orchardhome"
	"github.com/spf13/cobra"
	"log"
	"path/filepath"
)

var dataDirPath string

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

	command.PersistentFlags().StringVar(&dataDirPath, "data-dir", filepath.Join(orchardHome, "worker"),
		"path to the worker's data directory")

	return command
}

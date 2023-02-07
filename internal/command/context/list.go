package context

import (
	"fmt"
	"github.com/cirruslabs/orchard/internal/config"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func newListCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "list",
		Short: "List contexts",
		RunE:  runList,
	}

	return command
}

func runList(cmd *cobra.Command, args []string) error {
	configHandle, err := config.NewHandle()
	if err != nil {
		return err
	}

	config, err := configHandle.Config()
	if err != nil {
		return err
	}

	table := uitable.New()

	table.AddRow("Name", "URL")

	for name, context := range config.Contexts {
		table.AddRow(name, context.URL)
	}

	fmt.Println(table)

	return nil
}

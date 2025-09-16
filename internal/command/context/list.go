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
	table.Wrap = true

	table.AddRow("Name", "URL", "Default")

	for name, context := range config.Contexts {
		var defaultMark string
		if name == config.DefaultContext {
			defaultMark = "*"
		}

		table.AddRow(name, context.URL, defaultMark)
	}

	fmt.Println(table)

	return nil
}

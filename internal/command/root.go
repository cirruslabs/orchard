package command

import (
	"github.com/cirruslabs/orchard/internal/command/context"
	"github.com/cirruslabs/orchard/internal/command/controller"
	"github.com/cirruslabs/orchard/internal/command/create"
	deletepkg "github.com/cirruslabs/orchard/internal/command/deletecmd"
	"github.com/cirruslabs/orchard/internal/command/dev"
	"github.com/cirruslabs/orchard/internal/command/get"
	"github.com/cirruslabs/orchard/internal/command/list"
	"github.com/cirruslabs/orchard/internal/command/worker"
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	command := &cobra.Command{
		Use:           "orchard",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	addGroupedCommands(command, "Working With Resources:",
		create.NewCommand(),
		get.NewCommand(),
		list.NewCommand(),
		deletepkg.NewCommand(),
	)

	addGroupedCommands(command, "Administrative Tasks:",
		context.NewCommand(),
		controller.NewCommand(),
		worker.NewCommand(),
		dev.NewCommand(),
	)

	return command
}

func addGroupedCommands(parent *cobra.Command, title string, commands ...*cobra.Command) {
	group := &cobra.Group{
		ID:    title,
		Title: title,
	}
	parent.AddGroup(group)

	for _, command := range commands {
		command.GroupID = group.ID
		parent.AddCommand(command)
	}
}

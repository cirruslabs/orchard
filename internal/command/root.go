package command

import (
	"github.com/cirruslabs/orchard/internal/command/context"
	"github.com/cirruslabs/orchard/internal/command/controller"
	"github.com/cirruslabs/orchard/internal/command/create"
	deletepkg "github.com/cirruslabs/orchard/internal/command/deletecmd"
	"github.com/cirruslabs/orchard/internal/command/dev"
	"github.com/cirruslabs/orchard/internal/command/get"
	"github.com/cirruslabs/orchard/internal/command/list"
	"github.com/cirruslabs/orchard/internal/command/logs"
	"github.com/cirruslabs/orchard/internal/command/pause"
	"github.com/cirruslabs/orchard/internal/command/portforward"
	"github.com/cirruslabs/orchard/internal/command/resume"
	"github.com/cirruslabs/orchard/internal/command/set"
	"github.com/cirruslabs/orchard/internal/command/ssh"
	"github.com/cirruslabs/orchard/internal/command/vnc"
	"github.com/cirruslabs/orchard/internal/command/worker"
	"github.com/cirruslabs/orchard/internal/opentelemetry"
	"github.com/cirruslabs/orchard/internal/version"
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	command := &cobra.Command{
		Use:           "orchard",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.FullVersion,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Configure OpenTelemetry
			if err := opentelemetry.Configure(cmd.Context()); err != nil {
				return err
			}

			return nil
		},
	}

	addGroupedCommands(command, "Working With Resources:",
		create.NewCommand(),
		deletepkg.NewCommand(),
		get.NewCommand(),
		list.NewCommand(),
		logs.NewCommand(),
		pause.NewCommand(),
		portforward.NewCommand(),
		resume.NewCommand(),
		set.NewCommand(),
		ssh.NewCommand(),
		vnc.NewCommand(),
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

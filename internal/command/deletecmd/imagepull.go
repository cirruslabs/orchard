package deletecmd

import (
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

func newDeleteImagePullCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "imagepull NAME",
		Short: "Delete an image pull",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeleteImagePull,
	}
}

func runDeleteImagePull(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	return client.ImagePulls().Delete(cmd.Context(), name)
}

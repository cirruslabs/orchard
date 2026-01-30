package deletecmd

import (
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

func newDeleteImagePullJobCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "imagepulljob NAME",
		Short: "Delete an image pull job",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeleteImagePullJob,
	}
}

func runDeleteImagePullJob(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	return client.ImagePullJobs().Delete(cmd.Context(), name)
}

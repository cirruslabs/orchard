package create

import (
	"fmt"
	"os"

	"github.com/cirruslabs/orchard/internal/simplename"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/spf13/cobra"
)

func newCreateImagePullJob() *cobra.Command {
	command := &cobra.Command{
		Use:   "imagepulljob NAME",
		Short: "Create an image pull job",
		RunE:  runCreateImagePullJob,
		Args:  cobra.ExactArgs(1),
	}

	command.Flags().StringVar(&image, "image", "",
		"image to pull")
	command.Flags().StringToStringVar(&labels, "labels", map[string]string{},
		"labels required by this image pull job")

	return command
}

func runCreateImagePullJob(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Issue a warning if the name used will be invalid in the future
	if err := simplename.ValidateNext(name); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "WARNING: %v\n", err)
	}

	// Validate command-line arguments
	if image == "" {
		return fmt.Errorf("please specify an \"--image\" to pull")
	}

	imagePullJob := &v1.ImagePullJob{
		Meta: v1.Meta{
			Name: name,
		},
		Image:  image,
		Labels: labels,
	}

	client, err := client.New()
	if err != nil {
		return err
	}

	return client.ImagePullJobs().Create(cmd.Context(), imagePullJob)
}

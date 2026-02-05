package get

import (
	"fmt"
	"time"

	"github.com/cirruslabs/orchard/internal/vmtempauth"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

var vmAccessTokenTTL time.Duration

func newGetVMAccessTokenCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vm-access-token VM_NAME",
		Short: "Issue a temporary VM access token",
		RunE:  runGetVMAccessToken,
		Args:  cobra.ExactArgs(1),
	}

	command.Flags().DurationVar(&vmAccessTokenTTL, "ttl", vmtempauth.DefaultTTL,
		fmt.Sprintf("token TTL (default: %s, max: %s)", vmtempauth.DefaultTTL, vmtempauth.MaxTTL))

	return command
}

func runGetVMAccessToken(cmd *cobra.Command, args []string) error {
	name := args[0]

	if vmAccessTokenTTL <= 0 {
		return fmt.Errorf("%w: --ttl must be greater than 0", ErrGetFailed)
	}
	if vmAccessTokenTTL > vmtempauth.MaxTTL {
		return fmt.Errorf("%w: --ttl cannot exceed %s", ErrGetFailed, vmtempauth.MaxTTL)
	}

	ttlSeconds := uint64(vmAccessTokenTTL / time.Second)
	if ttlSeconds == 0 {
		return fmt.Errorf("%w: --ttl is too small", ErrGetFailed)
	}

	apiClient, err := client.New()
	if err != nil {
		return err
	}

	response, err := apiClient.VMs().IssueAccessToken(cmd.Context(), name, client.IssueAccessTokenOptions{
		TTLSeconds: &ttlSeconds,
	})
	if err != nil {
		return err
	}

	fmt.Println(response.Token)

	return nil
}

//go:build !unix

package worker

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	return nil
}

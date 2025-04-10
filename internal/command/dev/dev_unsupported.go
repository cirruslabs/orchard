//go:build !unix

package dev

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	return nil
}

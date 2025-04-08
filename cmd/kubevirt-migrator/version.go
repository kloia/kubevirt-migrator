package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kloia/kubevirt-migrator/internal/version"
)

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			fmt.Println(info.String())
		},
	}

	return cmd
}

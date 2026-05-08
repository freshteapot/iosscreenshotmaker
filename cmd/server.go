package cmd

import (
	"github.com/freshteapot/iosscreenshotmaker/internal/server"
	"github.com/spf13/cobra"
)

func newServerCommand() *cobra.Command {
	options := server.Options{}

	command := &cobra.Command{
		Use:   "server <www-dir>",
		Short: "Serve a static www directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Directory = args[0]
			return server.Run(options)
		},
	}

	command.Flags().StringVar(&options.Address, "addr", "127.0.0.1:8080", "listen address")

	return command
}

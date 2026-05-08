package cmd

import (
	"github.com/freshteapot/iosscreenshotmaker/internal/capture"
	"github.com/spf13/cobra"
)

func newCreateAppStoreScreenshotCommand() *cobra.Command {
	options := capture.Options{
		Device:      "iphone",
		Orientation: "portrait",
	}

	command := &cobra.Command{
		Use:   "create-app-store-screenshot",
		Short: "Capture the App Store screenshot HTML output with Rod",
		RunE: func(cmd *cobra.Command, args []string) error {
			return capture.Run(options)
		},
	}

	command.Flags().StringVar(&options.ChromePath, "chrome", capture.DefaultChromePath, "path to the local Chrome executable")
	command.Flags().StringVar(&options.ConfigName, "configName", "", "optional configName query parameter to pass to the page")
	command.Flags().StringVar(&options.BaseURL, "url", capture.DefaultBaseTargetURL, "base URL for the screenshot page")
	command.Flags().StringVar(&options.Device, "device", options.Device, "device preset: iphone or ipad")
	command.Flags().StringVar(&options.Orientation, "orientation", options.Orientation, "orientation preset: portrait or landscape")
	command.Flags().StringVar(&options.PlanPath, "plan", "", "path to an ASC screenshot plan JSON file")
	command.Flags().StringVar(&options.InputPath, "input", "", "path to a raw screenshot to expose under the static server and inject into the config")
	command.Flags().StringVar(&options.ServerDir, "server-dir", "www", "directory served by the local screenshot server where injected input assets should be copied")

	return command
}

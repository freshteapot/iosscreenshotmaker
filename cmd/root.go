package cmd

import "github.com/spf13/cobra"

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "iosscreenshotmaker",
		Short: "Screenshot tooling",
	}

	rootCmd.AddCommand(
		newGenerateFrameMaskCommand(),
		newFrameScreenshotCommand(),
		newFrameVideoCommand(),
		newCreateAppStoreScreenshotCommand(),
		newServerCommand(),
	)

	return rootCmd
}

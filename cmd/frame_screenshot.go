package cmd

import (
	"fmt"

	"github.com/freshteapot/iosscreenshotmaker/internal/frame"
	"github.com/spf13/cobra"
)

func newFrameScreenshotCommand() *cobra.Command {
	options := frame.Options{}

	command := &cobra.Command{
		Use:   "frame-screenshot",
		Short: "Composite a screenshot into a device frame",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := frame.Run(options); err != nil {
				return err
			}

			fmt.Printf("Wrote %s\n", options.OutputPath)
			return nil
		},
	}

	command.Flags().StringVar(&options.FramePath, "frame", "iphone-mask.png", "path to the device frame PNG")
	command.Flags().StringVar(&options.InputPath, "input", "screenshot.png", "path to the screenshot image")
	command.Flags().StringVar(&options.OutputPath, "output", "out.png", "path to the output PNG")

	return command
}

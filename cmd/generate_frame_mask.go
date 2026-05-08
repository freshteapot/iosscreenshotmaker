package cmd

import (
	"fmt"

	"github.com/freshteapot/iosscreenshotmaker/internal/frame"
	"github.com/spf13/cobra"
)

func newGenerateFrameMaskCommand() *cobra.Command {
	var framePath string
	var outputPath string

	command := &cobra.Command{
		Use:   "generate-frame-mask",
		Short: "Generate a phone silhouette mask PNG from a frame PNG",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := frame.GenerateMask(framePath, outputPath); err != nil {
				return err
			}

			fmt.Printf("Wrote %s\n", outputPath)
			return nil
		},
	}

	command.Flags().StringVar(&framePath, "frame", "", "path to the device frame PNG")
	command.Flags().StringVar(&outputPath, "output", "", "path to the output grayscale mask PNG")
	_ = command.MarkFlagRequired("frame")
	_ = command.MarkFlagRequired("output")

	return command
}

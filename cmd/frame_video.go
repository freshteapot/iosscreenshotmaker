package cmd

import (
	"github.com/freshteapot/iosscreenshotmaker/internal/frame"
	"github.com/spf13/cobra"
)

func newFrameVideoCommand() *cobra.Command {
	options := frame.VideoOptions{}

	command := &cobra.Command{
		Use:   "frame-video",
		Short: "Composite a simulator recording into a framed social video",
		Long: `

magick -size 1080x1920 xc:'#ffff00' screenshots/app/cta.png


go run . frame-video \
  --background "ffff00" \
  --frame "frames/iPhone Air - Cloud White - Portrait.png" \
  --input "example/frame-video/sample-video.mp4" \
  --output example/frame-video/output.mp4 \
  --cta example/frame-video/cta.png \
  --cta-fade 1 \
  --cta-length 2

		`,
		RunE: func(cmd *cobra.Command, args []string) error {

			return frame.RunVideo(options)
		},
	}

	command.Flags().StringVar(&options.FramePath, "frame", "", "path to the device frame PNG")
	command.Flags().StringVar(&options.InputPath, "input", "", "path to the input video")
	command.Flags().StringVar(&options.OutputPath, "output", "", "path to the output MP4")
	command.Flags().StringVar(&options.MaskPath, "mask", "", "optional path to a precomputed silhouette mask PNG")
	command.Flags().StringVar(&options.BackgroundColor, "background", "", "background color in 0xRRGGBB format")
	command.Flags().IntVar(&options.FPS, "fps", 0, "output frames per second")
	command.Flags().StringVar(&options.Bitrate, "bitrate", "", "video bitrate for h264_videotoolbox")
	command.Flags().StringVar(&options.CTAPath, "cta", "", "optional CTA image to append at the end")
	command.Flags().Float64Var(&options.CTAFadeSeconds, "cta-fade", 0, "CTA fade duration in seconds")
	command.Flags().Float64Var(&options.CTALengthSeconds, "cta-length", 0, "CTA hold duration after the fade in seconds")
	command.Flags().BoolVar(&options.KeepTemp, "keep-temp", false, "keep generated temporary assets for debugging")
	_ = command.MarkFlagRequired("frame")
	_ = command.MarkFlagRequired("input")
	_ = command.MarkFlagRequired("output")

	return command
}

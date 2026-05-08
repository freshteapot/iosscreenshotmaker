package frame

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultVideoBackground = "0xf7f6f2"
	defaultVideoFPS        = 30
	defaultVideoBitrate    = "8M"
	socialWidth            = 1080
	socialHeight           = 1920
)

type VideoOptions struct {
	FramePath        string
	InputPath        string
	OutputPath       string
	MaskPath         string
	BackgroundColor  string
	FPS              int
	Bitrate          string
	CTAPath          string
	CTAFadeSeconds   float64
	CTALengthSeconds float64
	KeepTemp         bool
}

type _frameLayout struct {
	frameImage image.Image
	screen     screenRegion
	maskImage  *image.Gray
}

type _commandRunner interface {
	Run(name string, args []string) ([]byte, []byte, error)
}

type _execCommandRunner struct{}

func (_execCommandRunner) Run(name string, args []string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	fmt.Printf("running: %s %s\n", cmd.Path, strings.Join(cmd.Args[1:], " "))

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func RunVideo(options VideoOptions) error {
	return runVideo(options, _execCommandRunner{})
}

func runVideo(options VideoOptions, runner _commandRunner) error {
	resolvedOptions := options
	if strings.TrimSpace(resolvedOptions.BackgroundColor) == "" {
		resolvedOptions.BackgroundColor = defaultVideoBackground
	}
	if resolvedOptions.FPS <= 0 {
		resolvedOptions.FPS = defaultVideoFPS
	}
	if strings.TrimSpace(resolvedOptions.Bitrate) == "" {
		resolvedOptions.Bitrate = defaultVideoBitrate
	}
	if err := parseHexColor(resolvedOptions.BackgroundColor); err != nil {
		return err
	}
	if err := validateCTAOptions(resolvedOptions); err != nil {
		return err
	}

	frameLayout, err := loadFrameLayout(resolvedOptions.FramePath)
	if err != nil {
		return err
	}

	maskPath, cleanup, generatedMask, err := resolveMaskPath(resolvedOptions, frameLayout)
	if err != nil {
		return err
	}
	if cleanup != nil && !resolvedOptions.KeepTemp {
		defer cleanup()
	}

	duration, err := probeVideoDuration(resolvedOptions.InputPath, runner)
	if err != nil {
		return err
	}

	if generatedMask {
		fmt.Printf("generated temporary mask: %s\n", maskPath)
	} else {
		fmt.Printf("using provided mask: %s\n", maskPath)
	}

	args, err := buildFFmpegArgs(resolvedOptions, frameLayout, maskPath, duration)
	if err != nil {
		return err
	}

	_, stderr, err := runner.Run("ffmpeg", args)
	if err != nil {
		return fmt.Errorf("run ffmpeg: %w\n%s", err, strings.TrimSpace(string(stderr)))
	}

	fmt.Printf("Wrote %s\n", resolvedOptions.OutputPath)
	return nil
}

func resolveMaskPath(options VideoOptions, frameLayout _frameLayout) (string, func(), bool, error) {
	if strings.TrimSpace(options.MaskPath) != "" {
		return options.MaskPath, nil, false, nil
	}

	tempDir, err := os.MkdirTemp("", "frame-video-mask-*")
	if err != nil {
		return "", nil, false, fmt.Errorf("create temp dir: %w", err)
	}
	maskPath := filepath.Join(tempDir, "mask.png")
	if err := savePNG(maskPath, frameLayout.maskImage); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, false, fmt.Errorf("write generated mask: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	return maskPath, cleanup, true, nil
}

func probeVideoDuration(inputPath string, runner _commandRunner) (string, error) {
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		inputPath,
	}
	stdout, stderr, err := runner.Run("ffprobe", args)
	if err != nil {
		return "", fmt.Errorf("run ffprobe: %w\n%s", err, strings.TrimSpace(string(stderr)))
	}

	duration := strings.TrimSpace(string(stdout))
	if duration == "" {
		return "", fmt.Errorf("ffprobe returned empty duration for %s", inputPath)
	}
	return duration, nil
}

func buildFFmpegArgs(options VideoOptions, layout _frameLayout, maskPath string, duration string) ([]string, error) {
	videoDuration, err := strconv.ParseFloat(duration, 64)
	if err != nil {
		return nil, fmt.Errorf("parse duration %q: %w", duration, err)
	}

	frameBounds := layout.frameImage.Bounds()
	screenRect := layout.screen.Rect

	frameWidth := frameBounds.Dx()
	frameHeight := frameBounds.Dy()
	screenWidth := screenRect.Dx()
	screenHeight := screenRect.Dy()
	screenX := screenRect.Min.X - frameBounds.Min.X
	screenY := screenRect.Min.Y - frameBounds.Min.Y

	socialPhoneWidth := int(float64(frameWidth) * float64(socialHeight) / float64(frameHeight))
	socialPhoneX := (socialWidth - socialPhoneWidth) / 2

	filterSteps := []string{
		fmt.Sprintf(
			"[0:v]fps=%d,scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,format=rgba[screen]",
			options.FPS, screenWidth, screenHeight, screenWidth, screenHeight,
		),
		fmt.Sprintf(
			"color=c=black@0.0:s=%dx%d:r=%d,format=rgba[canvas]",
			frameWidth, frameHeight, options.FPS,
		),
		fmt.Sprintf("[canvas][screen]overlay=%d:%d:format=auto[placed]", screenX, screenY),
		"[placed][1:v]overlay=0:0:format=auto[framed]",
		"[2:v]format=gray[mask]",
		"[framed][mask]alphamerge[cutout]",
		fmt.Sprintf("[cutout]scale=%d:%d:force_original_aspect_ratio=decrease[phone]", socialPhoneWidth, socialHeight),
		fmt.Sprintf("color=c=%s:s=%dx%d:r=%d,format=rgba[socialbg]", options.BackgroundColor, socialWidth, socialHeight, options.FPS),
		fmt.Sprintf("[socialbg][phone]overlay=%d:(H-h)/2:format=auto,format=yuv420p[basev]", socialPhoneX),
	}

	outputDuration := videoDuration
	if strings.TrimSpace(options.CTAPath) != "" {
		fadeOffset := maxFloat(videoDuration-options.CTAFadeSeconds, 0)
		filterSteps = append(filterSteps,
			fmt.Sprintf(
				"[3:v]fps=%d,settb=1/%d,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=%s,format=yuv420p[cta]",
				options.FPS,
				options.FPS,
				socialWidth,
				socialHeight,
				socialWidth,
				socialHeight,
				options.BackgroundColor,
			),
			fmt.Sprintf(
				"[basev]fps=%d,settb=1/%d,format=yuv420p[maincta]",
				options.FPS,
				options.FPS,
			),
			fmt.Sprintf(
				"[maincta][cta]xfade=transition=fade:duration=%s:offset=%s[v]",
				formatSeconds(options.CTAFadeSeconds),
				formatSeconds(fadeOffset),
			),
		)
		outputDuration = videoDuration + options.CTALengthSeconds
	} else {
		filterSteps = append(filterSteps, "[basev]null[v]")
	}

	filterGraph := strings.Join(filterSteps, ";")

	args := []string{
		"-y",
		"-i", options.InputPath,
		"-stream_loop", "-1",
		"-i", options.FramePath,
		"-stream_loop", "-1",
		"-i", maskPath,
	}
	if strings.TrimSpace(options.CTAPath) != "" {
		ctaDuration := options.CTAFadeSeconds + options.CTALengthSeconds
		args = append(args,
			"-loop", "1",
			"-t", formatSeconds(ctaDuration),
			"-i", options.CTAPath,
		)
	}

	args = append(args,
		"-filter_complex", filterGraph,
		"-map", "[v]",
		"-an",
		"-c:v", "h264_videotoolbox",
		"-b:v", options.Bitrate,
		"-t", formatSeconds(outputDuration),
		options.OutputPath,
	)

	return args, nil
}

func parseHexColor(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("background color is empty")
	}
	trimmed := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "0x")
	if len(trimmed) != 6 {
		return fmt.Errorf("background color %q must be in 0xRRGGBB format", value)
	}
	if _, err := strconv.ParseUint(trimmed, 16, 24); err != nil {
		return fmt.Errorf("background color %q is not valid hex", value)
	}
	return nil
}

func validateCTAOptions(options VideoOptions) error {
	if strings.TrimSpace(options.CTAPath) == "" {
		return nil
	}
	if options.CTAFadeSeconds <= 0 {
		return fmt.Errorf("cta fade must be greater than 0 when --cta is set")
	}
	if options.CTALengthSeconds < 0 {
		return fmt.Errorf("cta length must be 0 or greater when --cta is set")
	}
	return nil
}

func formatSeconds(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}

func maxFloat(a float64, b float64) float64 {
	return math.Max(a, b)
}

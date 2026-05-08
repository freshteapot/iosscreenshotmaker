package frame

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type _fakeCommandRunner struct {
	t                *testing.T
	ffprobeOutput    string
	ffprobeCalls     int
	ffmpegCalls      int
	lastFFmpegArgs   []string
	maskPathSeen     string
	maskExistedAtRun bool
}

func (f *_fakeCommandRunner) Run(name string, args []string) ([]byte, []byte, error) {
	switch name {
	case "ffprobe":
		f.ffprobeCalls += 1
		return []byte(f.ffprobeOutput), nil, nil
	case "ffmpeg":
		f.ffmpegCalls += 1
		f.lastFFmpegArgs = append([]string(nil), args...)
		maskPath := ""
		for index, value := range args {
			if value == "-i" && index > 0 && args[index-1] == "-1" && index+1 < len(args) {
				maskPath = args[index+1]
			}
		}
		if maskPath != "" {
			f.maskPathSeen = maskPath
			_, err := os.Stat(maskPath)
			f.maskExistedAtRun = err == nil
		}
		return nil, nil, nil
	default:
		f.t.Fatalf("unexpected command: %s", name)
		return nil, nil, nil
	}
}

func TestBuildFFmpegArgsUsesExpectedSocialPipeline(t *testing.T) {
	layout, err := loadFrameLayout(testFramePath())
	if err != nil {
		t.Fatalf("loadFrameLayout: %v", err)
	}

	options := VideoOptions{
		FramePath:       testFramePath(),
		InputPath:       "recording.mp4",
		OutputPath:      "framed.mp4",
		MaskPath:        "mask.png",
		BackgroundColor: "0xf7f6f2",
		FPS:             30,
		Bitrate:         "8M",
	}

	args, err := buildFFmpegArgs(options, layout, options.MaskPath, "18.25")
	if err != nil {
		t.Fatalf("buildFFmpegArgs: %v", err)
	}

	argsText := strings.Join(args, " ")
	for _, expected := range []string{
		"-t 18.25",
		"-c:v h264_videotoolbox",
		"-b:v 8M",
		"s=1080x1920",
		"overlay=60:72",
		"scale=1260:2736",
		"mask.png",
		"[basev]null[v]",
	} {
		if !strings.Contains(argsText, expected) {
			t.Fatalf("ffmpeg args missing %q: %s", expected, argsText)
		}
	}
}

func TestBuildFFmpegArgsWithCTAUsesXFadeAndExtendedDuration(t *testing.T) {
	layout, err := loadFrameLayout(testFramePath())
	if err != nil {
		t.Fatalf("loadFrameLayout: %v", err)
	}

	options := VideoOptions{
		FramePath:        testFramePath(),
		InputPath:        "recording.mp4",
		OutputPath:       "framed.mp4",
		MaskPath:         "mask.png",
		BackgroundColor:  "0xf7f6f2",
		FPS:              30,
		Bitrate:          "8M",
		CTAPath:          "cta.png",
		CTAFadeSeconds:   1,
		CTALengthSeconds: 2,
	}

	args, err := buildFFmpegArgs(options, layout, options.MaskPath, "10.433")
	if err != nil {
		t.Fatalf("buildFFmpegArgs: %v", err)
	}

	argsText := strings.Join(args, " ")
	for _, expected := range []string{
		"-loop 1 -t 3.000 -i cta.png",
		"xfade=transition=fade:duration=1.000:offset=9.433",
		"[3:v]fps=30,settb=1/30",
		"-t 12.433",
	} {
		if !strings.Contains(argsText, expected) {
			t.Fatalf("ffmpeg args missing %q: %s", expected, argsText)
		}
	}
}

func TestRunVideoUsesProvidedMaskWithoutGeneratingTempMask(t *testing.T) {
	tempDir := t.TempDir()
	maskPath := filepath.Join(tempDir, "provided-mask.png")
	if err := GenerateMask(testFramePath(), maskPath); err != nil {
		t.Fatalf("GenerateMask: %v", err)
	}

	runner := &_fakeCommandRunner{
		t:             t,
		ffprobeOutput: "18.5\n",
	}

	err := runVideo(VideoOptions{
		FramePath:  testFramePath(),
		InputPath:  "recording.mp4",
		OutputPath: filepath.Join(tempDir, "out.mp4"),
		MaskPath:   maskPath,
	}, runner)
	if err != nil {
		t.Fatalf("runVideo: %v", err)
	}

	if runner.ffprobeCalls != 1 || runner.ffmpegCalls != 1 {
		t.Fatalf("calls = ffprobe %d ffmpeg %d, want 1 each", runner.ffprobeCalls, runner.ffmpegCalls)
	}
	if runner.maskPathSeen != maskPath {
		t.Fatalf("maskPathSeen = %q, want %q", runner.maskPathSeen, maskPath)
	}
	if !runner.maskExistedAtRun {
		t.Fatalf("provided mask did not exist during ffmpeg invocation")
	}
}

func TestRunVideoGeneratesAndCleansUpTempMask(t *testing.T) {
	tempDir := t.TempDir()
	runner := &_fakeCommandRunner{
		t:             t,
		ffprobeOutput: "18.5\n",
	}

	err := runVideo(VideoOptions{
		FramePath:  testFramePath(),
		InputPath:  "recording.mp4",
		OutputPath: filepath.Join(tempDir, "out.mp4"),
	}, runner)
	if err != nil {
		t.Fatalf("runVideo: %v", err)
	}

	if runner.maskPathSeen == "" {
		t.Fatalf("expected generated temp mask path")
	}
	if !runner.maskExistedAtRun {
		t.Fatalf("generated mask did not exist during ffmpeg invocation")
	}
	if _, err := os.Stat(runner.maskPathSeen); !os.IsNotExist(err) {
		t.Fatalf("generated mask still exists after run: %v", err)
	}
}

func TestRunVideoKeepsTempMaskWhenRequested(t *testing.T) {
	tempDir := t.TempDir()
	runner := &_fakeCommandRunner{
		t:             t,
		ffprobeOutput: "18.5\n",
	}

	err := runVideo(VideoOptions{
		FramePath:  testFramePath(),
		InputPath:  "recording.mp4",
		OutputPath: filepath.Join(tempDir, "out.mp4"),
		KeepTemp:   true,
	}, runner)
	if err != nil {
		t.Fatalf("runVideo: %v", err)
	}

	if _, err := os.Stat(runner.maskPathSeen); err != nil {
		t.Fatalf("expected temp mask to remain: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(runner.maskPathSeen))
}

func TestRunVideoRejectsInvalidCTAOptions(t *testing.T) {
	runner := &_fakeCommandRunner{
		t:             t,
		ffprobeOutput: "18.5\n",
	}

	err := runVideo(VideoOptions{
		FramePath:        testFramePath(),
		InputPath:        "recording.mp4",
		OutputPath:       filepath.Join(t.TempDir(), "out.mp4"),
		CTAPath:          "cta.png",
		CTAFadeSeconds:   0,
		CTALengthSeconds: 2,
	}, runner)
	if err == nil || !strings.Contains(err.Error(), "cta fade") {
		t.Fatalf("expected CTA fade validation error, got %v", err)
	}
}

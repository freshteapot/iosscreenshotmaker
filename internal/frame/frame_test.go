package frame

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func testFramePath() string {
	return filepath.Join("..", "..", "frames", "iPhone Air - Cloud White - Portrait.png")
}

func TestDetectScreenRegionMatchesExpectedRect(t *testing.T) {
	frameImage, err := loadImage(testFramePath())
	if err != nil {
		t.Fatalf("load frame: %v", err)
	}

	screen, err := detectScreenRegion(frameImage)
	if err != nil {
		t.Fatalf("detect screen: %v", err)
	}

	expected := image.Rect(60, 72, 1320, 2808)
	if screen.Rect != expected {
		t.Fatalf("screen rect = %v, want %v", screen.Rect, expected)
	}
}

func TestGenerateSilhouetteMaskIncludesScreenOpening(t *testing.T) {
	frameImage, err := loadImage(testFramePath())
	if err != nil {
		t.Fatalf("load frame: %v", err)
	}

	screen, err := detectScreenRegion(frameImage)
	if err != nil {
		t.Fatalf("detect screen: %v", err)
	}

	maskImage, err := generateSilhouetteMask(frameImage, screen)
	if err != nil {
		t.Fatalf("generate silhouette mask: %v", err)
	}

	if got := maskImage.GrayAt(0, 0).Y; got != 0 {
		t.Fatalf("outside mask pixel = %d, want 0", got)
	}

	centerX := (screen.Rect.Min.X + screen.Rect.Max.X) / 2
	centerY := (screen.Rect.Min.Y + screen.Rect.Max.Y) / 2
	if got := maskImage.GrayAt(centerX, centerY).Y; got != 255 {
		t.Fatalf("screen opening mask pixel = %d, want 255", got)
	}

	frameBodyX := screen.Rect.Min.X - 10
	frameBodyY := (screen.Rect.Min.Y + screen.Rect.Max.Y) / 2
	if got := maskImage.GrayAt(frameBodyX, frameBodyY).Y; got != 255 {
		t.Fatalf("frame body mask pixel = %d, want 255", got)
	}
}

func TestGenerateMaskWritesGrayscalePNG(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "mask.png")

	if err := GenerateMask(testFramePath(), outputPath); err != nil {
		t.Fatalf("GenerateMask: %v", err)
	}

	file, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode mask png: %v", err)
	}

	if _, ok := img.(*image.Gray); !ok {
		t.Fatalf("mask image type = %T, want *image.Gray", img)
	}
}

func TestFrameScreenshotRegression(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.png")
	outputPath := filepath.Join(tempDir, "output.png")

	input := image.NewNRGBA(image.Rect(0, 0, 100, 200))
	fill := color.NRGBA{R: 12, G: 34, B: 56, A: 255}
	for y := 0; y < 200; y++ {
		for x := 0; x < 100; x++ {
			input.SetNRGBA(x, y, fill)
		}
	}
	if err := savePNG(inputPath, input); err != nil {
		t.Fatalf("save input: %v", err)
	}

	if err := Run(Options{
		FramePath:  testFramePath(),
		InputPath:  inputPath,
		OutputPath: outputPath,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	framedOutput, err := loadImage(outputPath)
	if err != nil {
		t.Fatalf("load output: %v", err)
	}

	if got := framedOutput.Bounds(); got != image.Rect(0, 0, 1380, 2880) {
		t.Fatalf("output bounds = %v, want 1380x2880", got)
	}

	screenCenter := color.NRGBAModel.Convert(framedOutput.At(689, 1440)).(color.NRGBA)
	if screenCenter.R != fill.R || screenCenter.G != fill.G || screenCenter.B != fill.B {
		t.Fatalf("screen center = %#v, want %#v", screenCenter, fill)
	}
}

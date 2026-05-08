package frame

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
)

type Options struct {
	FramePath  string
	InputPath  string
	OutputPath string
}

type point struct {
	X int
	Y int
}

type screenRegion struct {
	Rect image.Rectangle
	Mask [][]bool
}

func Run(options Options) error {
	frameImage, err := loadImage(options.FramePath)
	if err != nil {
		return fmt.Errorf("load frame: %w", err)
	}

	screenImage, err := loadImage(options.InputPath)
	if err != nil {
		return fmt.Errorf("load screenshot: %w", err)
	}

	frameLayout, err := loadFrameLayout(options.FramePath)
	if err != nil {
		return err
	}

	output := image.NewNRGBA(frameImage.Bounds())
	drawCoverMasked(output, frameLayout.screen, screenImage)
	draw.Draw(output, output.Bounds(), frameImage, frameImage.Bounds().Min, draw.Over)

	if err := savePNG(options.OutputPath, output); err != nil {
		return fmt.Errorf("save output: %w", err)
	}

	return nil
}

func GenerateMask(framePath string, outputPath string) error {
	frameLayout, err := loadFrameLayout(framePath)
	if err != nil {
		return err
	}

	if err := savePNG(outputPath, frameLayout.maskImage); err != nil {
		return fmt.Errorf("save mask: %w", err)
	}

	return nil
}

func loadFrameLayout(framePath string) (_frameLayout, error) {
	frameImage, err := loadImage(framePath)
	if err != nil {
		return _frameLayout{}, fmt.Errorf("load frame: %w", err)
	}

	screen, err := detectScreenRegion(frameImage)
	if err != nil {
		return _frameLayout{}, fmt.Errorf("detect screen mask: %w", err)
	}

	silhouetteMask, err := generateSilhouetteMask(frameImage, screen)
	if err != nil {
		return _frameLayout{}, fmt.Errorf("generate silhouette mask: %w", err)
	}

	return _frameLayout{
		frameImage: frameImage,
		screen:     screen,
		maskImage:  silhouetteMask,
	}, nil
}

func loadImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	imageValue, _, err := image.Decode(file)
	return imageValue, err
}

func savePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

func detectScreenRegion(img image.Image) (screenRegion, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	visited := make([][]bool, height)
	for y := 0; y < height; y++ {
		visited[y] = make([]bool, width)
	}

	bestArea := 0
	bestRect := image.Rectangle{}
	var bestMask [][]bool

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if visited[y][x] || !isTransparent(img.At(bounds.Min.X+x, bounds.Min.Y+y)) {
				continue
			}

			rect, touchesEdge, area, regionMask := floodTransparentRegion(img, x, y, visited)
			if touchesEdge || area <= bestArea {
				continue
			}

			bestRect = rect
			bestArea = area
			bestMask = regionMask
		}
	}

	if bestArea == 0 {
		return screenRegion{}, fmt.Errorf("no inner transparent screen region found")
	}

	return screenRegion{
		Rect: image.Rect(
			bestRect.Min.X+bounds.Min.X,
			bestRect.Min.Y+bounds.Min.Y,
			bestRect.Max.X+bounds.Min.X,
			bestRect.Max.Y+bounds.Min.Y,
		),
		Mask: bestMask,
	}, nil
}

func generateSilhouetteMask(frameImage image.Image, screen screenRegion) (*image.Gray, error) {
	bounds := frameImage.Bounds()
	maskImage := image.NewGray(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			keep := !isTransparent(frameImage.At(x, y))
			if !keep && screen.Mask[y-bounds.Min.Y][x-bounds.Min.X] {
				keep = true
			}

			if keep {
				maskImage.SetGray(x, y, color.Gray{Y: 255})
				continue
			}
			maskImage.SetGray(x, y, color.Gray{Y: 0})
		}
	}

	return maskImage, nil
}

func floodTransparentRegion(img image.Image, startX int, startY int, visited [][]bool) (image.Rectangle, bool, int, [][]bool) {
	bounds := img.Bounds()
	queue := []point{{X: startX, Y: startY}}
	visited[startY][startX] = true

	minX := startX
	maxX := startX
	minY := startY
	maxY := startY
	area := 0
	touchesEdge := false
	mask := make([][]bool, bounds.Dy())
	for y := 0; y < bounds.Dy(); y++ {
		mask[y] = make([]bool, bounds.Dx())
	}

	directions := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

	for len(queue) > 0 {
		currentPoint := queue[0]
		queue = queue[1:]
		area++
		mask[currentPoint.Y][currentPoint.X] = true

		if currentPoint.X == 0 || currentPoint.Y == 0 || currentPoint.X == bounds.Dx()-1 || currentPoint.Y == bounds.Dy()-1 {
			touchesEdge = true
		}

		if currentPoint.X < minX {
			minX = currentPoint.X
		}
		if currentPoint.X > maxX {
			maxX = currentPoint.X
		}
		if currentPoint.Y < minY {
			minY = currentPoint.Y
		}
		if currentPoint.Y > maxY {
			maxY = currentPoint.Y
		}

		for _, direction := range directions {
			nextX := currentPoint.X + direction[0]
			nextY := currentPoint.Y + direction[1]
			if nextX < 0 || nextY < 0 || nextX >= bounds.Dx() || nextY >= bounds.Dy() {
				continue
			}
			if visited[nextY][nextX] || !isTransparent(img.At(bounds.Min.X+nextX, bounds.Min.Y+nextY)) {
				continue
			}

			visited[nextY][nextX] = true
			queue = append(queue, point{X: nextX, Y: nextY})
		}
	}

	return image.Rect(minX, minY, maxX+1, maxY+1), touchesEdge, area, mask
}

func drawCoverMasked(destination *image.NRGBA, screen screenRegion, source image.Image) {
	sourceBounds := source.Bounds()
	sourceWidth := float64(sourceBounds.Dx())
	sourceHeight := float64(sourceBounds.Dy())
	targetWidth := float64(screen.Rect.Dx())
	targetHeight := float64(screen.Rect.Dy())

	scale := max(targetWidth/sourceWidth, targetHeight/sourceHeight)
	scaledWidth := sourceWidth * scale
	scaledHeight := sourceHeight * scale
	offsetX := (scaledWidth - targetWidth) / 2.0
	offsetY := (scaledHeight - targetHeight) / 2.0

	for y := screen.Rect.Min.Y; y < screen.Rect.Max.Y; y++ {
		for x := screen.Rect.Min.X; x < screen.Rect.Max.X; x++ {
			if !screen.Mask[y][x] {
				continue
			}

			sourceX := (float64(x-screen.Rect.Min.X) + offsetX) / scale
			sourceY := (float64(y-screen.Rect.Min.Y) + offsetY) / scale

			sampledX := clampInt(int(math.Floor(sourceX+0.5)), 0, sourceBounds.Dx()-1)
			sampledY := clampInt(int(math.Floor(sourceY+0.5)), 0, sourceBounds.Dy()-1)

			sampledColor := color.NRGBAModel.Convert(source.At(sourceBounds.Min.X+sampledX, sourceBounds.Min.Y+sampledY)).(color.NRGBA)
			destination.SetNRGBA(x, y, sampledColor)
		}
	}
}

func isTransparent(c color.Color) bool {
	return color.NRGBAModel.Convert(c).(color.NRGBA).A == 0
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

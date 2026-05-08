package capture

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
)

const (
	DefaultChromePath    = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	DefaultBaseTargetURL = "http://127.0.0.1:8080/index.html"
	targetID             = "#framed"
	frameImageID         = "#frame-image"
	defaultOutputImage   = "output-like-a-boss.png"
)

type Options struct {
	ChromePath  string
	ConfigName  string
	BaseURL     string
	Device      string
	Orientation string
	PlanPath    string
	InputPath   string
	ServerDir   string
}

type inputAsset struct {
	Path                string
	HasTransparency     bool
	ShouldPreserveAlpha bool
}

func Run(options Options) error {
	stdinConfig, err := resolveInputConfig(options)
	if err != nil {
		return err
	}

	mergedConfig, err := mergeDefaultDimensions(options, stdinConfig)
	if err != nil {
		return err
	}

	configWithInput, err := applyInputOverride(options, mergedConfig)
	if err != nil {
		return err
	}

	return runWithConfig(options, configWithInput)
}

func resolveInputConfig(options Options) (string, error) {
	stdinConfig, err := readConfigFromStdin()
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(options.PlanPath) == "" {
		return stdinConfig, nil
	}

	if strings.TrimSpace(stdinConfig) != "" {
		return "", fmt.Errorf("cannot use --plan together with stdin config")
	}

	return readConfigFromPlan(options.PlanPath, options)
}

func runWithConfig(options Options, stdinConfig string) error {
	if _, err := os.Stat(options.ChromePath); err != nil {
		return fmt.Errorf("chrome executable %q: %w", options.ChromePath, err)
	}

	input, err := resolveInputAsset(stdinConfig)
	if err != nil {
		return err
	}

	profileDir, err := os.MkdirTemp("", "rod-chrome-profile-*")
	if err != nil {
		return fmt.Errorf("create temp profile: %w", err)
	}

	outputFileName, err := resolveOutputImage(stdinConfig)
	if err != nil {
		return err
	}

	outputPath, err := filepath.Abs(outputFileName)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}

	targetURL, err := buildTargetURL(options.BaseURL, options.ConfigName, stdinConfig)
	if err != nil {
		return err
	}

	fmt.Println("Open in browser")
	printPreviewURL(targetURL)

	launch := launcher.New().
		Bin(options.ChromePath).
		UserDataDir(profileDir).
		Headless(true).
		Leakless(false)

	controlURL, err := launch.Launch()
	if err != nil {
		return fmt.Errorf("launch chrome: %w", err)
	}
	defer launch.Cleanup()

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return fmt.Errorf("connect to chrome: %w", err)
	}
	defer func() {
		_ = browser.Close()
	}()

	page, err := browser.Page(proto.TargetCreateTarget{URL: targetURL})
	if err != nil {
		return fmt.Errorf("open %s: %w", targetURL, err)
	}
	defer func() {
		_ = page.Close()
	}()

	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("wait for page load: %w", err)
	}

	if input.ShouldPreserveAlpha {
		if err := enableTransparentCapture(page); err != nil {
			return err
		}
	}

	width, height, err := readFrameDimensions(page)
	if err != nil {
		return err
	}

	if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             width,
		Height:            height,
		DeviceScaleFactor: 1,
		Mobile:            false,
	}); err != nil {
		return fmt.Errorf("set viewport: %w", err)
	}

	element, err := page.Element(targetID)
	if err != nil {
		return fmt.Errorf("find %s: %w", targetID, err)
	}

	screenshot, err := captureElementScreenshot(page, element, input.ShouldPreserveAlpha)
	if err != nil {
		return fmt.Errorf("screenshot %s: %w", targetID, err)
	}

	if input.ShouldPreserveAlpha {
		screenshot, err = applyInputAlphaMask(page, element, screenshot, input.Path)
		if err != nil {
			return fmt.Errorf("apply input alpha mask: %w", err)
		}
	}

	if err := os.WriteFile(outputPath, screenshot, 0o644); err != nil {
		return fmt.Errorf("write screenshot %s: %w", outputPath, err)
	}

	fmt.Println(`open %s`, outputPath)
	return nil
}

func enableTransparentCapture(page *rod.Page) error {
	alpha := 0.0
	if err := (proto.EmulationSetDefaultBackgroundColorOverride{
		Color: &proto.DOMRGBA{R: 0, G: 0, B: 0, A: &alpha},
	}).Call(page); err != nil {
		return fmt.Errorf("set transparent browser background: %w", err)
	}

	if _, err := page.Eval(`() => {
		document.documentElement.style.background = "transparent";
		document.body.style.background = "transparent";

		const framed = document.getElementById("framed");
		if (framed) {
			framed.style.background = "transparent";
		}
	}`); err != nil {
		return fmt.Errorf("set transparent capture styles: %w", err)
	}

	return nil
}

func captureElementScreenshot(page *rod.Page, element *rod.Element, omitBackground bool) ([]byte, error) {
	if !omitBackground {
		return element.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
	}

	if err := element.ScrollIntoView(); err != nil {
		return nil, err
	}

	var result struct {
		Data []byte `json:"data"`
	}

	request := map[string]any{
		"format":                "png",
		"omitBackground":        true,
		"fromSurface":           true,
		"captureBeyondViewport": false,
	}

	response, err := page.Call(context.Background(), string(page.SessionID), "Page.captureScreenshot", request)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return nil, fmt.Errorf("decode screenshot response: %w", err)
	}

	shape, err := element.Shape()
	if err != nil {
		return nil, err
	}

	box := shape.Box()
	return utils.CropImage(
		result.Data,
		0,
		int(box.X),
		int(box.Y),
		int(box.Width),
		int(box.Height),
	)
}

func applyInputAlphaMask(page *rod.Page, target *rod.Element, screenshot []byte, inputPath string) ([]byte, error) {
	frameImage, err := page.Element(frameImageID)
	if err != nil {
		return nil, fmt.Errorf("find %s: %w", frameImageID, err)
	}

	targetShape, err := target.Shape()
	if err != nil {
		return nil, fmt.Errorf("read %s bounds: %w", targetID, err)
	}

	frameShape, err := frameImage.Shape()
	if err != nil {
		return nil, fmt.Errorf("read %s bounds: %w", frameImageID, err)
	}

	targetBox := targetShape.Box()
	frameBox := frameShape.Box()

	maskedScreenshot, err := applyAlphaMaskToScreenshot(screenshot, inputPath, targetBox, frameBox)
	if err != nil {
		return nil, err
	}

	return maskedScreenshot, nil
}

func applyAlphaMaskToScreenshot(screenshot []byte, inputPath string, targetBox *proto.DOMRect, frameBox *proto.DOMRect) ([]byte, error) {
	outputImage, _, err := image.Decode(bytes.NewReader(screenshot))
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}

	inputFile, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("open input image %q: %w", inputPath, err)
	}
	defer func() {
		_ = inputFile.Close()
	}()

	inputImage, _, err := image.Decode(inputFile)
	if err != nil {
		return nil, fmt.Errorf("decode input image %q: %w", inputPath, err)
	}

	outputBounds := outputImage.Bounds()
	masked := image.NewNRGBA(outputBounds)
	for y := outputBounds.Min.Y; y < outputBounds.Max.Y; y++ {
		for x := outputBounds.Min.X; x < outputBounds.Max.X; x++ {
			masked.Set(x, y, outputImage.At(x, y))
		}
	}

	maskLeft := int(frameBox.X - targetBox.X)
	maskTop := int(frameBox.Y - targetBox.Y)
	maskWidth := int(frameBox.Width)
	maskHeight := int(frameBox.Height)
	if maskWidth <= 0 || maskHeight <= 0 {
		return screenshot, nil
	}

	inputBounds := inputImage.Bounds()
	for y := 0; y < maskHeight; y++ {
		destY := maskTop + y
		if destY < outputBounds.Min.Y || destY >= outputBounds.Max.Y {
			continue
		}

		sourceY := inputBounds.Min.Y + y*inputBounds.Dy()/maskHeight
		for x := 0; x < maskWidth; x++ {
			destX := maskLeft + x
			if destX < outputBounds.Min.X || destX >= outputBounds.Max.X {
				continue
			}

			sourceX := inputBounds.Min.X + x*inputBounds.Dx()/maskWidth
			red, green, blue, alpha := masked.At(destX, destY).RGBA()
			_, _, _, maskAlpha := inputImage.At(sourceX, sourceY).RGBA()
			if maskAlpha == 0xffff {
				continue
			}

			masked.SetNRGBA(destX, destY, color.NRGBA{
				R: uint8(red >> 8),
				G: uint8(green >> 8),
				B: uint8(blue >> 8),
				A: uint8((alpha * maskAlpha / 0xffff) >> 8),
			})
		}
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, masked); err != nil {
		return nil, fmt.Errorf("encode masked screenshot: %w", err)
	}

	return encoded.Bytes(), nil
}

func buildTargetURL(baseTargetURL string, configName string, stdinConfig string) (string, error) {
	targetURL, err := url.Parse(baseTargetURL)
	if err != nil {
		return "", fmt.Errorf("parse target url %q: %w", baseTargetURL, err)
	}

	query := targetURL.Query()
	query.Set("capture", "1")
	if configName != "" {
		query.Set("configName", configName)
	}
	if stdinConfig != "" {
		query.Set("config", stdinConfig)
	}
	targetURL.RawQuery = query.Encode()

	return targetURL.String(), nil
}

func printPreviewURL(rawTargetURL string) {
	targetURL, err := url.Parse(rawTargetURL)
	if err != nil {
		return
	}

	query := targetURL.Query()
	query.Set("capture", "0")
	targetURL.RawQuery = query.Encode()

	fmt.Println("open_url:", targetURL.String())
}

func resolveOutputImage(stdinConfig string) (string, error) {
	if stdinConfig == "" {
		return defaultOutputImage, nil
	}

	var config struct {
		Output string `json:"output"`
	}

	if err := json.Unmarshal([]byte(stdinConfig), &config); err != nil {
		return "", fmt.Errorf("parse stdin config for output: %w", err)
	}

	if strings.TrimSpace(config.Output) == "" {
		return defaultOutputImage, nil
	}

	return config.Output, nil
}

func mergeDefaultDimensions(options Options, stdinConfig string) (string, error) {
	defaultWidth, defaultHeight, err := resolveDefaultDimensions(options.Device, options.Orientation)
	if err != nil {
		return "", err
	}

	if stdinConfig == "" {
		config := map[string]any{
			"width":  defaultWidth,
			"height": defaultHeight,
		}

		encodedConfig, err := json.Marshal(config)
		if err != nil {
			return "", fmt.Errorf("encode default dimensions: %w", err)
		}

		return string(encodedConfig), nil
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(stdinConfig), &config); err != nil {
		return "", fmt.Errorf("parse stdin config: %w", err)
	}

	if _, exists := config["width"]; !exists {
		config["width"] = defaultWidth
	}
	if _, exists := config["height"]; !exists {
		config["height"] = defaultHeight
	}

	encodedConfig, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode stdin config: %w", err)
	}

	return string(encodedConfig), nil
}

func applyInputOverride(options Options, stdinConfig string) (string, error) {
	var config map[string]any
	if strings.TrimSpace(stdinConfig) == "" {
		config = map[string]any{}
	} else if err := json.Unmarshal([]byte(stdinConfig), &config); err != nil {
		return "", fmt.Errorf("parse stdin config for input override: %w", err)
	}

	inputPath := strings.TrimSpace(options.InputPath)
	if inputPath == "" {
		inputPath = strings.TrimSpace(stringValue(config["input"]))
	}
	if inputPath == "" {
		return stdinConfig, nil
	}

	if strings.TrimSpace(options.ServerDir) == "" {
		return "", fmt.Errorf("--server-dir is required when --input is set")
	}

	fileName, imageURI, err := copyInputAsset(inputPath, options.ServerDir)
	if err != nil {
		return "", err
	}

	config["input"] = filepath.ToSlash(inputPath)
	config["imageUri"] = imageURI
	if _, exists := config["name"]; !exists {
		config["name"] = fileName
	}

	encodedConfig, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode config with input override: %w", err)
	}

	return string(encodedConfig), nil
}

func stringValue(value any) string {
	stringValue, ok := value.(string)
	if !ok {
		return ""
	}

	return stringValue
}

func resolveInputAsset(stdinConfig string) (inputAsset, error) {
	if strings.TrimSpace(stdinConfig) == "" {
		return inputAsset{}, nil
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(stdinConfig), &config); err != nil {
		return inputAsset{}, fmt.Errorf("parse stdin config for transparency detection: %w", err)
	}

	inputPath := strings.TrimSpace(stringValue(config["input"]))
	if inputPath == "" {
		return inputAsset{}, nil
	}

	hasTransparency, err := imageHasTransparency(inputPath)
	if err != nil {
		return inputAsset{}, fmt.Errorf("inspect input transparency %q: %w", inputPath, err)
	}

	hasInnerTransparentRegion, err := imageHasInnerTransparentRegion(inputPath)
	if err != nil {
		return inputAsset{}, fmt.Errorf("inspect input transparent regions %q: %w", inputPath, err)
	}

	return inputAsset{
		Path:                inputPath,
		HasTransparency:     hasTransparency,
		ShouldPreserveAlpha: hasInnerTransparentRegion,
	}, nil
}

func imageHasTransparency(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = file.Close()
	}()

	img, _, err := image.Decode(file)
	if err != nil {
		return false, err
	}

	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			if alpha < 0xffff {
				return true, nil
			}
		}
	}

	return false, nil
}

func imageHasInnerTransparentRegion(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = file.Close()
	}()

	img, _, err := image.Decode(file)
	if err != nil {
		return false, err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width == 0 || height == 0 {
		return false, nil
	}

	visited := make([][]bool, height)
	for y := range visited {
		visited[y] = make([]bool, width)
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if visited[y][x] {
				continue
			}

			_, _, _, alpha := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			if alpha != 0 {
				continue
			}

			touchesEdge, area := floodTransparentRegion(img, bounds, x, y, visited)
			if !touchesEdge && area > 0 {
				return true, nil
			}
		}
	}

	return false, nil
}

func floodTransparentRegion(img image.Image, bounds image.Rectangle, startX int, startY int, visited [][]bool) (bool, int) {
	queue := [][2]int{{startX, startY}}
	visited[startY][startX] = true

	width := bounds.Dx()
	height := bounds.Dy()
	area := 0
	touchesEdge := false
	directions := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

	for len(queue) > 0 {
		point := queue[0]
		queue = queue[1:]
		x := point[0]
		y := point[1]
		area++

		if x == 0 || y == 0 || x == width-1 || y == height-1 {
			touchesEdge = true
		}

		for _, direction := range directions {
			nextX := x + direction[0]
			nextY := y + direction[1]
			if nextX < 0 || nextY < 0 || nextX >= width || nextY >= height {
				continue
			}
			if visited[nextY][nextX] {
				continue
			}

			_, _, _, alpha := img.At(bounds.Min.X+nextX, bounds.Min.Y+nextY).RGBA()
			if alpha != 0 {
				continue
			}

			visited[nextY][nextX] = true
			queue = append(queue, [2]int{nextX, nextY})
		}
	}

	return touchesEdge, area
}

func readConfigFromPlan(planPath string, options Options) (string, error) {
	planBytes, err := os.ReadFile(planPath)
	if err != nil {
		return "", fmt.Errorf("read plan %q: %w", planPath, err)
	}

	var plan struct {
		App struct {
			OutputDir string `json:"output_dir"`
		} `json:"app"`
		Steps []struct {
			Action string `json:"action"`
			Name   string `json:"name"`
		} `json:"steps"`
		Screenshot map[string]any `json:"screenshot"`
	}

	if err := json.Unmarshal(planBytes, &plan); err != nil {
		return "", fmt.Errorf("parse plan %q: %w", planPath, err)
	}

	outputDir := strings.TrimSpace(plan.App.OutputDir)
	if outputDir == "" {
		return "", fmt.Errorf("plan %q is missing app.output_dir", planPath)
	}

	screenshotName, err := findScreenshotName(planPath, plan.Steps)
	if err != nil {
		return "", err
	}

	config := map[string]any{}
	for key, value := range plan.Screenshot {
		config[key] = value
	}

	outputSuffix, err := buildRenderedOutputSuffix(options, screenshotName)
	if err != nil {
		return "", err
	}

	config["imageUri"] = "/" + screenshotName + ".png"
	config["output"] = filepath.ToSlash(filepath.Join(outputDir, outputSuffix))

	encodedConfig, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode plan config: %w", err)
	}
	fmt.Println(string(encodedConfig))
	return string(encodedConfig), nil
}

func findScreenshotName(planPath string, steps []struct {
	Action string `json:"action"`
	Name   string `json:"name"`
}) (string, error) {
	var screenshotNames []string

	for _, step := range steps {
		if step.Action != "screenshot" {
			continue
		}

		name := strings.TrimSpace(step.Name)
		if name == "" {
			return "", fmt.Errorf("plan %q has a screenshot step without a name", planPath)
		}

		screenshotNames = append(screenshotNames, name)
	}

	if len(screenshotNames) == 0 {
		return "", fmt.Errorf("plan %q has no screenshot step", planPath)
	}

	if len(screenshotNames) > 1 {
		return "", fmt.Errorf("plan %q has more than one screenshot step", planPath)
	}

	return screenshotNames[0], nil
}

func buildRenderedOutputSuffix(options Options, screenshotName string) (string, error) {
	device := strings.ToLower(strings.TrimSpace(options.Device))
	if device == "" {
		device = "iphone"
	}

	orientation := strings.ToLower(strings.TrimSpace(options.Orientation))
	if orientation == "" {
		orientation = "portrait"
	}

	if _, _, err := resolveDefaultDimensions(device, orientation); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s-%s-rendered.png", screenshotName, device, orientation), nil
}

func copyInputAsset(inputPath string, serverDir string) (string, string, error) {
	inputAbsPath, err := filepath.Abs(inputPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve input path %q: %w", inputPath, err)
	}

	inputInfo, err := os.Stat(inputAbsPath)
	if err != nil {
		return "", "", fmt.Errorf("stat input %q: %w", inputAbsPath, err)
	}
	if inputInfo.IsDir() {
		return "", "", fmt.Errorf("input %q is a directory", inputAbsPath)
	}

	serverAbsPath, err := filepath.Abs(serverDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve server dir %q: %w", serverDir, err)
	}

	inputRoot := filepath.Join(serverAbsPath, "input")
	if err := os.MkdirAll(inputRoot, 0o755); err != nil {
		return "", "", fmt.Errorf("create input directory %q: %w", inputRoot, err)
	}

	randomDirectoryName, err := randomHex(12)
	if err != nil {
		return "", "", fmt.Errorf("generate random input directory: %w", err)
	}

	targetDirectory := filepath.Join(inputRoot, randomDirectoryName)
	if err := os.MkdirAll(targetDirectory, 0o755); err != nil {
		return "", "", fmt.Errorf("create target directory %q: %w", targetDirectory, err)
	}

	fileName := filepath.Base(inputAbsPath)
	targetPath := filepath.Join(targetDirectory, fileName)
	if err := copyFile(inputAbsPath, targetPath); err != nil {
		return "", "", err
	}

	imageURI := filepath.ToSlash(filepath.Join("/input", randomDirectoryName, fileName))
	return fileName, imageURI, nil
}

func copyFile(sourcePath string, targetPath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", sourcePath, err)
	}
	defer func() {
		_ = sourceFile.Close()
	}()

	targetFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("create target file %q: %w", targetPath, err)
	}
	defer func() {
		_ = targetFile.Close()
	}()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		return fmt.Errorf("copy %q to %q: %w", sourcePath, targetPath, err)
	}

	return nil
}

func randomHex(byteLength int) (string, error) {
	randomBytes := make([]byte, byteLength)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	return hex.EncodeToString(randomBytes), nil
}

func resolveDefaultDimensions(device string, orientation string) (int, int, error) {
	var width int
	var height int

	switch strings.ToLower(device) {
	case "", "iphone":
		width = 1284
		height = 2778
	case "ipad":
		width = 2048
		height = 2732
	default:
		return 0, 0, fmt.Errorf("unsupported device %q", device)
	}

	switch strings.ToLower(orientation) {
	case "", "portrait":
		return width, height, nil
	case "landscape":
		return height, width, nil
	default:
		return 0, 0, fmt.Errorf("unsupported orientation %q", orientation)
	}
}

func readConfigFromStdin() (string, error) {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("stat stdin: %w", err)
	}

	if stdinInfo.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin config: %w", err)
	}

	config := strings.TrimSpace(string(input))
	if config == "" {
		return "", nil
	}

	if !json.Valid([]byte(config)) {
		return "", fmt.Errorf("stdin config is not valid JSON")
	}

	return config, nil
}

func readFrameDimensions(page *rod.Page) (int, int, error) {
	element, err := page.Element(targetID)
	if err != nil {
		return 0, 0, fmt.Errorf("find %s for dimensions: %w", targetID, err)
	}

	widthValue, err := element.Attribute("data-width")
	if err != nil {
		return 0, 0, fmt.Errorf("read data-width: %w", err)
	}
	if widthValue == nil {
		return 0, 0, fmt.Errorf("%s is missing data-width", targetID)
	}

	heightValue, err := element.Attribute("data-height")
	if err != nil {
		return 0, 0, fmt.Errorf("read data-height: %w", err)
	}
	if heightValue == nil {
		return 0, 0, fmt.Errorf("%s is missing data-height", targetID)
	}

	width, err := strconv.Atoi(*widthValue)
	if err != nil {
		return 0, 0, fmt.Errorf("parse data-width %q: %w", *widthValue, err)
	}

	height, err := strconv.Atoi(*heightValue)
	if err != nil {
		return 0, 0, fmt.Errorf("parse data-height %q: %w", *heightValue, err)
	}

	return width, height, nil
}

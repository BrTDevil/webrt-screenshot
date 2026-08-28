package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"golang.org/x/image/draw"

	// Register the PNG decoder (chrome screenshots come back as PNG).
	_ "image/png"
)

// device is a preset viewport + user-agent combo, similar in spirit to
// Chrome DevTools' device toolbar presets.
type device struct {
	name          string
	width, height int64
	scale         float64
	mobile        bool
	userAgent     string
}

var devices = map[string]device{
	"desktop": {
		name: "desktop", width: 1920, height: 1080, scale: 1, mobile: false,
		userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
			"(KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36",
	},
	"tablet": {
		name: "tablet", width: 768, height: 1024, scale: 2, mobile: true,
		userAgent: "Mozilla/5.0 (iPad; CPU OS 17_4 like Mac OS X) AppleWebKit/605.1.15 " +
			"(KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
	},
	"mobile": {
		name: "mobile", width: 390, height: 844, scale: 3, mobile: true,
		userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) " +
			"AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
	},
}

// deviceOrder controls the capture order for -all.
var deviceOrder = []string{"desktop", "tablet", "mobile"}

func main() {
	targetURL := flag.String("url", "", "URL-ul paginii de capturat (obligatoriu)")
	full := flag.Bool("full", false, "Captură pe toată înălțimea paginii (implicit: doar zona vizibilă)")
	deviceName := flag.String("device", "desktop", "Preset: desktop, tablet sau mobile")
	all := flag.Bool("all", false, "Captură pentru toate device-urile (desktop, tablet, mobile) dintr-o dată; ignoră -device")
	width := flag.Int64("width", 0, "Lățime custom în px (suprascrie preset-ul de device; ignorat cu -all)")
	height := flag.Int64("height", 0, "Înălțime custom în px (suprascrie preset-ul de device; ignorat cu -all)")
	quality := flag.Float64("q", 85, "Calitate WebP (0-100)")
	wait := flag.Duration("wait", 500*time.Millisecond, "Așteptare suplimentară după încărcare (ex: 1s, 500ms)")
	timeout := flag.Duration("timeout", 30*time.Second, "Timeout total pentru încărcarea paginii")
	out := flag.String("out", "", "Numele fișierului de ieșire (implicit: generat din URL + device; ignorat cu -all)")
	outDir := flag.String("dir", ".", "Directorul unde se salvează captura")

	flag.Parse()

	if *targetURL == "" {
		fmt.Println("Eroare: -url este obligatoriu")
		fmt.Println()
		flag.Usage()
		os.Exit(1)
	}
	if *quality < 0 || *quality > 100 {
		fmt.Println("Eroare: -q trebuie să fie între 0 și 100")
		os.Exit(1)
	}

	fullURL := *targetURL
	if !strings.Contains(fullURL, "://") {
		fullURL = "https://" + fullURL
	}

	fmt.Println("scrennshots")
	fmt.Println("───────────")
	fmt.Printf("URL:     %s\n", fullURL)
	fmt.Printf("Mod:     %s\n", map[bool]string{true: "pagină completă", false: "doar zona vizibilă"}[*full])
	fmt.Println()

	if *all {
		failed := 0
		for _, name := range deviceOrder {
			dev := devices[name]
			outputPath := autoOutputPath("", *outDir, fullURL, dev, *full)
			if err := runOne(fullURL, dev, *full, *wait, *timeout, *quality, outputPath); err != nil {
				fmt.Printf("❌ %s → %v\n", dev.name, err)
				failed++
				continue
			}
			printSaved(outputPath)
		}
		if failed > 0 {
			os.Exit(1)
		}
		return
	}

	dev, ok := devices[strings.ToLower(*deviceName)]
	if !ok {
		fmt.Printf("Eroare: device necunoscut %q (valid: desktop, tablet, mobile)\n", *deviceName)
		os.Exit(1)
	}
	if *width > 0 {
		dev.width = *width
	}
	if *height > 0 {
		dev.height = *height
	}

	outputPath := autoOutputPath(*out, *outDir, fullURL, dev, *full)

	fmt.Printf("Device:  %s (%dx%d @%.0fx)\n", dev.name, dev.width, dev.height, dev.scale)
	fmt.Printf("Output:  %s\n", outputPath)
	fmt.Println()

	if err := runOne(fullURL, dev, *full, *wait, *timeout, *quality, outputPath); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}
	printSaved(outputPath)
}

// runOne captures a screenshot for a single device and saves it as WebP.
func runOne(fullURL string, dev device, full bool, wait, timeout time.Duration, quality float64, outputPath string) error {
	png, err := capture(fullURL, dev, full, wait, timeout)
	if err != nil {
		return fmt.Errorf("captura a eșuat: %w", err)
	}
	if err := savePNGAsWebP(png, outputPath, quality); err != nil {
		return fmt.Errorf("conversia la WebP a eșuat: %w", err)
	}
	return nil
}

func autoOutputPath(out, outDir, fullURL string, dev device, full bool) string {
	outputPath := out
	if outputPath == "" {
		mode := "viewport"
		if full {
			mode = "full"
		}
		outputPath = filepath.Join(outDir, fmt.Sprintf("%s_%s_%s.webp", slugFromURL(fullURL), dev.name, mode))
	} else if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(outDir, outputPath)
	}
	if !strings.HasSuffix(strings.ToLower(outputPath), ".webp") {
		outputPath += ".webp"
	}
	return outputPath
}

func printSaved(outputPath string) {
	info, _ := os.Stat(outputPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	fmt.Printf("✓  Salvat: %s (%.1f KB)\n", outputPath, float64(size)/1024)
}

// capture launches headless Chrome, emulates the requested device viewport
// and user-agent, navigates to the URL and returns a PNG screenshot.
func capture(targetURL string, dev device, full bool, wait, timeout time.Duration) ([]byte, error) {
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.WindowSize(int(dev.width), int(dev.height)),
	)
	if execPath, err := findChrome(); err == nil {
		allocOpts = append(allocOpts, chromedp.ExecPath(execPath))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocOpts...)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	ctx, cancelTimeout := context.WithTimeout(taskCtx, timeout)
	defer cancelTimeout()

	viewportOpts := []chromedp.EmulateViewportOption{chromedp.EmulateScale(dev.scale)}
	if dev.mobile {
		viewportOpts = append(viewportOpts, chromedp.EmulateMobile, chromedp.EmulateTouch)
	}

	actions := []chromedp.Action{
		chromedp.EmulateViewport(dev.width, dev.height, viewportOpts...),
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(wait),
	}

	var buf []byte
	if full {
		// Scroll through the whole page once first: this fires native
		// loading="lazy" images and any plain scroll-event listeners.
		actions = append(actions, autoScrollAction())

		// Then re-emulate the viewport at the page's full height, so the
		// entire page becomes "on screen" at once. This is what actually
		// makes scroll-triggered reveal libraries (AOS, framer-motion
		// whileInView, etc.) settle into their final visible state — many
		// of them replay/reset on every viewport enter+exit, which is
		// exactly what a plain "scroll down then jump back to top" does.
		var totalHeight float64
		actions = append(actions,
			chromedp.Evaluate(`Math.min(20000, Math.max(document.body.scrollHeight, document.documentElement.scrollHeight))`, &totalHeight),
			chromedp.ActionFunc(func(ctx context.Context) error {
				h := int64(totalHeight)
				if h < dev.height {
					h = dev.height
				}
				return chromedp.EmulateViewport(dev.width, h, viewportOpts...).Do(ctx)
			}),
			chromedp.Sleep(wait),
			waitImagesAction(5*time.Second),
		)
	}
	actions = append(actions, chromedp.CaptureScreenshot(&buf))

	// User-agent override must be set before navigation happens.
	tasks := append(chromedp.Tasks{
		emulation.SetUserAgentOverride(dev.userAgent),
	}, actions...)

	if err := chromedp.Run(ctx, tasks); err != nil {
		return nil, err
	}

	return buf, nil
}

// autoScrollScript scrolls the page from top to bottom in viewport-sized
// steps, which fires native loading="lazy" images and any listener bound
// to plain 'scroll' events. It deliberately does NOT jump back to the top
// afterwards — the caller instead re-emulates the viewport at full page
// height, which is what actually settles IntersectionObserver-based
// reveal animations (they tend to reset every time the element leaves the
// viewport, which a scroll-then-jump-to-top approach would trigger).
const autoScrollScript = `
(async () => {
	const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
	const step = Math.max(200, Math.floor(window.innerHeight * 0.8));
	let lastY = -1;
	for (let i = 0; i < 500; i++) {
		window.scrollBy(0, step);
		await delay(200);
		const atBottom = window.innerHeight + window.scrollY >= document.body.scrollHeight - 2;
		if (atBottom || window.scrollY === lastY) break;
		lastY = window.scrollY;
	}
})()
`

func autoScrollAction() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, exc, err := runtime.Evaluate(autoScrollScript).WithAwaitPromise(true).Do(ctx)
		if err != nil {
			return err
		}
		if exc != nil {
			return exc
		}
		return nil
	})
}

// waitImagesScript polls document.images until every one of them has
// finished loading (img.complete), or the given timeout elapses.
const waitImagesScript = `
(async (timeoutMs) => {
	const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
	const start = Date.now();
	while (Date.now() - start < timeoutMs) {
		const pending = Array.from(document.images).filter((img) => !img.complete);
		if (pending.length === 0) return;
		await delay(100);
	}
})(%d)
`

func waitImagesAction(timeout time.Duration) chromedp.Action {
	script := fmt.Sprintf(waitImagesScript, timeout.Milliseconds())
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, exc, err := runtime.Evaluate(script).WithAwaitPromise(true).Do(ctx)
		if err != nil {
			return err
		}
		if exc != nil {
			return exc
		}
		return nil
	})
}

// webpMaxDimension is libwebp's hard limit (WEBP_MAX_DIMENSION) on width
// and height. Full-page screenshots of long pages at a high device scale
// factor (e.g. mobile @3x) can exceed it, so we downscale to fit first.
const webpMaxDimension = 16383

func fitWithinWebPLimits(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= webpMaxDimension && h <= webpMaxDimension {
		return img
	}

	scale := float64(webpMaxDimension) / float64(max(w, h))
	newW := max(1, int(float64(w)*scale))
	newH := max(1, int(float64(h)*scale))

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

func savePNGAsWebP(pngData []byte, outputPath string, quality float64) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("nu pot crea directorul de output: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(pngData))
	if err != nil {
		return fmt.Errorf("decodare PNG eșuată: %w", err)
	}
	img = fitWithinWebPLimits(img)

	tempFile, err := os.CreateTemp(filepath.Dir(outputPath), ".scrennshots-*.webp")
	if err != nil {
		return fmt.Errorf("nu pot crea fișier temporar: %w", err)
	}
	tempName := tempFile.Name()
	defer func() {
		tempFile.Close()
		os.Remove(tempName)
	}()

	if err := webp.Encode(tempFile, img, &webp.Options{Lossless: false, Quality: float32(quality)}); err != nil {
		return fmt.Errorf("encodare WebP eșuată: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("nu pot înlocui fișierul existent: %w", err)
	}

	return os.Rename(tempName, outputPath)
}

// findChrome prefers a real Google Chrome install over other browsers
// (e.g. a snap-packaged Chromium), which tends to start faster and more
// reliably in sandboxed/headless environments.
func findChrome() (string, error) {
	candidates := []string{"google-chrome", "google-chrome-stable", "chromium-browser", "chromium"}
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no chrome/chromium binary found")
}

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// slugFromURL turns "https://example.com/some/page?x=1" into "example.com_some_page".
func slugFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "screenshot"
	}
	base := u.Hostname() + u.Path
	slug := strings.Trim(slugRe.ReplaceAllString(base, "_"), "_")
	if slug == "" {
		slug = "screenshot"
	}
	return slug
}

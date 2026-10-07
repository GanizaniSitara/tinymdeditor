package main

// --selftest OUT runs the rendered-editing checks in selftest.js inside the real
// WebView2 page, with the window hidden, writes the results to OUT and exits. It
// needs no input devices and never takes the foreground.
//
// Optional --shots DIR captures screenshots of the main window for each test case.

import (
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
)

//go:embed selftest.js
var selftestJS string

var (
	getDC              = user32.NewProc("GetDC")
	releaseDC          = user32.NewProc("ReleaseDC")
	printWindow        = user32.NewProc("PrintWindow")
	setWindowPos       = user32.NewProc("SetWindowPos")
	getWindowRect      = user32.NewProc("GetWindowRect")
	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	deleteDC           = gdi32.NewProc("DeleteDC")
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	gdiFlush           = gdi32.NewProc("GdiFlush")
)

var (
	shotMu      sync.Mutex
	shotCounter int
)

func sanitizeShotName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	lastHyphen := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastHyphen = false
		} else if r == '-' || r == '_' || r == ' ' {
			if !lastHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	s := strings.TrimRight(b.String(), "-")
	if s == "" {
		s = "shot"
	}
	return s
}

func captureWindow(hwnd uintptr, shotsDir, name string) string {
	shotMu.Lock()
	defer shotMu.Unlock()

	shotCounter++
	cleanName := sanitizeShotName(name)
	filename := fmt.Sprintf("%02d-%s.png", shotCounter, cleanName)
	filePath := filepath.Join(shotsDir, filename)

	var rc struct {
		Left, Top, Right, Bottom int32
	}
	ret, _, _ := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	width := int(rc.Right - rc.Left)
	height := int(rc.Bottom - rc.Top)
	if ret == 0 || width <= 0 || height <= 0 {
		width = 1280
		height = 900
	}

	hdcScreen, _, _ := getDC.Call(0)
	if hdcScreen == 0 {
		return "error: GetDC failed"
	}
	defer releaseDC.Call(0, hdcScreen)

	memDC, _, _ := createCompatibleDC.Call(hdcScreen)
	if memDC == 0 {
		return "error: CreateCompatibleDC failed"
	}
	defer deleteDC.Call(memDC)

	var bmi struct {
		biSize          uint32
		biWidth         int32
		biHeight        int32
		biPlanes        uint16
		biBitCount      uint16
		biCompression   uint32
		biSizeImage     uint32
		biXPelsPerMeter int32
		biYPelsPerMeter int32
		biClrUsed       uint32
		biClrImportant  uint32
	}
	bmi.biSize = uint32(unsafe.Sizeof(bmi))
	bmi.biWidth = int32(width)
	bmi.biHeight = -int32(height) // negative = top-down
	bmi.biPlanes = 1
	bmi.biBitCount = 32
	bmi.biCompression = 0 // BI_RGB

	var bits uintptr
	hBitmap, _, _ := createDIBSection.Call(
		memDC,
		uintptr(unsafe.Pointer(&bmi)),
		0, // DIB_RGB_COLORS
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hBitmap == 0 || bits == 0 {
		return "error: CreateDIBSection failed"
	}
	defer deleteObject.Call(hBitmap)

	oldBmp, _, _ := selectObject.Call(memDC, hBitmap)
	defer selectObject.Call(memDC, oldBmp)

	const PW_RENDERFULLCONTENT = 2
	pRet, _, pErr := printWindow.Call(hwnd, memDC, PW_RENDERFULLCONTENT)
	if pRet == 0 {
		pRet, _, pErr = printWindow.Call(hwnd, memDC, 0)
	}
	if pRet == 0 {
		return fmt.Sprintf("error: PrintWindow failed: %v", pErr)
	}
	gdiFlush.Call()

	dib := unsafe.Slice((*byte)(unsafe.Pointer(bits)), width*height*4)

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	allBlack := true
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcIdx := (y*width + x) * 4
			dstIdx := y*img.Stride + x*4
			b := dib[srcIdx+0]
			g := dib[srcIdx+1]
			r := dib[srcIdx+2]
			if r != 0 || g != 0 || b != 0 {
				allBlack = false
			}
			img.Pix[dstIdx+0] = r
			img.Pix[dstIdx+1] = g
			img.Pix[dstIdx+2] = b
			img.Pix[dstIdx+3] = 255
		}
	}

	f, err := os.Create(filePath)
	if err != nil {
		return "error: " + err.Error()
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return "error: " + err.Error()
	}

	if allBlack {
		return "error: blank capture"
	}
	return "ok"
}

func startSelftest(w webview2.WebView, out string, shotsDir string) {
	settingsFrozen = true
	hwnd := uintptr(w.Window())
	if shotsDir != "" {
		_ = os.MkdirAll(shotsDir, 0755)
		const SWP_SHOWWINDOW = 0x0040
		setWindowPos.Call(hwnd, 0, 0, 0, 1280, 900, SWP_SHOWWINDOW)
		updateWindowProc.Call(hwnd)
		w.Bind("goShot", func(name string) string {
			return captureWindow(hwnd, shotsDir, name)
		})
	} else {
		showWindowProc.Call(hwnd, 0) // SW_HIDE
	}

	w.Bind("goSelftestDone", func(result string) {
		os.WriteFile(out, []byte(result), 0o644)
		w.Dispatch(w.Terminate)
	})
	w.Init(selftestJS)

	timeout := 90 * time.Second
	if shotsDir != "" {
		timeout = 240 * time.Second
	}
	go func() {
		time.Sleep(timeout)
		os.WriteFile(out, []byte(`{"error":"self-test timed out"}`), 0o644)
		os.Exit(2)
	}()
}

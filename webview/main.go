// TinyMD — a fast, minimal Markdown editor for Windows.
//
// Build for release (2.6 MB, no console window):
//   GOOS=windows go build -ldflags="-s -w -H windowsgui" -trimpath -o tinymd.exe .
//
// Build for development (with debug info and console):
//   GOOS=windows go build -o tinymd.exe .

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
)

var currentFile string
var browseRoot string

// addRecentLater defers recording the command-line file until settings are loaded.
var addRecentLater bool

type treeNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path,omitempty"`
	Dir      bool       `json:"dir"`
	Children []treeNode `json:"children,omitempty"`
}

// scanTree walks root and returns a tree of folders plus .md files only.
// Folders with no .md descendants are pruned.
func scanTree(root string) treeNode {
	node := treeNode{Name: filepath.Base(root), Dir: true}
	entries, err := os.ReadDir(root)
	if err != nil {
		return node
	}
	var dirs, files []treeNode
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(root, name)
		if e.IsDir() {
			child := scanTree(full)
			if len(child.Children) > 0 {
				dirs = append(dirs, child)
			}
		} else if strings.EqualFold(filepath.Ext(name), ".md") {
			files = append(files, treeNode{Name: name, Path: full, Dir: false})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })
	node.Children = append(dirs, files...)
	return node
}

func treeJSON(root string) string {
	t := scanTree(root)
	t.Name = root
	b, _ := json.Marshal(t)
	return string(b)
}

var (
	comdlg32         = syscall.NewLazyDLL("comdlg32.dll")
	getOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
	getSaveFileNameW = comdlg32.NewProc("GetSaveFileNameW")

	user32            = syscall.NewLazyDLL("user32.dll")
	getForegroundWin  = user32.NewProc("GetForegroundWindow")
	registerClassExW  = user32.NewProc("RegisterClassExW")
	createWindowExW   = user32.NewProc("CreateWindowExW")
	showWindowProc    = user32.NewProc("ShowWindow")
	updateWindowProc  = user32.NewProc("UpdateWindow")
	destroyWindowProc = user32.NewProc("DestroyWindow")
	defWindowProcW    = user32.NewProc("DefWindowProcW")
	getSystemMetrics  = user32.NewProc("GetSystemMetrics")
	beginPaint        = user32.NewProc("BeginPaint")
	endPaint          = user32.NewProc("EndPaint")
	fillRect          = user32.NewProc("FillRect")
	drawTextW         = user32.NewProc("DrawTextW")
	loadCursorW       = user32.NewProc("LoadCursorW")
	getClientRect     = user32.NewProc("GetClientRect")
	getKeyState       = user32.NewProc("GetKeyState")
	getAsyncKeyState  = user32.NewProc("GetAsyncKeyState")
	createMenu        = user32.NewProc("CreateMenu")
	createPopupMenu   = user32.NewProc("CreatePopupMenu")
	appendMenuW       = user32.NewProc("AppendMenuW")
	setMenu           = user32.NewProc("SetMenu")
	drawMenuBar       = user32.NewProc("DrawMenuBar")
	setWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	callWindowProcW   = user32.NewProc("CallWindowProcW")
	messageBoxW       = user32.NewProc("MessageBoxW")
	openClipboard     = user32.NewProc("OpenClipboard")
	closeClipboard    = user32.NewProc("CloseClipboard")
	emptyClipboard    = user32.NewProc("EmptyClipboard")
	getClipboardData  = user32.NewProc("GetClipboardData")
	setClipboardData  = user32.NewProc("SetClipboardData")
	isClipboardFormat = user32.NewProc("IsClipboardFormatAvailable")
	setWindowsHookExW = user32.NewProc("SetWindowsHookExW")
	callNextHookEx    = user32.NewProc("CallNextHookEx")
	unhookWindowsHook = user32.NewProc("UnhookWindowsHookEx")

	gdi32            = syscall.NewLazyDLL("gdi32.dll")
	createFontW      = gdi32.NewProc("CreateFontW")
	selectObject     = gdi32.NewProc("SelectObject")
	setBkMode        = gdi32.NewProc("SetBkMode")
	setTextColor     = gdi32.NewProc("SetTextColor")
	deleteObject     = gdi32.NewProc("DeleteObject")
	getSysColorBrush = user32.NewProc("GetSysColorBrush")

	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalFree       = kernel32.NewProc("GlobalFree")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
	globalSize       = kernel32.NewProc("GlobalSize")

	shell32       = syscall.NewLazyDLL("shell32.dll")
	shellExecuteW = shell32.NewProc("ShellExecuteW")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
	gmemZeroInit  = 0x0040
	vkControl     = 0x11
	vkMenu        = 0x12
	whKeyboardLL  = 13
	hcAction      = 0
	wmKeyDown     = 0x0100
	wmSysKeyDown  = 0x0104
	wmCommand     = 0x0111
	mfString      = 0x0000
	mfPopup       = 0x0010
	mfSeparator   = 0x0800
	mbOk          = 0x0000
	mbInfo        = 0x0040

	idFileOpen   = 1001
	idFileSave   = 1002
	idFileSaveAs = 1003
	idFilePrint  = 1004
	idFileReveal = 1005
	idFileExit   = 1006
	idEditUndo   = 1101
	idEditCut    = 1102
	idEditCopy   = 1103
	idEditPaste  = 1104
	idEditAll    = 1105
	idHelpAbout  = 1201
)

var (
	editorKeyboardHook     uintptr
	editorKeyboardCallback uintptr
	menuWndProcCallback    uintptr
	oldMainWndProc         uintptr
)

type kbdLLHookStruct struct {
	VKCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

func getHWND() uintptr {
	hwnd, _, _ := getForegroundWin.Call()
	return hwnd
}

func winCallErr(name string, err error) error {
	if err != nil && err != syscall.Errno(0) {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return fmt.Errorf("%s failed", name)
}

func openClipboardWithRetry(hwnd uintptr) error {
	var lastErr error
	for i := 0; i < 20; i++ {
		ret, _, err := openClipboard.Call(hwnd)
		if ret != 0 {
			return nil
		}
		lastErr = winCallErr("OpenClipboard", err)
		time.Sleep(10 * time.Millisecond)
	}
	return lastErr
}

func readClipboardText(hwnd uintptr) (string, error) {
	available, _, _ := isClipboardFormat.Call(cfUnicodeText)
	if available == 0 {
		return "", nil
	}
	if err := openClipboardWithRetry(hwnd); err != nil {
		return "", err
	}
	defer closeClipboard.Call()

	handle, _, err := getClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		return "", winCallErr("GetClipboardData", err)
	}
	ptr, _, err := globalLock.Call(handle)
	if ptr == 0 {
		return "", winCallErr("GlobalLock", err)
	}
	defer globalUnlock.Call(handle)

	size, _, _ := globalSize.Call(handle)
	if size == 0 {
		return "", nil
	}
	raw := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), int(size/2))
	n := 0
	for n < len(raw) && raw[n] != 0 {
		n++
	}
	return syscall.UTF16ToString(raw[:n]), nil
}

func writeClipboardText(hwnd uintptr, text string) error {
	if hwnd == 0 {
		return fmt.Errorf("clipboard owner window unavailable")
	}
	if err := openClipboardWithRetry(hwnd); err != nil {
		return err
	}
	defer closeClipboard.Call()

	if ret, _, err := emptyClipboard.Call(); ret == 0 {
		return winCallErr("EmptyClipboard", err)
	}

	data := utf16From(text)
	handle, _, err := globalAlloc.Call(gmemMoveable|gmemZeroInit, uintptr(len(data)*2))
	if handle == 0 {
		return winCallErr("GlobalAlloc", err)
	}
	ptr, _, err := globalLock.Call(handle)
	if ptr == 0 {
		globalFree.Call(handle)
		return winCallErr("GlobalLock", err)
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(data)), data)
	globalUnlock.Call(handle)

	if ret, _, err := setClipboardData.Call(cfUnicodeText, handle); ret == 0 {
		globalFree.Call(handle)
		return winCallErr("SetClipboardData", err)
	}
	return nil
}

func installEditorKeyboardHook(w webview2.WebView) {
	hwnd := uintptr(w.Window())
	editorKeyboardCallback = syscall.NewCallback(func(nCode uintptr, wParam uintptr, lParam uintptr) uintptr {
		if nCode == hcAction && (wParam == wmKeyDown || wParam == wmSysKeyDown) {
			vk := uintptr((*kbdLLHookStruct)(unsafe.Pointer(lParam)).VKCode)
			if key, ok := editorShortcutFromKey(vk); ok && foregroundIs(hwnd) {
				w.Dispatch(func() {
					w.Eval(fmt.Sprintf("window.tinyMdHandleEditorShortcut && window.tinyMdHandleEditorShortcut(%q);", key))
				})
				return 1
			}
		}
		ret, _, _ := callNextHookEx.Call(editorKeyboardHook, uintptr(nCode), wParam, lParam)
		return ret
	})
	editorKeyboardHook, _, _ = setWindowsHookExW.Call(whKeyboardLL, editorKeyboardCallback, 0, 0)
}

func uninstallEditorKeyboardHook() {
	if editorKeyboardHook != 0 {
		unhookWindowsHook.Call(editorKeyboardHook)
		editorKeyboardHook = 0
	}
}

func editorShortcutFromKey(virtualKey uintptr) (string, bool) {
	ctrl, _, _ := getAsyncKeyState.Call(vkControl)
	alt, _, _ := getAsyncKeyState.Call(vkMenu)
	if int16(ctrl) >= 0 || int16(alt) < 0 {
		return "", false
	}
	return hookedEditorShortcut(virtualKey)
}

func hookedEditorShortcut(virtualKey uintptr) (string, bool) {
	switch virtualKey {
	case 'A':
		return "a", true
	case 'V':
		return "v", true
	case 'X':
		return "x", true
	default:
		return "", false
	}
}

func foregroundIs(hwnd uintptr) bool {
	foreground, _, _ := getForegroundWin.Call()
	return foreground == hwnd
}

func appendMenu(menu uintptr, flags uintptr, id uintptr, text string) {
	if flags&mfSeparator != 0 {
		appendMenuW.Call(menu, flags, 0, 0)
		return
	}
	t := utf16From(text)
	appendMenuW.Call(menu, flags, id, uintptr(unsafe.Pointer(&t[0])))
}

func installMenuWndProc(w webview2.WebView) {
	hwnd := uintptr(w.Window())
	menuWndProcCallback = syscall.NewCallback(func(hwnd uintptr, msg uintptr, wParam uintptr, lParam uintptr) uintptr {
		if msg == wmCommand && handleMenuCommand(w, wParam&0xffff) {
			return 0
		}
		if msg == wmClose && !handleClose(w) {
			return 0
		}
		ret, _, _ := callWindowProcW.Call(oldMainWndProc, hwnd, msg, wParam, lParam)
		return ret
	})
	oldMainWndProc, _, _ = setWindowLongPtrW.Call(hwnd, ^uintptr(0)-3, menuWndProcCallback)
}

func showAboutDialog(hwnd uintptr) {
	text := utf16From("TinyMD\nMarkdown editor for Windows.")
	title := utf16From("About TinyMD")
	messageBoxW.Call(hwnd, uintptr(unsafe.Pointer(&text[0])), uintptr(unsafe.Pointer(&title[0])), mbOk|mbInfo)
}

func showOpenDialog(hwnd uintptr) string {
	buf := make([]uint16, 260)

	filter := append(utf16From("Markdown Files (*.md)"), 0)
	filter = append(filter, utf16From("*.md")...)
	filter = append(filter, 0)
	filter = append(filter, utf16From("All Files (*.*)")...)
	filter = append(filter, 0)
	filter = append(filter, utf16From("*.*")...)
	filter = append(filter, 0, 0)

	title := utf16From("Open")
	title = append(title, 0)

	const structSize = 152
	var ofn [structSize]byte

	*(*uint32)(unsafe.Pointer(&ofn[0])) = structSize
	*(*uintptr)(unsafe.Pointer(&ofn[8])) = hwnd
	*(*uintptr)(unsafe.Pointer(&ofn[24])) = uintptr(unsafe.Pointer(&filter[0]))
	*(*uint32)(unsafe.Pointer(&ofn[44])) = 1
	*(*uintptr)(unsafe.Pointer(&ofn[48])) = uintptr(unsafe.Pointer(&buf[0]))
	*(*uint32)(unsafe.Pointer(&ofn[56])) = uint32(len(buf))
	*(*uintptr)(unsafe.Pointer(&ofn[80])) = uintptr(unsafe.Pointer(&title[0]))
	*(*uint32)(unsafe.Pointer(&ofn[88])) = 0x00001000 | 0x00000800 | 0x00000004

	ret, _, _ := getOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn[0])))
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func showSaveDialog(hwnd uintptr, defaultPath string) string {
	buf := make([]uint16, 260)
	defaultName := filepath.Base(defaultPath)
	if defaultName == "." || defaultName == string(filepath.Separator) {
		defaultName = "untitled.md"
	}
	copy(buf, utf16From(defaultName))

	filter := append(utf16From("Markdown Files (*.md)"), 0)
	filter = append(filter, utf16From("*.md")...)
	filter = append(filter, 0)
	filter = append(filter, utf16From("All Files (*.*)")...)
	filter = append(filter, 0)
	filter = append(filter, utf16From("*.*")...)
	filter = append(filter, 0, 0)

	title := utf16From("Save As")
	title = append(title, 0)
	defExt := utf16From("md")
	defExt = append(defExt, 0)

	const structSize = 152
	var ofn [structSize]byte

	*(*uint32)(unsafe.Pointer(&ofn[0])) = structSize
	*(*uintptr)(unsafe.Pointer(&ofn[8])) = hwnd
	*(*uintptr)(unsafe.Pointer(&ofn[24])) = uintptr(unsafe.Pointer(&filter[0]))
	*(*uint32)(unsafe.Pointer(&ofn[44])) = 1
	*(*uintptr)(unsafe.Pointer(&ofn[48])) = uintptr(unsafe.Pointer(&buf[0]))
	*(*uint32)(unsafe.Pointer(&ofn[56])) = uint32(len(buf))
	*(*uintptr)(unsafe.Pointer(&ofn[80])) = uintptr(unsafe.Pointer(&title[0]))
	*(*uint32)(unsafe.Pointer(&ofn[88])) = 0x00000002 | 0x00000800
	*(*uintptr)(unsafe.Pointer(&ofn[96])) = uintptr(unsafe.Pointer(&defExt[0]))

	ret, _, _ := getSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn[0])))
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func revealCurrentFile(hwnd uintptr) bool {
	if currentFile == "" {
		return false
	}
	verb := utf16From("open")
	exe := utf16From("explorer.exe")
	params := utf16From(`/select,"` + currentFile + `"`)
	shellExecuteW.Call(
		hwnd,
		uintptr(unsafe.Pointer(&verb[0])),
		uintptr(unsafe.Pointer(&exe[0])),
		uintptr(unsafe.Pointer(&params[0])),
		0,
		1,
	)
	return true
}

func utf16From(s string) []uint16 {
	r, _ := syscall.UTF16FromString(s)
	return r
}

// splashTitle is set before showSplash and used by the WndProc to paint text.
var splashTitle string

func splashWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	const (
		WM_PAINT      = 0x000F
		WM_DESTROY    = 0x0002
		TRANSPARENT   = 1
		DT_CENTER     = 0x01
		DT_VCENTER    = 0x04
		DT_SINGLELINE = 0x20
		DT_NOPREFIX   = 0x0800
	)

	switch msg {
	case WM_PAINT:
		// PAINTSTRUCT is 72 bytes on 64-bit
		var ps [72]byte
		hdc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps[0])))
		if hdc == 0 {
			break
		}

		var rc [16]byte // RECT: 4x int32
		getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc[0])))

		// Fill white background
		whiteBrush, _, _ := getSysColorBrush.Call(0) // COLOR_WINDOW
		fillRect.Call(hdc, uintptr(unsafe.Pointer(&rc[0])), whiteBrush)

		setBkMode.Call(hdc, TRANSPARENT)

		// Large title font
		titleFont, _, _ := createFontW.Call(
			uintptr(uint32(0xFFFFFFD8)), // -40 (40px)
			0, 0, 0,
			700, // bold
			0, 0, 0, 0, 0, 0, 0, 0,
			uintptr(unsafe.Pointer(utf16Ptr("Arial"))),
		)
		oldFont, _, _ := selectObject.Call(hdc, titleFont)
		setTextColor.Call(hdc, 0x00222222) // dark gray

		// Draw "TinyMD" centered, slightly above middle
		titleRC := rc
		// Shift up by 30px: reduce bottom by 60
		bottom := *(*int32)(unsafe.Pointer(&titleRC[12]))
		*(*int32)(unsafe.Pointer(&titleRC[12])) = bottom - 60
		titleText := utf16From("TinyMD")
		drawTextW.Call(hdc, uintptr(unsafe.Pointer(&titleText[0])),
			uintptr(len(titleText)-1), // exclude null terminator
			uintptr(unsafe.Pointer(&titleRC[0])),
			DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX)

		// Small subtitle font
		subFont, _, _ := createFontW.Call(
			uintptr(uint32(0xFFFFFFF2)), // -14 (14px)
			0, 0, 0,
			400, // normal
			0, 0, 0, 0, 0, 0, 0, 0,
			uintptr(unsafe.Pointer(utf16Ptr("Arial"))),
		)
		selectObject.Call(hdc, subFont)
		setTextColor.Call(hdc, 0x00999999) // light gray

		// Draw subtitle centered, slightly below middle
		subRC := rc
		*(*int32)(unsafe.Pointer(&subRC[4])) = *(*int32)(unsafe.Pointer(&subRC[4])) + 30 // shift top down
		subText := utf16From(splashTitle)
		drawTextW.Call(hdc, uintptr(unsafe.Pointer(&subText[0])),
			uintptr(len(subText)-1),
			uintptr(unsafe.Pointer(&subRC[0])),
			DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX)

		selectObject.Call(hdc, oldFont)
		deleteObject.Call(titleFont)
		deleteObject.Call(subFont)
		endPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps[0])))
		return 0
	}

	ret, _, _ := defWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func showSplash(title string) uintptr {
	splashTitle = title

	const (
		WS_OVERLAPPEDWINDOW = 0x00CF0000
		WS_VISIBLE          = 0x10000000
		CW_USEDEFAULT       = 0x80000000
		SM_CXSCREEN         = 0
		SM_CYSCREEN         = 1
		IDC_ARROW           = 32512
	)

	hInstance, _, _ := getModuleHandleW.Call(0)
	cursor, _, _ := loadCursorW.Call(0, IDC_ARROW)

	className := utf16From("TinyMDSplash")

	// WNDCLASSEXW struct (80 bytes on 64-bit)
	var wc [80]byte
	*(*uint32)(unsafe.Pointer(&wc[0])) = 80                                       // cbSize
	*(*uintptr)(unsafe.Pointer(&wc[8])) = syscall.NewCallback(splashWndProc)      // lpfnWndProc
	*(*uintptr)(unsafe.Pointer(&wc[48])) = hInstance                              // hInstance
	*(*uintptr)(unsafe.Pointer(&wc[56])) = cursor                                 // hCursor
	*(*uintptr)(unsafe.Pointer(&wc[64])) = 6                                      // hbrBackground = COLOR_WINDOW+1
	*(*uintptr)(unsafe.Pointer(&wc[72])) = uintptr(unsafe.Pointer(&className[0])) // lpszClassName

	registerClassExW.Call(uintptr(unsafe.Pointer(&wc[0])))

	// Center on screen
	w, h := uintptr(1400), uintptr(900)
	screenW, _, _ := getSystemMetrics.Call(SM_CXSCREEN)
	screenH, _, _ := getSystemMetrics.Call(SM_CYSCREEN)
	x := (screenW - w) / 2
	y := (screenH - h) / 2

	windowTitle := utf16From(title)
	hwnd, _, _ := createWindowExW.Call(
		0, // dwExStyle
		uintptr(unsafe.Pointer(&className[0])),
		uintptr(unsafe.Pointer(&windowTitle[0])),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		x, y, w, h,
		0, 0, hInstance, 0,
	)

	showWindowProc.Call(hwnd, 5) // SW_SHOW
	updateWindowProc.Call(hwnd)

	return hwnd
}

func destroySplash(hwnd uintptr) {
	if hwnd != 0 {
		destroyWindowProc.Call(hwnd)
	}
}

func main() {
	var initialContent string
	autoPrint := false
	selftestOut := ""
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--print" {
			autoPrint = true
		} else if arg == "--selftest" && i+1 < len(args) {
			selftestOut = args[i+1]
			i++
		} else if currentFile == "" && browseRoot == "" {
			info, err := os.Stat(arg)
			if err == nil && info.IsDir() {
				abs, _ := filepath.Abs(arg)
				browseRoot = abs
			} else {
				currentFile = arg
				if content, err := readDocument(currentFile); err == nil {
					initialContent = content
					addRecentLater = true
				}
			}
		}
	}

	loadSettings()
	if addRecentLater {
		addRecent(currentFile)
	}

	// Show a native splash window immediately (~50ms) while WebView2 loads (~2-3s).
	var splash uintptr
	if selftestOut == "" {
		splash = showSplash(windowTitle())
	}

	// Use a fixed data path so WebView2 reuses its cached browser profile
	// instead of recreating it every launch (~1s saving on cold start).
	dataPath := filepath.Join(os.Getenv("LOCALAPPDATA"), "TinyMD", "webview2")

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  dataPath,
		WindowOptions: webview2.WindowOptions{
			Title:  windowTitle(),
			Width:  1400,
			Height: 900,
			Center: true,
		},
	})
	if w == nil {
		destroySplash(splash)
		fmt.Fprintln(os.Stderr, "Failed to create webview2 — is Edge WebView2 Runtime installed?")
		os.Exit(1)
	}
	defer w.Destroy()
	mainHwnd = uintptr(w.Window())
	installMainMenu(w)
	if selftestOut == "" {
		installEditorKeyboardHook(w)
		defer uninstallEditorKeyboardHook()
	} else {
		startSelftest(w, selftestOut)
	}
	bindShell(w)

	// WebView2 is ready — destroy splash so the real window takes over.
	destroySplash(splash)

	// Bind Go functions for JS to call
	w.Bind("goReadClipboard", func() map[string]string {
		text, err := readClipboardText(getHWND())
		if err != nil {
			return map[string]string{"error": err.Error()}
		}
		return map[string]string{"text": text}
	})

	w.Bind("goWriteClipboard", func(text string) string {
		if err := writeClipboardText(getHWND(), text); err != nil {
			return "error: " + err.Error()
		}
		return "ok"
	})

	w.Bind("goSaveFile", func(content string) string {
		if currentFile == "" {
			return "no file"
		}
		err := os.WriteFile(currentFile, documentBytes(content), 0644)
		if err != nil {
			return "error: " + err.Error()
		}
		addRecent(currentFile)
		return "ok"
	})

	w.Bind("goLoadFile", func(path string) map[string]string {
		// Only allow loads under browseRoot to avoid arbitrary file reads from page content.
		if browseRoot == "" {
			return map[string]string{"error": "not in browse mode"}
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return map[string]string{"error": err.Error()}
		}
		if !strings.HasPrefix(strings.ToLower(abs), strings.ToLower(browseRoot)) {
			return map[string]string{"error": "path outside browse root"}
		}
		content, err := readDocument(abs)
		if err != nil {
			return map[string]string{"error": err.Error()}
		}
		currentFile = abs
		dirty = false
		addRecent(abs)
		w.SetTitle(windowTitle())
		return map[string]string{"content": content, "name": filepath.Base(abs)}
	})

	w.Bind("goOpenFile", func() map[string]string {
		path := showOpenDialog(getHWND())
		if path == "" {
			return map[string]string{}
		}
		content, err := readDocument(path)
		if err != nil {
			return map[string]string{"error": err.Error()}
		}
		currentFile = path
		dirty = false
		addRecent(path)
		w.SetTitle(windowTitle())
		return map[string]string{"content": content, "name": filepath.Base(path)}
	})

	w.Bind("goShowSaveDialog", func() string {
		path := showSaveDialog(getHWND(), currentFile)
		if path == "" {
			return ""
		}
		currentFile = path
		w.SetTitle(windowTitle())
		return filepath.Base(path)
	})

	w.Bind("goSaveFileAs", func(content string) string {
		path := showSaveDialog(getHWND(), currentFile)
		if path == "" {
			return ""
		}
		if err := os.WriteFile(path, documentBytes(content), 0644); err != nil {
			return "error: " + err.Error()
		}
		currentFile = path
		addRecent(path)
		w.SetTitle(windowTitle())
		return filepath.Base(path)
	})

	w.Bind("goRevealFile", func() string {
		if revealCurrentFile(getHWND()) {
			return "ok"
		}
		return "no file"
	})

	var tree string
	if browseRoot != "" {
		tree = treeJSON(browseRoot)
	}
	w.SetHtml(htmlPage(initialContent, currentFile, tree))

	if autoPrint {
		// Trigger print after a short delay to let marked.js load
		w.Dispatch(func() {
			w.Eval("setTimeout(function(){ window.print(); }, 500);")
		})
	}

	w.Run()
}

func windowTitle() string {
	name := "Untitled"
	switch {
	case currentFile != "":
		name = currentFile
	case browseRoot != "":
		name = browseRoot
	}
	if dirty {
		// The leading asterisk is the usual Windows sign of unsaved changes.
		return "*" + name + " — TinyMD"
	}
	return name + " — TinyMD"
}

func jsEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
		"</", `<\/`,
	)
	return r.Replace(s)
}

func htmlPage(initialContent, fileName, treeJSONStr string) string {
	if treeJSONStr == "" {
		treeJSONStr = "null"
	}
	return strings.NewReplacer(
		"{{INIT_CONTENT}}", jsEscape(initialContent),
		"{{INIT_FNAME}}", jsEscape(fileName),
		"{{TREE}}", treeJSONStr,
		"{{SETTINGS}}", settingsJSON(),
		"{{MARKED_JS}}", strings.ReplaceAll(markedJS, "</script", "<\\/script"),
		"{{APP_JS}}", strings.ReplaceAll(appJS, "</script", "<\\/script"),
	).Replace(pageHTML)
}

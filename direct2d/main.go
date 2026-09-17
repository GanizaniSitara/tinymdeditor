// TinyMD Direct2D Prototype — native markdown preview using Direct2D + DirectWrite.
//
// Build: GOOS=windows go build -ldflags="-s -w -H windowsgui" -trimpath -o tinymd-d2d.exe .

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// Win32 DLLs and procs
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	d2d1     = syscall.NewLazyDLL("d2d1.dll")
	dwrite   = syscall.NewLazyDLL("dwrite.dll")

	// user32
	registerClassExW     = user32.NewProc("RegisterClassExW")
	createWindowExW      = user32.NewProc("CreateWindowExW")
	showWindowProc       = user32.NewProc("ShowWindow")
	updateWindowProc     = user32.NewProc("UpdateWindow")
	defWindowProcW       = user32.NewProc("DefWindowProcW")
	getSystemMetrics     = user32.NewProc("GetSystemMetrics")
	loadCursorW          = user32.NewProc("LoadCursorW")
	getClientRect        = user32.NewProc("GetClientRect")
	getMessageW          = user32.NewProc("GetMessageW")
	translateMessage     = user32.NewProc("TranslateMessage")
	dispatchMessageW     = user32.NewProc("DispatchMessageW")
	postQuitMessage      = user32.NewProc("PostQuitMessage")
	sendMessageW         = user32.NewProc("SendMessageW")
	moveWindow           = user32.NewProc("MoveWindow")
	invalidateRect       = user32.NewProc("InvalidateRect")
	setFocus             = user32.NewProc("SetFocus")
	setTimer             = user32.NewProc("SetTimer")
	killTimer            = user32.NewProc("KillTimer")
	getWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	getWindowTextW       = user32.NewProc("GetWindowTextW")
	setWindowTextW       = user32.NewProc("SetWindowTextW")
	beginPaint           = user32.NewProc("BeginPaint")
	endPaint             = user32.NewProc("EndPaint")
	setScrollInfo        = user32.NewProc("SetScrollInfo")
	getKeyState          = user32.NewProc("GetKeyState")
	destroyWindowProc    = user32.NewProc("DestroyWindow")
	setCapture           = user32.NewProc("SetCapture")
	releaseCapture       = user32.NewProc("ReleaseCapture")
	setCursorProc        = user32.NewProc("SetCursor")
	getCursorPos         = user32.NewProc("GetCursorPos")
	screenToClient       = user32.NewProc("ScreenToClient")
	createMenu           = user32.NewProc("CreateMenu")
	createPopupMenu      = user32.NewProc("CreatePopupMenu")
	appendMenuW          = user32.NewProc("AppendMenuW")
	setMenu              = user32.NewProc("SetMenu")
	drawMenuBar          = user32.NewProc("DrawMenuBar")
	messageBoxW          = user32.NewProc("MessageBoxW")
	openClipboard        = user32.NewProc("OpenClipboard")
	closeClipboard       = user32.NewProc("CloseClipboard")
	emptyClipboard       = user32.NewProc("EmptyClipboard")
	setClipboardData     = user32.NewProc("SetClipboardData")

	// gdi32
	createFontW  = gdi32.NewProc("CreateFontW")
	deleteObject = gdi32.NewProc("DeleteObject")

	// kernel32
	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
	globalFree       = kernel32.NewProc("GlobalFree")

	// comdlg32
	getOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
	getSaveFileNameW = comdlg32.NewProc("GetSaveFileNameW")
	printDlgW        = comdlg32.NewProc("PrintDlgW")

	// shell32
	shellExecuteW = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

	// gdi32 — printing
	startDocW             = gdi32.NewProc("StartDocW")
	endDoc                = gdi32.NewProc("EndDoc")
	startPage             = gdi32.NewProc("StartPage")
	endPage               = gdi32.NewProc("EndPage")
	getDeviceCaps         = gdi32.NewProc("GetDeviceCaps")
	deleteDC              = gdi32.NewProc("DeleteDC")
	selectObject          = gdi32.NewProc("SelectObject")
	setBkMode             = gdi32.NewProc("SetBkMode")
	setTextColor          = gdi32.NewProc("SetTextColor")
	drawTextW             = user32.NewProc("DrawTextW")
	createSolidBrush      = gdi32.NewProc("CreateSolidBrush")
	fillRect              = user32.NewProc("FillRect")
	textOutW              = gdi32.NewProc("TextOutW")
	getTextExtentPoint32W = gdi32.NewProc("GetTextExtentPoint32W")
	createPen             = gdi32.NewProc("CreatePen")
	moveToEx              = gdi32.NewProc("MoveToEx")
	lineTo                = gdi32.NewProc("LineTo")
	ellipseProc           = gdi32.NewProc("Ellipse")

	// ole32
	coInitializeEx = ole32.NewProc("CoInitializeEx")

	// d2d1
	d2d1CreateFactory = d2d1.NewProc("D2D1CreateFactory")

	// dwrite
	dwriteCreateFactory = dwrite.NewProc("DWriteCreateFactory")
)

// Win32 constants
const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_VSCROLL          = 0x00200000
	WS_CLIPCHILDREN     = 0x02000000
	WS_EX_CLIENTEDGE    = 0x00000200
	ES_MULTILINE        = 0x0004
	ES_AUTOVSCROLL      = 0x0040
	ES_WANTRETURN       = 0x1000
	SM_CXSCREEN         = 0
	SM_CYSCREEN         = 1
	IDC_ARROW           = 32512

	WM_CREATE     = 0x0001
	WM_DESTROY    = 0x0002
	WM_SIZE       = 0x0005
	WM_PAINT      = 0x000F
	WM_COMMAND    = 0x0111
	WM_TIMER      = 0x0113
	WM_KEYDOWN    = 0x0100
	WM_MOUSEWHEEL = 0x020A

	WM_SETCURSOR      = 0x0020
	WM_MOUSEMOVE      = 0x0200
	WM_LBUTTONDOWN    = 0x0201
	WM_LBUTTONUP      = 0x0202
	WM_LBUTTONDBLCLK  = 0x0203
	WM_CAPTURECHANGED = 0x0215
	HTCLIENT          = 1
	IDC_SIZEWE        = 32644
	CS_DBLCLKS        = 0x0008
	SW_HIDE           = 0
	SW_SHOWNOACTIVATE = 4
	WM_VSCROLL        = 0x0115
	WM_SETFONT        = 0x0030
	WM_SETFOCUS       = 0x0007
	WM_ERASEBKGND     = 0x0014
	WM_SETTEXT        = 0x000C
	WM_CUT            = 0x0300
	WM_COPY           = 0x0301
	WM_PASTE          = 0x0302
	WM_UNDO           = 0x0304

	EN_CHANGE  = 0x0300
	VK_A       = 0x41
	VK_C       = 0x43
	VK_V       = 0x56
	VK_X       = 0x58
	VK_S       = 0x53
	VK_P       = 0x50
	VK_O       = 0x4F
	VK_E       = 0x45
	VK_0       = 0x30
	VK_1       = 0x31
	VK_2       = 0x32
	VK_SHIFT   = 0x10
	VK_CONTROL = 0x11

	EM_SETSEL = 0x00B1

	mfString    = 0x0000
	mfPopup     = 0x0010
	mfSeparator = 0x0800
	mbOk        = 0x0000
	mbInfo      = 0x0040
	mbIconError = 0x0010

	cfUnicodeText = 13
	gmemMoveable  = 0x0002

	idFileOpen     = 1001
	idFileSave     = 1002
	idFileSaveAs   = 1003
	idFilePrint    = 1004
	idFileReveal   = 1005
	idFileExit     = 1006
	idFileCopyPath = 1007
	idEditUndo     = 1101
	idEditCut      = 1102
	idEditCopy     = 1103
	idEditPaste    = 1104
	idEditAll      = 1105
	idViewEditor   = 1301
	idViewPreview  = 1302
	idViewSplit    = 1303
	idHelpAbout    = 1201

	SB_VERT          = 1
	SIF_RANGE        = 0x01
	SIF_PAGE         = 0x02
	SIF_POS          = 0x04
	SB_LINEUP        = 0
	SB_LINEDOWN      = 1
	SB_PAGEUP        = 2
	SB_PAGEDOWN      = 3
	SB_THUMBTRACK    = 5
	SB_THUMBPOSITION = 4

	TIMER_DEBOUNCE = 1

	// COM
	COINIT_APARTMENTTHREADED = 0x2

	// D2D1
	D2D1_FACTORY_TYPE_SINGLE_THREADED = 0

	// DWRITE
	DWRITE_FACTORY_TYPE_SHARED      = 0
	DWRITE_FONT_WEIGHT_NORMAL       = 400
	DWRITE_FONT_WEIGHT_BOLD         = 700
	DWRITE_FONT_STYLE_NORMAL        = 0
	DWRITE_FONT_STYLE_ITALIC        = 1
	DWRITE_FONT_STRETCH_NORMAL      = 5
	DWRITE_TEXT_ALIGNMENT_LEADING   = 0
	DWRITE_PARAGRAPH_ALIGNMENT_NEAR = 0
	DWRITE_WORD_WRAPPING_WRAP       = 0

	// Printing
	PD_RETURNDC                   = 0x00000100
	PD_USEDEVMODECOPIESANDCOLLATE = 0x00040000
	LOGPIXELSX                    = 88
	LOGPIXELSY                    = 90
	HORZRES                       = 8
	VERTRES                       = 10
	TRANSPARENT_BK                = 1
	DT_WORDBREAK                  = 0x0010
	DT_NOPREFIX                   = 0x0800
	DT_CALCRECT                   = 0x0400
	DT_EXPANDTABS                 = 0x0040
	DT_EDITCONTROL                = 0x2000
	DT_SINGLELINE                 = 0x0020
	DT_NOCLIP                     = 0x0100
)

// GUIDs
var (
	IID_ID2D1Factory   = guid(0x06152247, 0x6f50, 0x465a, [8]byte{0x92, 0x45, 0x11, 0x8b, 0xfd, 0x3b, 0x60, 0x07})
	IID_IDWriteFactory = guid(0xb859ee5a, 0xd838, 0x4b5b, [8]byte{0xa2, 0xe8, 0x1a, 0xdc, 0x7d, 0x93, 0xdb, 0x48})
)

func guid(d1 uint32, d2, d3 uint16, d4 [8]byte) [16]byte {
	var g [16]byte
	*(*uint32)(unsafe.Pointer(&g[0])) = d1
	*(*uint16)(unsafe.Pointer(&g[4])) = d2
	*(*uint16)(unsafe.Pointer(&g[6])) = d3
	copy(g[8:], d4[:])
	return g
}

// Helpers
func utf16From(s string) []uint16 {
	r, _ := syscall.UTF16FromString(s)
	return r
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func loword(v uintptr) uint16 { return uint16(v & 0xFFFF) }
func hiword(v uintptr) uint16 { return uint16((v >> 16) & 0xFFFF) }

func installMainMenu(hwnd uintptr) {
	menu, _, _ := createMenu.Call()
	fileMenu, _, _ := createPopupMenu.Call()
	editMenu, _, _ := createPopupMenu.Call()
	viewMenu, _, _ := createPopupMenu.Call()
	helpMenu, _, _ := createPopupMenu.Call()

	appendMenu(fileMenu, mfString, idFileOpen, "&Open...\tCtrl+O")
	appendMenu(fileMenu, mfString, idFileSave, "&Save\tCtrl+S")
	appendMenu(fileMenu, mfString, idFileSaveAs, "Save &As...\tCtrl+Shift+S")
	appendMenu(fileMenu, mfString, idFilePrint, "&Print...\tCtrl+P")
	appendMenu(fileMenu, mfSeparator, 0, "")
	appendMenu(fileMenu, mfString, idFileReveal, "Show in &Folder\tCtrl+E")
	appendMenu(fileMenu, mfString, idFileCopyPath, "Copy File &Path\tCtrl+Shift+C")
	appendMenu(fileMenu, mfSeparator, 0, "")
	appendMenu(fileMenu, mfString, idFileExit, "E&xit")

	appendMenu(editMenu, mfString, idEditUndo, "&Undo\tCtrl+Z")
	appendMenu(editMenu, mfSeparator, 0, "")
	appendMenu(editMenu, mfString, idEditCut, "Cu&t\tCtrl+X")
	appendMenu(editMenu, mfString, idEditCopy, "&Copy\tCtrl+C")
	appendMenu(editMenu, mfString, idEditPaste, "&Paste\tCtrl+V")
	appendMenu(editMenu, mfSeparator, 0, "")
	appendMenu(editMenu, mfString, idEditAll, "Select &All\tCtrl+A")

	appendMenu(viewMenu, mfString, idViewEditor, "&Editor only\tCtrl+1")
	appendMenu(viewMenu, mfString, idViewPreview, "&Preview only\tCtrl+2")
	appendMenu(viewMenu, mfString, idViewSplit, "&Split evenly\tCtrl+0")

	appendMenu(helpMenu, mfString, idHelpAbout, "&About TinyMD")

	appendMenu(menu, mfPopup, fileMenu, "&File")
	appendMenu(menu, mfPopup, editMenu, "&Edit")
	appendMenu(menu, mfPopup, viewMenu, "&View")
	appendMenu(menu, mfPopup, helpMenu, "&Help")
	setMenu.Call(hwnd, menu)
	drawMenuBar.Call(hwnd)
}

func appendMenu(menu uintptr, flags uintptr, id uintptr, text string) {
	if flags&mfSeparator != 0 {
		appendMenuW.Call(menu, flags, 0, 0)
		return
	}
	t := utf16From(text)
	appendMenuW.Call(menu, flags, id, uintptr(unsafe.Pointer(&t[0])))
}

func handleMenuCommand(id uintptr) bool {
	switch id {
	case idFileOpen:
		openFile()
	case idFileSave:
		saveFile()
	case idFileSaveAs:
		saveFileAs()
	case idFilePrint:
		printFormatted()
	case idFileReveal:
		openInFolder()
	case idFileCopyPath:
		copyCurrentFilePath()
	case idFileExit:
		destroyWindowProc.Call(mainHwnd)
	case idEditUndo:
		sendMessageW.Call(editorHwnd, WM_UNDO, 0, 0)
		updatePreview()
	case idEditCut:
		sendMessageW.Call(editorHwnd, WM_CUT, 0, 0)
		updatePreview()
	case idEditCopy:
		sendMessageW.Call(editorHwnd, WM_COPY, 0, 0)
	case idEditPaste:
		sendMessageW.Call(editorHwnd, WM_PASTE, 0, 0)
		updatePreview()
	case idEditAll:
		sendMessageW.Call(editorHwnd, EM_SETSEL, 0, ^uintptr(0))
	case idViewEditor:
		toggleCollapse(false) // collapse the preview, or bring it back
	case idViewPreview:
		toggleCollapse(true) // collapse the editor, or bring it back
	case idViewSplit:
		setSplitRatio(0.5)
	case idHelpAbout:
		showAboutDialog()
	default:
		return false
	}
	return true
}

func showAboutDialog() {
	text := utf16From("TinyMD Direct2D\nMarkdown editor for Windows.")
	title := utf16From("About TinyMD")
	messageBoxW.Call(mainHwnd, uintptr(unsafe.Pointer(&text[0])), uintptr(unsafe.Pointer(&title[0])), mbOk|mbInfo)
}

// COM vtable call helper
func comCall(obj uintptr, methodIndex int, args ...uintptr) uintptr {
	vtable := *(*uintptr)(unsafe.Pointer(obj))
	method := *(*uintptr)(unsafe.Pointer(vtable + uintptr(methodIndex)*unsafe.Sizeof(uintptr(0))))

	allArgs := make([]uintptr, 0, 1+len(args))
	allArgs = append(allArgs, obj) // this pointer
	allArgs = append(allArgs, args...)

	var ret uintptr
	switch len(allArgs) {
	case 1:
		ret, _, _ = syscall.Syscall(method, 1, allArgs[0], 0, 0)
	case 2:
		ret, _, _ = syscall.Syscall(method, 2, allArgs[0], allArgs[1], 0)
	case 3:
		ret, _, _ = syscall.Syscall(method, 3, allArgs[0], allArgs[1], allArgs[2])
	case 4:
		ret, _, _ = syscall.Syscall6(method, 4, allArgs[0], allArgs[1], allArgs[2], allArgs[3], 0, 0)
	case 5:
		ret, _, _ = syscall.Syscall6(method, 5, allArgs[0], allArgs[1], allArgs[2], allArgs[3], allArgs[4], 0)
	case 6:
		ret, _, _ = syscall.Syscall6(method, 6, allArgs[0], allArgs[1], allArgs[2], allArgs[3], allArgs[4], allArgs[5])
	case 7:
		ret, _, _ = syscall.Syscall9(method, 7, allArgs[0], allArgs[1], allArgs[2], allArgs[3], allArgs[4], allArgs[5], allArgs[6], 0, 0)
	case 8:
		ret, _, _ = syscall.Syscall9(method, 8, allArgs[0], allArgs[1], allArgs[2], allArgs[3], allArgs[4], allArgs[5], allArgs[6], allArgs[7], 0)
	case 9:
		ret, _, _ = syscall.Syscall9(method, 9, allArgs[0], allArgs[1], allArgs[2], allArgs[3], allArgs[4], allArgs[5], allArgs[6], allArgs[7], allArgs[8])
	case 10:
		ret, _, _ = syscall.Syscall12(method, 10, allArgs[0], allArgs[1], allArgs[2], allArgs[3], allArgs[4], allArgs[5], allArgs[6], allArgs[7], allArgs[8], allArgs[9], 0, 0)
	case 11:
		ret, _, _ = syscall.Syscall12(method, 11, allArgs[0], allArgs[1], allArgs[2], allArgs[3], allArgs[4], allArgs[5], allArgs[6], allArgs[7], allArgs[8], allArgs[9], allArgs[10], 0)
	case 12:
		ret, _, _ = syscall.Syscall12(method, 12, allArgs[0], allArgs[1], allArgs[2], allArgs[3], allArgs[4], allArgs[5], allArgs[6], allArgs[7], allArgs[8], allArgs[9], allArgs[10], allArgs[11])
	}
	return ret
}

func comRelease(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2) // IUnknown::Release is vtable index 2
	}
}

// Layout types for markdown rendering
const (
	blockHeading = iota
	blockParagraph
	blockCode
	blockQuote
	blockListItem
	blockHR
	blockTable
)

type TableCell struct {
	Text string
	Bold bool
}
type TableData struct {
	Headers []TableCell
	Rows    [][]TableCell
	// Aligns holds the per-column alignment from the delimiter row, as a DirectWrite
	// DWRITE_TEXT_ALIGNMENT value: 0 leading, 1 trailing, 2 centre.
	Aligns []uint32

	// cached holds the table measured for one pane width, so repaints and scrolling
	// do not re-measure every cell.
	cached *tableGeometry
}

// textAlignmentFor maps a Markdown column alignment onto the DirectWrite value.
func textAlignmentFor(alignment extast.Alignment) uint32 {
	switch alignment {
	case extast.AlignRight:
		return 1 // DWRITE_TEXT_ALIGNMENT_TRAILING
	case extast.AlignCenter:
		return 2 // DWRITE_TEXT_ALIGNMENT_CENTER
	default:
		return 0 // DWRITE_TEXT_ALIGNMENT_LEADING
	}
}

type InlineSpan struct {
	Start  int
	Length int
	Bold   bool
	Italic bool
	Code   bool
}

type LayoutBlock struct {
	Type       int
	Text       string
	FontSize   float32
	Bold       bool
	Indent     float32
	SpaceAbove float32
	Color      uint32 // ARGB
	BgColor    uint32 // ARGB, 0 = none
	BarColor   uint32 // ARGB, 0 = none
	Table      *TableData
	Spans      []InlineSpan
	Y          float32 // computed during layout
	Height     float32 // measured height

	// A block's text layout only changes when the text or the pane width changes,
	// so it is built once and kept. Rebuilding every block on every repaint made
	// scrolling and typing cost tens of milliseconds on a document of any size.
	layout      uintptr
	layoutWidth float32
}

// releaseLayout drops a block's cached text layout, and a table's measured cells.
func (b *LayoutBlock) releaseLayout() {
	if b.layout != 0 {
		comRelease(b.layout)
		b.layout = 0
		b.layoutWidth = 0
	}
	if b.Table != nil && b.Table.cached != nil {
		b.Table.cached.release()
		b.Table.cached = nil
	}
}

// releaseBlockLayouts drops every cached layout, for when the document is replaced.
func releaseBlockLayouts(blocks []LayoutBlock) {
	for i := range blocks {
		blocks[i].releaseLayout()
	}
}

// D2D resources (created when render target exists)
type d2dResources struct {
	factory      uintptr // ID2D1Factory
	renderTarget uintptr // ID2D1HwndRenderTarget
	dwFactory    uintptr // IDWriteFactory

	// Text formats
	fmtH1   uintptr // IDWriteTextFormat
	fmtH2   uintptr
	fmtH3   uintptr
	fmtBody uintptr
	fmtCode uintptr

	// Brushes
	brushText   uintptr // ID2D1SolidColorBrush
	brushGray   uintptr
	brushBlue   uintptr
	brushCodeBg uintptr
	brushHR     uintptr
	brushWhite  uintptr
}

var res d2dResources

func floatBits(f float32) uintptr {
	return uintptr(math.Float32bits(f))
}

// Pack D2D1_POINT_2F as value (8-byte structs are passed by value on x64)
func packPoint2F(x, y float32) uintptr {
	return uintptr(uint64(math.Float32bits(x)) | (uint64(math.Float32bits(y)) << 32))
}

func createTextFormat(dwFactory uintptr, family string, weight, style, stretch uint32, size float32) uintptr {
	familyU := utf16From(family)
	localeU := utf16From("en-us")
	var fmt uintptr
	comCall(dwFactory, 15, // IDWriteFactory::CreateTextFormat
		uintptr(unsafe.Pointer(&familyU[0])),
		0, // font collection (nil = system)
		uintptr(weight),
		uintptr(style),
		uintptr(stretch),
		floatBits(size),
		uintptr(unsafe.Pointer(&localeU[0])),
		uintptr(unsafe.Pointer(&fmt)),
	)
	if fmt != 0 {
		// Set word wrapping
		comCall(fmt, 5, uintptr(DWRITE_WORD_WRAPPING_WRAP)) // SetWordWrapping
	}
	return fmt
}

func createBrush(rt uintptr, r, g, b, a float32) uintptr {
	color := [4]float32{r, g, b, a}
	var brush uintptr
	comCall(rt, 8, // ID2D1RenderTarget::CreateSolidColorBrush
		uintptr(unsafe.Pointer(&color[0])),
		0,
		uintptr(unsafe.Pointer(&brush)),
	)
	return brush
}

func initD2D(hwnd uintptr) bool {
	// Initialize COM
	coInitializeEx.Call(0, COINIT_APARTMENTTHREADED)

	// Create D2D1 factory
	iid := IID_ID2D1Factory
	hr, _, _ := d2d1CreateFactory.Call(
		D2D1_FACTORY_TYPE_SINGLE_THREADED,
		uintptr(unsafe.Pointer(&iid[0])),
		0,
		uintptr(unsafe.Pointer(&res.factory)),
	)
	if hr != 0 || res.factory == 0 {
		return false
	}

	// Create DWrite factory
	iid2 := IID_IDWriteFactory
	hr, _, _ = dwriteCreateFactory.Call(
		DWRITE_FACTORY_TYPE_SHARED,
		uintptr(unsafe.Pointer(&iid2[0])),
		uintptr(unsafe.Pointer(&res.dwFactory)),
	)
	if hr != 0 || res.dwFactory == 0 {
		return false
	}

	// Don't create render target here — window has 0x0 size during WM_CREATE.
	// renderPreview() will lazily create it on first WM_PAINT when window is sized.
	return true
}

func createDeviceResources(hwnd uintptr) bool {
	if res.renderTarget != 0 {
		return true
	}

	var rc [16]byte
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc[0])))
	w := *(*int32)(unsafe.Pointer(&rc[8]))
	h := *(*int32)(unsafe.Pointer(&rc[12]))

	// D2D1_RENDER_TARGET_PROPERTIES (default)
	var rtProps [28]byte // type(4)+pixelFormat(8)+dpiX(4)+dpiY(4)+usage(4)+minLevel(4)

	// D2D1_HWND_RENDER_TARGET_PROPERTIES
	var hwndProps [20]byte // hwnd(8)+size(8)+presentOptions(4)
	*(*uintptr)(unsafe.Pointer(&hwndProps[0])) = hwnd
	*(*uint32)(unsafe.Pointer(&hwndProps[8])) = uint32(w)
	*(*uint32)(unsafe.Pointer(&hwndProps[12])) = uint32(h)
	// D2D1_PRESENT_OPTIONS_IMMEDIATELY. Without it EndDraw waits for the display's
	// next vertical blank, which put a fixed ~25ms on every repaint whatever the
	// document contained, and that wait is what made typing and scrolling feel slow.
	*(*uint32)(unsafe.Pointer(&hwndProps[16])) = 2

	// ID2D1Factory::CreateHwndRenderTarget is vtable index 14
	hr := comCall(res.factory, 14,
		uintptr(unsafe.Pointer(&rtProps[0])),
		uintptr(unsafe.Pointer(&hwndProps[0])),
		uintptr(unsafe.Pointer(&res.renderTarget)),
	)
	if hr != 0 || res.renderTarget == 0 {
		return false
	}

	// Create text formats
	res.fmtH1 = createTextFormat(res.dwFactory, "Segoe UI", DWRITE_FONT_WEIGHT_BOLD, DWRITE_FONT_STYLE_NORMAL, DWRITE_FONT_STRETCH_NORMAL, 28)
	res.fmtH2 = createTextFormat(res.dwFactory, "Segoe UI", DWRITE_FONT_WEIGHT_BOLD, DWRITE_FONT_STYLE_NORMAL, DWRITE_FONT_STRETCH_NORMAL, 22)
	res.fmtH3 = createTextFormat(res.dwFactory, "Segoe UI", DWRITE_FONT_WEIGHT_BOLD, DWRITE_FONT_STYLE_NORMAL, DWRITE_FONT_STRETCH_NORMAL, 18)
	res.fmtBody = createTextFormat(res.dwFactory, "Segoe UI", DWRITE_FONT_WEIGHT_NORMAL, DWRITE_FONT_STYLE_NORMAL, DWRITE_FONT_STRETCH_NORMAL, 14)
	res.fmtCode = createTextFormat(res.dwFactory, "Consolas", DWRITE_FONT_WEIGHT_NORMAL, DWRITE_FONT_STYLE_NORMAL, DWRITE_FONT_STRETCH_NORMAL, 13)
	// Create brushes
	res.brushText = createBrush(res.renderTarget, 0, 0, 0, 1)            // pure black text
	res.brushGray = createBrush(res.renderTarget, 0.4, 0.4, 0.4, 1)      // gray
	res.brushBlue = createBrush(res.renderTarget, 0.01, 0.4, 0.84, 1)    // link blue
	res.brushCodeBg = createBrush(res.renderTarget, 0.94, 0.94, 0.94, 1) // code bg
	res.brushHR = createBrush(res.renderTarget, 0.78, 0.78, 0.78, 1)     // hr line (darker gray for visibility)
	res.brushWhite = createBrush(res.renderTarget, 1, 1, 1, 1)           // white bg
	return true
}

func discardDeviceResources() {
	for _, p := range []*uintptr{
		&res.brushText, &res.brushGray, &res.brushBlue,
		&res.brushCodeBg, &res.brushHR, &res.brushWhite,
		&res.fmtH1, &res.fmtH2, &res.fmtH3, &res.fmtBody, &res.fmtCode,
		&res.renderTarget,
	} {
		if *p != 0 {
			comRelease(*p)
			*p = 0
		}
	}
}

// Markdown → layout blocks
func markdownToLayout(source []byte) []LayoutBlock {
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	var blocks []LayoutBlock
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindHeading:
			h := n.(*ast.Heading)
			txt := extractInlineText(n, source)
			var fs float32
			var sa float32
			switch h.Level {
			case 1:
				fs = 28
				sa = 24
			case 2:
				fs = 22
				sa = 20
			default:
				fs = 18
				sa = 16
			}
			blocks = append(blocks, LayoutBlock{
				Type: blockHeading, Text: txt, FontSize: fs, Bold: true,
				SpaceAbove: sa, Color: 0xFF222222,
			})
			return ast.WalkSkipChildren, nil

		case ast.KindParagraph:
			if n.Parent() != nil && n.Parent().Kind() == ast.KindBlockquote {
				return ast.WalkContinue, nil
			}
			txt, spans := extractInlineTextWithSpans(n, source)
			if txt != "" {
				blocks = append(blocks, LayoutBlock{
					Type: blockParagraph, Text: txt, FontSize: 14,
					SpaceAbove: 12, Color: 0xFF222222, Spans: spans,
				})
			}
			return ast.WalkSkipChildren, nil

		case ast.KindFencedCodeBlock, ast.KindCodeBlock:
			var buf strings.Builder
			lines := n.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				buf.Write(seg.Value(source))
			}
			blocks = append(blocks, LayoutBlock{
				Type: blockCode, Text: strings.TrimRight(buf.String(), "\n"),
				FontSize: 13, SpaceAbove: 12,
				Color: 0xFF333333, BgColor: 0xFFF0F0F0,
			})
			return ast.WalkSkipChildren, nil

		case ast.KindBlockquote:
			txt, spans := extractInlineTextWithSpans(n, source)
			blocks = append(blocks, LayoutBlock{
				Type: blockQuote, Text: txt, FontSize: 14,
				Indent: 30, SpaceAbove: 12,
				Color: 0xFF666666, BarColor: 0xFF0366D6, Spans: spans,
			})
			return ast.WalkSkipChildren, nil

		case ast.KindListItem:
			txt, spans := extractInlineTextWithSpans(n, source)
			blocks = append(blocks, LayoutBlock{
				Type: blockListItem, Text: txt, FontSize: 14,
				Indent: 30, SpaceAbove: 6, Color: 0xFF222222, Spans: spans,
			})
			return ast.WalkSkipChildren, nil

		case ast.KindThematicBreak:
			blocks = append(blocks, LayoutBlock{
				Type: blockHR, SpaceAbove: 12,
			})
			return ast.WalkSkipChildren, nil
		}

		if n.Kind() == extast.KindTable {
			td := &TableData{}
			if table, ok := n.(*extast.Table); ok {
				for _, alignment := range table.Alignments {
					td.Aligns = append(td.Aligns, textAlignmentFor(alignment))
				}
			}
			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				if child.Kind() == extast.KindTableHeader {
					for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
						if cell.Kind() == extast.KindTableCell {
							td.Headers = append(td.Headers, TableCell{
								Text: extractInlineText(cell, source),
								Bold: true,
							})
						}
					}
				} else if child.Kind() == extast.KindTableRow {
					var cells []TableCell
					for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
						if cell.Kind() == extast.KindTableCell {
							cells = append(cells, TableCell{
								Text: extractInlineText(cell, source),
								Bold: false,
							})
						}
					}
					td.Rows = append(td.Rows, cells)
				}
			}
			blocks = append(blocks, LayoutBlock{
				Type:       blockTable,
				Table:      td,
				SpaceAbove: 12,
			})
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})
	return blocks
}

func extractInlineText(n ast.Node, source []byte) string {
	var buf strings.Builder
	ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c.Kind() {
		case ast.KindText:
			t := c.(*ast.Text)
			buf.Write(t.Segment.Value(source))
			if t.SoftLineBreak() {
				buf.WriteByte(' ')
			}
		case ast.KindCodeSpan:
			for gc := c.FirstChild(); gc != nil; gc = gc.NextSibling() {
				if gc.Kind() == ast.KindText {
					buf.Write(gc.(*ast.Text).Segment.Value(source))
				}
			}
			return ast.WalkSkipChildren, nil
		case ast.KindString:
			buf.Write(c.(*ast.String).Value)
		}
		return ast.WalkContinue, nil
	})
	return buf.String()
}

func extractInlineTextWithSpans(n ast.Node, source []byte) (string, []InlineSpan) {
	var buf strings.Builder
	var spans []InlineSpan

	ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c.Kind() {
		case ast.KindText:
			t := c.(*ast.Text)
			text := string(t.Segment.Value(source))
			// Check if any parent is emphasis
			bold := false
			italic := false
			for p := c.Parent(); p != nil && p != n; p = p.Parent() {
				if p.Kind() == ast.KindEmphasis {
					em := p.(*ast.Emphasis)
					if em.Level == 2 {
						bold = true
					} else {
						italic = true
					}
				}
			}
			if bold || italic {
				spans = append(spans, InlineSpan{
					Start: buf.Len(), Length: len(text),
					Bold: bold, Italic: italic,
				})
			}
			buf.WriteString(text)
			if t.SoftLineBreak() {
				buf.WriteByte(' ')
			}
		case ast.KindCodeSpan:
			start := buf.Len()
			for gc := c.FirstChild(); gc != nil; gc = gc.NextSibling() {
				if gc.Kind() == ast.KindText {
					buf.Write(gc.(*ast.Text).Segment.Value(source))
				}
			}
			spans = append(spans, InlineSpan{
				Start: start, Length: buf.Len() - start, Code: true,
			})
			return ast.WalkSkipChildren, nil
		case ast.KindString:
			buf.Write(c.(*ast.String).Value)
		case ast.KindEmphasis:
			// Don't write anything, just let children be processed
			return ast.WalkContinue, nil
		}
		return ast.WalkContinue, nil
	})
	return buf.String(), spans
}

// Pack DWRITE_TEXT_RANGE (startPosition uint32 + length uint32) as 8 bytes by value on x64
func packTextRange(start, length uint32) uintptr {
	return uintptr(uint64(start) | (uint64(length) << 32))
}

// Global state
var (
	hInstance   uintptr
	mainHwnd    uintptr
	editorHwnd  uintptr
	previewHwnd uintptr
	editorFont  uintptr
	currentFile string
	lineEnding  = "\n"

	currentBlocks []LayoutBlock
	scrollY       float32
	totalHeight   float32
)

func getTextFormat(b *LayoutBlock) uintptr {
	if b.Type == blockCode {
		return res.fmtCode
	}
	if b.Bold {
		switch {
		case b.FontSize >= 28:
			return res.fmtH1
		case b.FontSize >= 22:
			return res.fmtH2
		default:
			return res.fmtH3
		}
	}
	return res.fmtBody
}

func getBrush(b *LayoutBlock) uintptr {
	if b.Color == 0xFF666666 {
		return res.brushGray
	}
	return res.brushText
}

// fitColumnsToWidth shrinks table columns in place so the table fits the pane.
// Columns are measured at the width they would like to be; left alone, a table of
// prose runs off the right edge and the text is clipped. Columns already narrower
// than an equal share keep their measured width, and the columns over that share
// divide what is left in proportion to what they asked for, so a one-word column
// stays narrow and the prose column gives up the space.
func fitColumnsToWidth(colWidths []float32, available, cellPad float32) {
	if len(colWidths) == 0 || available <= 0 {
		return
	}
	var total float32
	for _, w := range colWidths {
		total += w
	}
	if total <= available {
		return
	}

	minWidth := cellPad*2 + 24
	if evenShare := available / float32(len(colWidths)); minWidth > evenShare {
		minWidth = evenShare
	}

	settled := make([]bool, len(colWidths))
	for {
		var settledWidth, flexibleWidth float32
		flexible := 0
		for i, w := range colWidths {
			if settled[i] {
				settledWidth += w
				continue
			}
			flexibleWidth += w
			flexible++
		}
		if flexible == 0 {
			break
		}

		budget := available - settledWidth
		if budget <= 0 {
			for i := range colWidths {
				if !settled[i] {
					colWidths[i] = minWidth
				}
			}
			break
		}

		// Anything under the fair cut is not the problem; leave it be and re-cut
		// what remains among the columns that are still too wide.
		share := budget / float32(flexible)
		narrowed := false
		for i, w := range colWidths {
			if !settled[i] && w <= share {
				settled[i] = true
				narrowed = true
			}
		}
		if narrowed {
			continue
		}

		scale := budget / flexibleWidth
		for i := range colWidths {
			if settled[i] {
				continue
			}
			colWidths[i] *= scale
			if colWidths[i] < minWidth {
				colWidths[i] = minWidth
			}
		}
		break
	}

	// Clamping to a minimum can push the total back over; squeeze the excess out.
	total = 0
	for _, w := range colWidths {
		total += w
	}
	if total > available {
		scale := available / total
		for i := range colWidths {
			colWidths[i] *= scale
		}
	}
}

const tableCellPad = float32(8)

// tableGeometry is a table measured for one pane width: the column positions and,
// for each row, its height and a text layout per cell. Measuring a table means
// creating a layout for every cell, so it is done once per width rather than on
// every repaint.
type tableGeometry struct {
	width      float32 // the drawWidth it was measured for
	colWidths  []float32
	colOffsets []float32
	tableWidth float32
	rows       []tableRowGeometry
}

type tableRowGeometry struct {
	height float32
	cells  []uintptr // cached text layouts, one per column
}

func (g *tableGeometry) release() {
	for _, row := range g.rows {
		for _, layout := range row.cells {
			comRelease(layout)
		}
	}
	g.rows = nil
}

// cellLayout builds the text layout for one cell at a given width.
func cellLayout(cell TableCell, width float32, align uint32) uintptr {
	text := utf16From(cell.Text)
	length := uint32(len(text) - 1)
	if length == 0 {
		text = utf16From(" ")
		length = 1
	}
	format := res.fmtBody
	if cell.Bold {
		format = res.fmtH3
	}
	var layout uintptr
	// IDWriteFactory::CreateTextLayout is vtable index 18
	comCall(res.dwFactory, 18,
		uintptr(unsafe.Pointer(&text[0])),
		uintptr(length),
		format,
		floatBits(width),
		floatBits(10000),
		uintptr(unsafe.Pointer(&layout)),
	)
	if layout != 0 && align != 0 {
		// IDWriteTextFormat::SetTextAlignment is vtable index 3
		comCall(layout, 3, uintptr(align))
	}
	return layout
}

// layoutHeight reads the measured height of a text layout.
func layoutHeight(layout uintptr) float32 {
	if layout == 0 {
		return 0
	}
	var metrics [36]byte
	comCall(layout, 60, uintptr(unsafe.Pointer(&metrics[0])))
	return *(*float32)(unsafe.Pointer(&metrics[16]))
}

// layoutWidthOf reads the measured width of a text layout, trailing whitespace included.
func layoutWidthOf(layout uintptr) float32 {
	if layout == 0 {
		return 0
	}
	var metrics [36]byte
	comCall(layout, 60, uintptr(unsafe.Pointer(&metrics[0])))
	return *(*float32)(unsafe.Pointer(&metrics[12]))
}

// geometry returns the table measured for the given pane width, reusing the previous
// measurement when the width has not changed.
func (td *TableData) geometry(drawWidth float32) *tableGeometry {
	if td.cached != nil && td.cached.width == drawWidth {
		return td.cached
	}
	if td.cached != nil {
		td.cached.release()
	}

	numCols := len(td.Headers)
	rows := make([][]TableCell, 0, len(td.Rows)+1)
	rows = append(rows, td.Headers)
	rows = append(rows, td.Rows...)

	// What each column would like to be, measured unconstrained.
	colWidths := make([]float32, numCols)
	for _, cells := range rows {
		for ci := 0; ci < numCols && ci < len(cells); ci++ {
			measure := cellLayout(cells[ci], 10000, 0)
			if measure == 0 {
				continue
			}
			if want := layoutWidthOf(measure) + tableCellPad*2; want > colWidths[ci] {
				colWidths[ci] = want
			}
			comRelease(measure)
		}
	}
	fitColumnsToWidth(colWidths, drawWidth, tableCellPad)

	colOffsets := make([]float32, numCols+1)
	for ci := 0; ci < numCols; ci++ {
		colOffsets[ci+1] = colOffsets[ci] + colWidths[ci]
	}

	geometry := &tableGeometry{
		width:      drawWidth,
		colWidths:  colWidths,
		colOffsets: colOffsets,
		tableWidth: colOffsets[numCols],
	}
	for _, cells := range rows {
		row := tableRowGeometry{cells: make([]uintptr, numCols)}
		for ci := 0; ci < numCols && ci < len(cells); ci++ {
			var align uint32
			if ci < len(td.Aligns) {
				align = td.Aligns[ci]
			}
			layout := cellLayout(cells[ci], colWidths[ci]-tableCellPad*2, align)
			row.cells[ci] = layout
			if height := layoutHeight(layout); height > row.height {
				row.height = height
			}
		}
		row.height += tableCellPad * 2
		geometry.rows = append(geometry.rows, row)
	}

	td.cached = geometry
	return geometry
}

// Splitter state. splitRatio is the share of the window given to the editor: 0
// collapses the editor, 1 collapses the preview. restoreSplitRatio remembers where
// the splitter was before a collapse so the pane can be brought back where it was.
var (
	splitRatio        = 0.5
	restoreSplitRatio = 0.5
	splitDragging     bool
)

const dividerWidth = int32(6)

// paneGeometry works out where the two panes and the divider sit for a given split.
// It is pure so the collapse and clamping rules can be tested without a window.
func paneGeometry(width, height int32, ratio float64) (editorW, dividerX, previewX, previewW int32) {
	if width <= 0 {
		return 0, 0, 0, 0
	}
	if ratio <= 0 {
		// Editor collapsed: the preview takes everything.
		return 0, 0, 0, width
	}
	if ratio >= 1 {
		// Preview collapsed: the editor takes everything.
		return width, width, width, 0
	}

	editorW = int32(float64(width) * ratio)
	// Keep the divider on screen even at the extremes of a drag.
	if editorW > width-dividerWidth {
		editorW = width - dividerWidth
	}
	if editorW < 0 {
		editorW = 0
	}
	dividerX = editorW
	previewX = editorW + dividerWidth
	previewW = width - previewX
	if previewW < 0 {
		previewW = 0
	}
	return editorW, dividerX, previewX, previewW
}

// splitRatioAt converts a mouse position into a split ratio, snapping to a full
// collapse near either edge so the panes can be closed by dragging.
func splitRatioAt(x, width int32) float64 {
	if width <= 0 {
		return splitRatio
	}
	const snap = int32(24)
	if x <= snap {
		return 0
	}
	if x >= width-snap {
		return 1
	}
	return float64(x) / float64(width)
}

// setSplitRatio applies a new split and re-lays out the panes.
func setSplitRatio(ratio float64) {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	splitRatio = ratio
	if ratio > 0 && ratio < 1 {
		restoreSplitRatio = ratio
	}
	layoutPanes()
}

// toggleCollapse collapses the named pane, or restores the previous split if that
// pane is already collapsed, so the same command works both ways.
func toggleCollapse(collapseEditor bool) {
	target := 1.0 // preview collapsed
	if collapseEditor {
		target = 0
	}
	if splitRatio == target {
		restore := restoreSplitRatio
		if restore <= 0 || restore >= 1 {
			restore = 0.5
		}
		setSplitRatio(restore)
		return
	}
	setSplitRatio(target)
}

// layoutPanes positions the editor, the preview and the divider for the current split.
func layoutPanes() {
	if mainHwnd == 0 {
		return
	}
	var rc [16]byte
	getClientRect.Call(mainHwnd, uintptr(unsafe.Pointer(&rc[0])))
	width := *(*int32)(unsafe.Pointer(&rc[8]))
	height := *(*int32)(unsafe.Pointer(&rc[12]))

	editorW, _, previewX, previewW := paneGeometry(width, height, splitRatio)

	// A hidden window is cheaper than a zero-width one, and Direct2D will not make a
	// render target for a zero-sized client area.
	if editorW <= 0 {
		showWindowProc.Call(editorHwnd, SW_HIDE)
	} else {
		moveWindow.Call(editorHwnd, 0, 0, uintptr(editorW), uintptr(height), 1)
		showWindowProc.Call(editorHwnd, SW_SHOWNOACTIVATE)
	}
	if previewW <= 0 {
		showWindowProc.Call(previewHwnd, SW_HIDE)
	} else {
		moveWindow.Call(previewHwnd, uintptr(previewX), 0, uintptr(previewW), uintptr(height), 1)
		showWindowProc.Call(previewHwnd, SW_SHOWNOACTIVATE)
	}
	invalidateRect.Call(mainHwnd, 0, 1)
}

// overDivider reports whether a client x coordinate falls on the splitter.
func overDivider(x, width int32) bool {
	_, dividerX, previewX, _ := paneGeometry(width, 0, splitRatio)
	if splitRatio <= 0 {
		// Collapsed editor: leave a grab strip at the very left edge.
		return x < dividerWidth
	}
	if splitRatio >= 1 {
		return x > width-dividerWidth
	}
	return x >= dividerX && x < previewX
}

func renderPreview(hwnd uintptr) {
	if res.renderTarget == 0 {
		if !createDeviceResources(hwnd) {
			return
		}
	}

	var rc [16]byte
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc[0])))
	clientW := float32(*(*int32)(unsafe.Pointer(&rc[8])))
	clientH := float32(*(*int32)(unsafe.Pointer(&rc[12])))

	padding := float32(30)
	drawWidth := clientW - padding*2

	// Begin draw — ID2D1RenderTarget::BeginDraw is vtable index 48
	comCall(res.renderTarget, 48)

	// Clear to white — ID2D1RenderTarget::Clear is vtable index 47
	white := [4]float32{1, 1, 1, 1}
	comCall(res.renderTarget, 47, uintptr(unsafe.Pointer(&white[0])))

	// Set transform for scrolling — ID2D1RenderTarget::SetTransform is vtable index 30
	// Matrix3x2F: [m11, m12, m21, m22, dx, dy]
	transform := [6]float32{1, 0, 0, 1, 0, -scrollY}
	comCall(res.renderTarget, 30, uintptr(unsafe.Pointer(&transform[0])))

	y := padding
	for i := range currentBlocks {
		b := &currentBlocks[i]
		y += b.SpaceAbove
		b.Y = y

		if b.Type == blockHR {
			// Draw subtle 1px horizontal line centered in 24px space
			hrHeight := float32(24)
			lineY := y + hrHeight/2
			b.Height = hrHeight
			if lineY-scrollY >= -10 && lineY-scrollY < clientH+10 {
				// Use FillRectangle for a crisp 1px line instead of DrawLine
				hrRect := [4]float32{padding, lineY, clientW - padding, lineY + 1.5}
				comCall(res.renderTarget, 17,
					uintptr(unsafe.Pointer(&hrRect[0])),
					res.brushHR,
				)
			}
			y += hrHeight
			continue
		}

		if b.Type == blockTable && b.Table != nil {
			td := b.Table
			if len(td.Headers) == 0 {
				continue
			}
			geometry := td.geometry(drawWidth)

			for ri, row := range geometry.rows {
				fullRowH := row.height
				// Only draw if visible
				if y-scrollY+fullRowH >= 0 && y-scrollY < clientH {
					// Header row background
					if ri == 0 {
						bgRect := [4]float32{padding, y, padding + geometry.tableWidth, y + fullRowH}
						comCall(res.renderTarget, 17,
							uintptr(unsafe.Pointer(&bgRect[0])),
							res.brushCodeBg,
						)
					}

					// Draw each cell text from the cached layouts
					for ci, cellLayout := range row.cells {
						if cellLayout == 0 {
							continue
						}
						comCall(res.renderTarget, 28,
							packPoint2F(padding+geometry.colOffsets[ci]+tableCellPad, y+tableCellPad),
							cellLayout,
							res.brushText,
							0,
						)
					}

					// Draw cell borders
					for ci := 0; ci <= len(geometry.colWidths); ci++ {
						lineX := padding + geometry.colOffsets[ci]
						// Vertical line
						comCall(res.renderTarget, 15,
							packPoint2F(lineX, y),
							packPoint2F(lineX, y+fullRowH),
							res.brushHR,
							floatBits(1),
							0,
						)
					}
					// Top border
					comCall(res.renderTarget, 15,
						packPoint2F(padding, y),
						packPoint2F(padding+geometry.tableWidth, y),
						res.brushHR,
						floatBits(1),
						0,
					)
					// Bottom border
					comCall(res.renderTarget, 15,
						packPoint2F(padding, y+fullRowH),
						packPoint2F(padding+geometry.tableWidth, y+fullRowH),
						res.brushHR,
						floatBits(1),
						0,
					)
				}

				y += fullRowH
			}
			b.Height = y - b.Y
			continue
		}

		// Reuse the block's layout unless the pane width changed under it.
		layoutWidth := drawWidth - b.Indent
		if b.layout != 0 && b.layoutWidth != layoutWidth {
			b.releaseLayout()
		}
		if b.layout == 0 {
			textU := utf16From(b.Text)
			textLen := uint32(len(textU) - 1)
			fmt := getTextFormat(b)
			if fmt == 0 {
				continue
			}

			var layout uintptr
			// IDWriteFactory::CreateTextLayout is vtable index 18
			comCall(res.dwFactory, 18,
				uintptr(unsafe.Pointer(&textU[0])),
				uintptr(textLen),
				fmt,
				floatBits(layoutWidth),
				floatBits(10000),
				uintptr(unsafe.Pointer(&layout)),
			)
			if layout == 0 {
				continue
			}

			// Apply inline formatting spans (bold, italic, code) via IDWriteTextLayout
			consolasU := utf16From("Consolas")
			for _, span := range b.Spans {
				if span.Bold {
					// IDWriteTextLayout::SetFontWeight is vtable index 32
					comCall(layout, 32, uintptr(DWRITE_FONT_WEIGHT_BOLD), packTextRange(uint32(span.Start), uint32(span.Length)))
				}
				if span.Italic {
					// IDWriteTextLayout::SetFontStyle is vtable index 33
					comCall(layout, 33, uintptr(DWRITE_FONT_STYLE_ITALIC), packTextRange(uint32(span.Start), uint32(span.Length)))
				}
				if span.Code {
					// IDWriteTextLayout::SetFontFamilyName is vtable index 31
					comCall(layout, 31, uintptr(unsafe.Pointer(&consolasU[0])), packTextRange(uint32(span.Start), uint32(span.Length)))
					// IDWriteTextLayout::SetFontSize is vtable index 35
					comCall(layout, 35, floatBits(13), packTextRange(uint32(span.Start), uint32(span.Length)))
				}
			}

			// Get metrics to measure height
			// IDWriteTextLayout inherits from IDWriteTextFormat
			// GetMetrics is at vtable index 60 for IDWriteTextLayout
			var metrics [36]byte // DWRITE_TEXT_METRICS struct
			comCall(layout, 60, uintptr(unsafe.Pointer(&metrics[0])))

			b.layout = layout
			b.layoutWidth = layoutWidth
			b.Height = *(*float32)(unsafe.Pointer(&metrics[16])) // height field
		}
		layout := b.layout
		textH := b.Height
		// Only draw if visible
		if y-scrollY+textH >= 0 && y-scrollY < clientH {
			// Draw code block background
			if b.BgColor != 0 {
				bgRect := [4]float32{
					padding + b.Indent - 10, y - 8,
					clientW - padding + 10, y + textH + 8,
				}
				// ID2D1RenderTarget::FillRectangle is vtable index 17
				comCall(res.renderTarget, 17,
					uintptr(unsafe.Pointer(&bgRect[0])),
					res.brushCodeBg,
				)
			}

			// Draw blockquote left bar
			if b.BarColor != 0 {
				barRect := [4]float32{
					padding, y - 2,
					padding + 4, y + textH + 2,
				}
				comCall(res.renderTarget, 17,
					uintptr(unsafe.Pointer(&barRect[0])),
					res.brushBlue,
				)
			}

			// Draw bullet for list items
			if b.Type == blockListItem {
				// D2D1_ELLIPSE: {D2D1_POINT_2F center, FLOAT radiusX, FLOAT radiusY}
				bulletR := float32(2.5) // ~5px diameter to match WebView2 reference
				bulletCX := padding + 14
				bulletCY := y + 9 // vertically center with first line of text
				ellipse := [4]float32{bulletCX, bulletCY, bulletR, bulletR}
				// ID2D1RenderTarget::FillEllipse is vtable index 21
				comCall(res.renderTarget, 21,
					uintptr(unsafe.Pointer(&ellipse[0])),
					res.brushText,
				)
			}

			// Draw text
			brush := getBrush(b)
			textX := padding + b.Indent
			if b.BgColor != 0 {
				textX += 10 // offset text inside code block background
			}
			// ID2D1RenderTarget::DrawTextLayout is vtable index 28
			// D2D1_POINT_2F (8 bytes) passed by value on x64
			comCall(res.renderTarget, 28,
				packPoint2F(textX, y),
				layout,
				brush,
				0, // D2D1_DRAW_TEXT_OPTIONS_NONE
			)
		}

		// The layout stays on the block; it is released when the document changes.
		y += textH
	}

	totalHeight = y + padding
	maxScroll := max(float32(0), totalHeight-clientH)
	if scrollY > maxScroll {
		scrollY = maxScroll
		invalidateRect.Call(hwnd, 0, 0)
	}

	// End draw — ID2D1RenderTarget::EndDraw is vtable index 49
	var tag1, tag2 uint64
	hr := comCall(res.renderTarget, 49,
		uintptr(unsafe.Pointer(&tag1)),
		uintptr(unsafe.Pointer(&tag2)),
	)
	// Check for device loss (D2DERR_RECREATE_TARGET = 0x8899000C)
	if hr == 0x8899000C {
		discardDeviceResources()
		invalidateRect.Call(hwnd, 0, 0)
	}

	// Update scrollbar
	updateScrollbar(hwnd, int32(clientH))
}

func updateScrollbar(hwnd uintptr, clientH int32) {
	var si [28]byte
	*(*uint32)(unsafe.Pointer(&si[0])) = 28
	*(*uint32)(unsafe.Pointer(&si[4])) = SIF_RANGE | SIF_PAGE | SIF_POS
	*(*int32)(unsafe.Pointer(&si[8])) = 0
	*(*int32)(unsafe.Pointer(&si[12])) = int32(totalHeight)
	*(*uint32)(unsafe.Pointer(&si[16])) = uint32(clientH)
	*(*int32)(unsafe.Pointer(&si[20])) = int32(scrollY)
	setScrollInfo.Call(hwnd, SB_VERT, uintptr(unsafe.Pointer(&si[0])), 1)
}

// Main window proc
func mainWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		// Editor (left)
		editorHwnd, _, _ = createWindowExW.Call(
			WS_EX_CLIENTEDGE,
			uintptr(unsafe.Pointer(utf16Ptr("EDIT"))),
			0,
			WS_CHILD|WS_VISIBLE|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_WANTRETURN,
			0, 0, 0, 0,
			hwnd, 1, hInstance, 0,
		)
		editorFont, _, _ = createFontW.Call(
			uintptr(0xFFFFFFF2), 0, 0, 0, // -14 pixel height
			400, 0, 0, 0, 0, 0, 0, 0, 0,
			uintptr(unsafe.Pointer(utf16Ptr("Consolas"))),
		)
		sendMessageW.Call(editorHwnd, WM_SETFONT, editorFont, 1)

		// Set editor margins (10px left and right)
		const EM_SETMARGINS = 0x00D3
		margins := uintptr((10 << 16) | 10)                      // 10px left and right
		sendMessageW.Call(editorHwnd, EM_SETMARGINS, 3, margins) // EC_LEFTMARGIN|EC_RIGHTMARGIN = 3

		// Preview (right) — custom D2D window
		previewClass := utf16From("TinyMDD2DPreview")
		var wc [80]byte
		*(*uint32)(unsafe.Pointer(&wc[0])) = 80
		*(*uintptr)(unsafe.Pointer(&wc[8])) = syscall.NewCallback(previewWndProc)
		*(*uintptr)(unsafe.Pointer(&wc[24])) = hInstance
		cursor, _, _ := loadCursorW.Call(0, IDC_ARROW)
		*(*uintptr)(unsafe.Pointer(&wc[40])) = cursor
		*(*uintptr)(unsafe.Pointer(&wc[64])) = uintptr(unsafe.Pointer(&previewClass[0]))
		registerClassExW.Call(uintptr(unsafe.Pointer(&wc[0])))

		previewHwnd, _, _ = createWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(&previewClass[0])),
			0,
			WS_CHILD|WS_VISIBLE|WS_VSCROLL,
			0, 0, 0, 0,
			hwnd, 2, hInstance, 0,
		)

		// Initialize D2D with preview window
		initD2D(previewHwnd)
		installMainMenu(hwnd)

		return 0

	case WM_SIZE:
		layoutPanes()
		return 0

	case WM_SETCURSOR:
		// Only the client area, and only while the pointer is over the splitter.
		if loword(lParam) == HTCLIENT {
			var pt [8]byte
			getCursorPos.Call(uintptr(unsafe.Pointer(&pt[0])))
			screenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt[0])))
			x := *(*int32)(unsafe.Pointer(&pt[0]))
			var rc [16]byte
			getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc[0])))
			if splitDragging || overDivider(x, *(*int32)(unsafe.Pointer(&rc[8]))) {
				cursor, _, _ := loadCursorW.Call(0, IDC_SIZEWE)
				setCursorProc.Call(cursor)
				return 1
			}
		}

	case WM_LBUTTONDOWN:
		var rc [16]byte
		getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc[0])))
		if overDivider(int32(int16(loword(lParam))), *(*int32)(unsafe.Pointer(&rc[8]))) {
			splitDragging = true
			setCapture.Call(hwnd)
			return 0
		}

	case WM_MOUSEMOVE:
		if splitDragging {
			var rc [16]byte
			getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc[0])))
			setSplitRatio(splitRatioAt(int32(int16(loword(lParam))), *(*int32)(unsafe.Pointer(&rc[8]))))
			return 0
		}

	case WM_LBUTTONUP:
		if splitDragging {
			splitDragging = false
			releaseCapture.Call()
			return 0
		}

	case WM_CAPTURECHANGED:
		splitDragging = false

	case WM_LBUTTONDBLCLK:
		// Double-clicking the splitter restores an even split, which is the quickest
		// way back from either collapsed state.
		var rc [16]byte
		getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc[0])))
		if overDivider(int32(int16(loword(lParam))), *(*int32)(unsafe.Pointer(&rc[8]))) {
			setSplitRatio(0.5)
			return 0
		}

	case WM_COMMAND:
		if hiword(wParam) == EN_CHANGE && loword(wParam) == 1 {
			// Re-parsing a large document costs under a millisecond and a repaint
			// about the same, so the preview can follow typing closely. The debounce
			// only needs to coalesce a burst of keystrokes, not hide slow work.
			setTimer.Call(hwnd, TIMER_DEBOUNCE, 16, 0)
		}
		if handleMenuCommand(uintptr(loword(wParam))) {
			return 0
		}
		return 0

	case WM_TIMER:
		if wParam == TIMER_DEBOUNCE {
			killTimer.Call(hwnd, TIMER_DEBOUNCE)
			updatePreview()
		}
		return 0

	case WM_KEYDOWN:
		if handleShortcut(wParam) {
			return 0
		}

	case WM_SETFOCUS:
		setFocus.Call(editorHwnd)
		return 0

	case WM_DESTROY:
		discardDeviceResources()
		comRelease(res.dwFactory)
		comRelease(res.factory)
		deleteObject.Call(editorFont)
		postQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := defWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func previewWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_PAINT:
		var ps [72]byte
		beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps[0])))
		renderPreview(hwnd)
		endPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps[0])))
		return 0

	case WM_ERASEBKGND:
		return 1

	case WM_SIZE:
		// Discard render target so it gets recreated with new size on next paint
		if res.renderTarget != 0 {
			discardDeviceResources()
		}
		invalidateRect.Call(hwnd, 0, 0)
		return 0

	case WM_MOUSEWHEEL:
		delta := int16(hiword(wParam))
		scrollY -= float32(delta) / 3
		if scrollY < 0 {
			scrollY = 0
		}
		var rc [16]byte
		getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc[0])))
		clientH := float32(*(*int32)(unsafe.Pointer(&rc[12])))
		maxScroll := totalHeight - clientH
		if maxScroll < 0 {
			maxScroll = 0
		}
		if scrollY > maxScroll {
			scrollY = maxScroll
		}
		invalidateRect.Call(hwnd, 0, 0)
		return 0

	case WM_VSCROLL:
		code := loword(wParam)
		var rc [16]byte
		getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc[0])))
		clientH := float32(*(*int32)(unsafe.Pointer(&rc[12])))
		maxScroll := totalHeight - clientH
		if maxScroll < 0 {
			maxScroll = 0
		}
		switch code {
		case SB_LINEUP:
			scrollY -= 30
		case SB_LINEDOWN:
			scrollY += 30
		case SB_PAGEUP:
			scrollY -= clientH
		case SB_PAGEDOWN:
			scrollY += clientH
		case SB_THUMBTRACK, SB_THUMBPOSITION:
			scrollY = float32(int16(hiword(wParam)))
		}
		if scrollY < 0 {
			scrollY = 0
		}
		if scrollY > maxScroll {
			scrollY = maxScroll
		}
		invalidateRect.Call(hwnd, 0, 0)
		return 0
	}

	ret, _, _ := defWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func updatePreview() {
	// The old blocks own Direct2D text layouts; drop them before replacing them.
	releaseBlockLayouts(currentBlocks)

	length, _, _ := getWindowTextLengthW.Call(editorHwnd)
	if length == 0 {
		currentBlocks = nil
		scrollY = 0
		totalHeight = 0
		invalidateRect.Call(previewHwnd, 0, 0)
		return
	}
	buf := make([]uint16, length+1)
	getWindowTextW.Call(editorHwnd, uintptr(unsafe.Pointer(&buf[0])), length+1)
	mdText := syscall.UTF16ToString(buf)

	currentBlocks = markdownToLayout([]byte(mdText))
	invalidateRect.Call(previewHwnd, 0, 0)
}

// File operations
func windowTitle() string {
	if currentFile != "" {
		return "TinyMD D2D — " + currentFile
	}
	return "TinyMD D2D Prototype"
}

func refreshWindowTitle() {
	title := utf16From(windowTitle())
	setWindowTextW.Call(mainHwnd, uintptr(unsafe.Pointer(&title[0])))
}

func editorText() string {
	length, _, _ := getWindowTextLengthW.Call(editorHwnd)
	buf := make([]uint16, length+1)
	getWindowTextW.Call(editorHwnd, uintptr(unsafe.Pointer(&buf[0])), length+1)
	return syscall.UTF16ToString(buf)
}

func saveFile() {
	if currentFile == "" {
		saveFileAs()
		return
	}
	os.WriteFile(currentFile, []byte(documentText()), 0644)
}

func saveFileAs() {
	path := showSaveDialog(mainHwnd, currentFile)
	if path == "" {
		return
	}
	if err := os.WriteFile(path, []byte(documentText()), 0644); err != nil {
		return
	}
	currentFile = path
	refreshWindowTitle()
}

func openFile() {
	path := showOpenDialog(mainHwnd)
	if path == "" {
		return
	}
	if err := loadFile(path); err != nil {
		showFileOpenError(path, err)
	}
}

func documentText() string {
	return strings.ReplaceAll(strings.ReplaceAll(editorText(), "\r\n", "\n"), "\n", lineEnding)
}

func loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	ending := "\n"
	if strings.Contains(content, "\r\n") {
		ending = "\r\n"
	}
	content = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\n", "\r\n")
	encoded, err := syscall.UTF16FromString(content)
	if err != nil {
		return fmt.Errorf("the file contains NUL characters and cannot be opened as UTF-8 Markdown")
	}
	loaded, _, _ := sendMessageW.Call(editorHwnd, WM_SETTEXT, 0, uintptr(unsafe.Pointer(&encoded[0])))
	if loaded == 0 {
		return fmt.Errorf("could not load the document into the editor")
	}
	currentFile = path
	lineEnding = ending
	scrollY = 0
	totalHeight = 0
	updatePreview()
	refreshWindowTitle()
	return nil
}

func showFileOpenError(path string, err error) {
	body := utf16From(fmt.Sprintf("Could not open %s\n\n%v", path, err))
	title := utf16From("TinyMD — Open failed")
	messageBoxW.Call(mainHwnd, uintptr(unsafe.Pointer(&body[0])), uintptr(unsafe.Pointer(&title[0])), mbOk|mbIconError)
}

func openInFolder() {
	if currentFile == "" {
		return
	}
	verb := utf16From("open")
	exe := utf16From("explorer.exe")
	params := utf16From(`/select,"` + currentFile + `"`)
	shellExecuteW.Call(mainHwnd, uintptr(unsafe.Pointer(&verb[0])), uintptr(unsafe.Pointer(&exe[0])), uintptr(unsafe.Pointer(&params[0])), 0, 1)
}

func absoluteFilePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("save or open a file before copying its path")
	}
	return filepath.Abs(path)
}

func setUnicodeClipboardText(owner uintptr, value string) error {
	text := utf16From(value)
	bytes := uintptr(len(text) * 2)
	handle, _, _ := globalAlloc.Call(gmemMoveable, bytes)
	if handle == 0 {
		return fmt.Errorf("could not allocate clipboard memory")
	}
	locked, _, _ := globalLock.Call(handle)
	if locked == 0 {
		globalFree.Call(handle)
		return fmt.Errorf("could not lock clipboard memory")
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(locked)), len(text)), text)
	globalUnlock.Call(handle)

	opened, _, _ := openClipboard.Call(owner)
	if opened == 0 {
		globalFree.Call(handle)
		return fmt.Errorf("the clipboard is currently unavailable")
	}
	defer closeClipboard.Call()
	if emptied, _, _ := emptyClipboard.Call(); emptied == 0 {
		globalFree.Call(handle)
		return fmt.Errorf("could not clear the clipboard")
	}
	if stored, _, _ := setClipboardData.Call(cfUnicodeText, handle); stored == 0 {
		globalFree.Call(handle)
		return fmt.Errorf("could not write to the clipboard")
	}
	return nil
}

func copyCurrentFilePath() {
	path, err := absoluteFilePath(currentFile)
	if err == nil {
		err = setUnicodeClipboardText(mainHwnd, path)
	}
	if err != nil {
		body := utf16From(err.Error())
		title := utf16From("Copy File Path")
		messageBoxW.Call(mainHwnd, uintptr(unsafe.Pointer(&body[0])), uintptr(unsafe.Pointer(&title[0])), mbOk|mbIconError)
	}
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
	defExt := utf16From("md")

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

func handleShortcut(wParam uintptr) bool {
	ctrl, _, _ := getKeyState.Call(VK_CONTROL)
	if int16(ctrl) >= 0 {
		return false
	}
	shift, _, _ := getKeyState.Call(VK_SHIFT)
	switch {
	case wParam == VK_A:
		sendMessageW.Call(editorHwnd, EM_SETSEL, 0, ^uintptr(0))
	case wParam == VK_C && int16(shift) < 0:
		copyCurrentFilePath()
	case wParam == VK_C:
		sendMessageW.Call(editorHwnd, WM_COPY, 0, 0)
	case wParam == VK_V:
		sendMessageW.Call(editorHwnd, WM_PASTE, 0, 0)
		updatePreview()
	case wParam == VK_X:
		sendMessageW.Call(editorHwnd, WM_CUT, 0, 0)
		updatePreview()
	case wParam == VK_O:
		openFile()
	case wParam == VK_S && int16(shift) < 0:
		saveFileAs()
	case wParam == VK_S:
		saveFile()
	case wParam == VK_E:
		openInFolder()
	case wParam == VK_P:
		printFormatted()
	case wParam == VK_1:
		toggleCollapse(false)
	case wParam == VK_2:
		toggleCollapse(true)
	case wParam == VK_0:
		setSplitRatio(0.5)
	default:
		return false
	}
	return true
}

func makePrintFont(height, weight int32, italic uint32, face string, scale float64) uintptr {
	scaledH := int32(float64(height) * scale)
	f, _, _ := createFontW.Call(
		uintptr(uint32(uint16(scaledH))|0xFFFF0000),
		0, 0, 0,
		uintptr(weight),
		uintptr(italic),
		0, 0, 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(utf16Ptr(face))),
	)
	return f
}

func printFormatted() {
	if len(currentBlocks) == 0 {
		return
	}

	const pdSize = 120
	var pd [pdSize]byte
	*(*uint32)(unsafe.Pointer(&pd[0])) = pdSize
	*(*uintptr)(unsafe.Pointer(&pd[8])) = mainHwnd
	*(*uint32)(unsafe.Pointer(&pd[40])) = PD_RETURNDC | PD_USEDEVMODECOPIESANDCOLLATE

	ret, _, _ := printDlgW.Call(uintptr(unsafe.Pointer(&pd[0])))
	if ret == 0 {
		return
	}

	printerDC := *(*uintptr)(unsafe.Pointer(&pd[32])) // hDC
	if printerDC == 0 {
		return
	}
	defer deleteDC.Call(printerDC)

	dpiX, _, _ := getDeviceCaps.Call(printerDC, LOGPIXELSX)
	dpiY, _, _ := getDeviceCaps.Call(printerDC, LOGPIXELSY)
	pageW, _, _ := getDeviceCaps.Call(printerDC, HORZRES)
	pageH, _, _ := getDeviceCaps.Call(printerDC, VERTRES)

	scaleX := float64(dpiX) / 96.0
	scaleY := float64(dpiY) / 96.0

	// Create scaled fonts
	pFontH1 := makePrintFont(-28, 700, 0, "Segoe UI", scaleY)
	pFontH2 := makePrintFont(-22, 700, 0, "Segoe UI", scaleY)
	pFontH3 := makePrintFont(-18, 700, 0, "Segoe UI", scaleY)
	pFontBody := makePrintFont(-14, 400, 0, "Segoe UI", scaleY)
	pFontBodyBold := makePrintFont(-14, 700, 0, "Segoe UI", scaleY)
	pFontBodyItalic := makePrintFont(-14, 400, 1, "Segoe UI", scaleY)
	pFontCode := makePrintFont(-13, 400, 0, "Consolas", scaleY)
	defer func() {
		for _, f := range []uintptr{pFontH1, pFontH2, pFontH3, pFontBody, pFontBodyBold, pFontBodyItalic, pFontCode} {
			deleteObject.Call(f)
		}
	}()

	// Map block properties to printer fonts
	getFontForBlock := func(b *LayoutBlock) uintptr {
		if b.Type == blockCode {
			return pFontCode
		}
		if b.Bold {
			switch {
			case b.FontSize >= 28:
				return pFontH1
			case b.FontSize >= 22:
				return pFontH2
			default:
				return pFontH3
			}
		}
		return pFontBody
	}

	// StartDoc
	docName := utf16From("TinyMD Print")
	var di [40]byte
	*(*int32)(unsafe.Pointer(&di[0])) = 40
	*(*uintptr)(unsafe.Pointer(&di[8])) = uintptr(unsafe.Pointer(&docName[0]))
	r, _, _ := startDocW.Call(printerDC, uintptr(unsafe.Pointer(&di[0])))
	if int32(r) <= 0 {
		return
	}

	padding := int32(float64(30) * scaleX)
	drawWidth := int32(pageW) - padding*2

	pCodeBrush, _, _ := createSolidBrush.Call(0x00F0F0F0)
	pBlueBrush, _, _ := createSolidBrush.Call(0x00D66603)
	defer func() {
		deleteObject.Call(pCodeBrush)
		deleteObject.Call(pBlueBrush)
	}()

	startPage.Call(printerDC)
	setBkMode.Call(printerDC, TRANSPARENT_BK)

	y := padding
	maxY := int32(pageH) - padding

	for i := range currentBlocks {
		b := &currentBlocks[i]
		spaceAbove := int32(float64(b.SpaceAbove) * scaleY)
		indent := int32(float64(b.Indent) * scaleX)
		pFont := getFontForBlock(b)

		if b.Type == blockHR {
			y += spaceAbove
			hrPen, _, _ := createPen.Call(0, 1, 0x00CCCCCC)
			oldP, _, _ := selectObject.Call(printerDC, hrPen)
			hrY := y + int32(12*scaleY)
			moveToEx.Call(printerDC, uintptr(padding), uintptr(hrY), 0)
			lineTo.Call(printerDC, uintptr(int32(pageW)-padding), uintptr(hrY))
			selectObject.Call(printerDC, oldP)
			deleteObject.Call(hrPen)
			y += int32(24 * scaleY)
			continue
		}

		if b.Type == blockTable && b.Table != nil {
			y += spaceAbove
			td := b.Table
			numCols := len(td.Headers)
			if numCols == 0 {
				continue
			}
			cellPad := int32(float64(8) * scaleX)

			colWidths := make([]int32, numCols)
			oldF, _, _ := selectObject.Call(printerDC, pFontH3)
			for ci, cell := range td.Headers {
				cellTxt := utf16From(cell.Text)
				var mRC [16]byte
				*(*int32)(unsafe.Pointer(&mRC[8])) = 10000
				*(*int32)(unsafe.Pointer(&mRC[12])) = 10000
				drawTextW.Call(printerDC, uintptr(unsafe.Pointer(&cellTxt[0])),
					uintptr(len(cellTxt)-1), uintptr(unsafe.Pointer(&mRC[0])),
					DT_CALCRECT|DT_NOPREFIX|DT_SINGLELINE)
				w := *(*int32)(unsafe.Pointer(&mRC[8]))
				if w > colWidths[ci] {
					colWidths[ci] = w
				}
			}
			selectObject.Call(printerDC, pFontBody)
			for _, row := range td.Rows {
				for ci, cell := range row {
					if ci >= numCols {
						break
					}
					cellTxt := utf16From(cell.Text)
					var mRC [16]byte
					*(*int32)(unsafe.Pointer(&mRC[8])) = 10000
					*(*int32)(unsafe.Pointer(&mRC[12])) = 10000
					drawTextW.Call(printerDC, uintptr(unsafe.Pointer(&cellTxt[0])),
						uintptr(len(cellTxt)-1), uintptr(unsafe.Pointer(&mRC[0])),
						DT_CALCRECT|DT_NOPREFIX|DT_SINGLELINE)
					w := *(*int32)(unsafe.Pointer(&mRC[8]))
					if w > colWidths[ci] {
						colWidths[ci] = w
					}
				}
			}
			for ci := range colWidths {
				colWidths[ci] += cellPad * 2
			}

			sampleTxt := utf16From("Ay")
			var measureRC [16]byte
			*(*int32)(unsafe.Pointer(&measureRC[8])) = 10000
			*(*int32)(unsafe.Pointer(&measureRC[12])) = 10000
			drawTextW.Call(printerDC, uintptr(unsafe.Pointer(&sampleTxt[0])),
				uintptr(len(sampleTxt)-1), uintptr(unsafe.Pointer(&measureRC[0])),
				DT_CALCRECT|DT_NOPREFIX|DT_SINGLELINE)
			rowH := *(*int32)(unsafe.Pointer(&measureRC[12])) + cellPad*2
			selectObject.Call(printerDC, oldF)

			colLeft := make([]int32, numCols+1)
			colLeft[0] = padding
			for ci := 0; ci < numCols; ci++ {
				colLeft[ci+1] = colLeft[ci] + colWidths[ci]
			}

			tablePen, _, _ := createPen.Call(0, 1, 0x00DDDDDD)
			oldPen, _, _ := selectObject.Call(printerDC, tablePen)
			setTextColor.Call(printerDC, 0x00222222)

			// Header
			oldF2, _, _ := selectObject.Call(printerDC, pFontH3)
			for ci, cell := range td.Headers {
				cl := colLeft[ci]
				cr := colLeft[ci+1]
				var cellRC [16]byte
				*(*int32)(unsafe.Pointer(&cellRC[0])) = cl + cellPad
				*(*int32)(unsafe.Pointer(&cellRC[4])) = y + cellPad
				*(*int32)(unsafe.Pointer(&cellRC[8])) = cr - cellPad
				*(*int32)(unsafe.Pointer(&cellRC[12])) = y + rowH - cellPad
				cellTxt := utf16From(cell.Text)
				drawTextW.Call(printerDC, uintptr(unsafe.Pointer(&cellTxt[0])),
					uintptr(len(cellTxt)-1), uintptr(unsafe.Pointer(&cellRC[0])),
					DT_NOPREFIX|DT_SINGLELINE|DT_NOCLIP)
				moveToEx.Call(printerDC, uintptr(cl), uintptr(y), 0)
				lineTo.Call(printerDC, uintptr(cr), uintptr(y))
				moveToEx.Call(printerDC, uintptr(cl), uintptr(y), 0)
				lineTo.Call(printerDC, uintptr(cl), uintptr(y+rowH))
				moveToEx.Call(printerDC, uintptr(cr), uintptr(y), 0)
				lineTo.Call(printerDC, uintptr(cr), uintptr(y+rowH))
				moveToEx.Call(printerDC, uintptr(cl), uintptr(y+rowH), 0)
				lineTo.Call(printerDC, uintptr(cr), uintptr(y+rowH))
			}
			selectObject.Call(printerDC, oldF2)
			y += rowH

			oldF2, _, _ = selectObject.Call(printerDC, pFontBody)
			for _, row := range td.Rows {
				if y+rowH > maxY {
					endPage.Call(printerDC)
					startPage.Call(printerDC)
					setBkMode.Call(printerDC, TRANSPARENT_BK)
					y = padding
				}
				for ci, cell := range row {
					if ci >= numCols {
						break
					}
					cl := colLeft[ci]
					cr := colLeft[ci+1]
					var cellRC [16]byte
					*(*int32)(unsafe.Pointer(&cellRC[0])) = cl + cellPad
					*(*int32)(unsafe.Pointer(&cellRC[4])) = y + cellPad
					*(*int32)(unsafe.Pointer(&cellRC[8])) = cr - cellPad
					*(*int32)(unsafe.Pointer(&cellRC[12])) = y + rowH - cellPad
					cellTxt := utf16From(cell.Text)
					drawTextW.Call(printerDC, uintptr(unsafe.Pointer(&cellTxt[0])),
						uintptr(len(cellTxt)-1), uintptr(unsafe.Pointer(&cellRC[0])),
						DT_NOPREFIX|DT_SINGLELINE|DT_NOCLIP)
					moveToEx.Call(printerDC, uintptr(cl), uintptr(y), 0)
					lineTo.Call(printerDC, uintptr(cr), uintptr(y))
					moveToEx.Call(printerDC, uintptr(cl), uintptr(y), 0)
					lineTo.Call(printerDC, uintptr(cl), uintptr(y+rowH))
					moveToEx.Call(printerDC, uintptr(cr), uintptr(y), 0)
					lineTo.Call(printerDC, uintptr(cr), uintptr(y+rowH))
					moveToEx.Call(printerDC, uintptr(cl), uintptr(y+rowH), 0)
					lineTo.Call(printerDC, uintptr(cr), uintptr(y+rowH))
				}
				y += rowH
			}
			selectObject.Call(printerDC, oldF2)
			selectObject.Call(printerDC, oldPen)
			deleteObject.Call(tablePen)
			continue
		}

		// Regular text blocks
		codePad := int32(0)
		if b.BgColor != 0 {
			codePad = int32(float64(10) * scaleX)
		}

		var measureRC [16]byte
		*(*int32)(unsafe.Pointer(&measureRC[8])) = drawWidth - indent - codePad*2
		*(*int32)(unsafe.Pointer(&measureRC[12])) = 10000
		txt := utf16From(b.Text)
		oldF, _, _ := selectObject.Call(printerDC, pFont)
		drawTextW.Call(printerDC, uintptr(unsafe.Pointer(&txt[0])),
			uintptr(len(txt)-1), uintptr(unsafe.Pointer(&measureRC[0])),
			DT_CALCRECT|DT_WORDBREAK|DT_NOPREFIX|DT_EXPANDTABS|DT_EDITCONTROL)
		textH := *(*int32)(unsafe.Pointer(&measureRC[12]))

		blockH := textH + codePad*2 + spaceAbove
		if y+blockH > maxY && y > padding {
			endPage.Call(printerDC)
			startPage.Call(printerDC)
			setBkMode.Call(printerDC, TRANSPARENT_BK)
			y = padding
		}

		y += spaceAbove

		if b.BgColor != 0 {
			var bgRC [16]byte
			*(*int32)(unsafe.Pointer(&bgRC[0])) = padding + indent - int32(4*scaleX)
			*(*int32)(unsafe.Pointer(&bgRC[4])) = y
			*(*int32)(unsafe.Pointer(&bgRC[8])) = int32(pageW) - padding + int32(4*scaleX)
			*(*int32)(unsafe.Pointer(&bgRC[12])) = y + textH + codePad*2
			fillRect.Call(printerDC, uintptr(unsafe.Pointer(&bgRC[0])), pCodeBrush)
		}

		if b.BarColor != 0 {
			var barRC [16]byte
			*(*int32)(unsafe.Pointer(&barRC[0])) = padding
			*(*int32)(unsafe.Pointer(&barRC[4])) = y - int32(2*scaleY)
			*(*int32)(unsafe.Pointer(&barRC[8])) = padding + int32(4*scaleX)
			*(*int32)(unsafe.Pointer(&barRC[12])) = y + textH + int32(2*scaleY)
			fillRect.Call(printerDC, uintptr(unsafe.Pointer(&barRC[0])), pBlueBrush)
		}

		if b.Type == blockListItem {
			selectObject.Call(printerDC, pFontBody)
			setTextColor.Call(printerDC, 0x00333333)
			bullet := utf16From("\u2022")
			textOutW.Call(printerDC,
				uintptr(padding+int32(8*scaleX)), uintptr(y+codePad),
				uintptr(unsafe.Pointer(&bullet[0])), uintptr(len(bullet)-1))
		}

		// Set text color from block
		color := uint32(0)
		if b.Color == 0xFF666666 {
			color = 0x00666666
		} else {
			color = 0x00222222
		}
		setTextColor.Call(printerDC, uintptr(color))

		// Draw with inline spans
		if len(b.Spans) > 0 {
			curX := padding + indent + codePad
			curY := y + codePad
			maxX := int32(pageW) - padding - codePad

			hSample := utf16From("Ay")
			var hRC [16]byte
			*(*int32)(unsafe.Pointer(&hRC[8])) = 10000
			*(*int32)(unsafe.Pointer(&hRC[12])) = 10000
			drawTextW.Call(printerDC, uintptr(unsafe.Pointer(&hSample[0])),
				uintptr(len(hSample)-1), uintptr(unsafe.Pointer(&hRC[0])),
				DT_CALCRECT|DT_NOPREFIX)
			lineH := *(*int32)(unsafe.Pointer(&hRC[12]))

			// Render the text character by character using spans
			textBytes := []byte(b.Text)
			pos := 0
			for _, span := range b.Spans {
				// Text before this span (normal)
				if span.Start > pos {
					chunk := string(textBytes[pos:span.Start])
					if chunk != "" {
						selectObject.Call(printerDC, pFont)
						chunkU := utf16From(chunk)
						var sz [8]byte
						getTextExtentPoint32W.Call(printerDC,
							uintptr(unsafe.Pointer(&chunkU[0])), uintptr(len(chunkU)-1),
							uintptr(unsafe.Pointer(&sz[0])))
						cw := *(*int32)(unsafe.Pointer(&sz[0]))
						if curX+cw > maxX && curX > padding+indent+codePad {
							curX = padding + indent + codePad
							curY += lineH
						}
						textOutW.Call(printerDC, uintptr(curX), uintptr(curY),
							uintptr(unsafe.Pointer(&chunkU[0])), uintptr(len(chunkU)-1))
						curX += cw
					}
				}
				// Span text
				spanText := string(textBytes[span.Start : span.Start+span.Length])
				spanFont := pFont
				if span.Bold {
					spanFont = pFontBodyBold
				}
				if span.Italic {
					spanFont = pFontBodyItalic
				}
				if span.Code {
					spanFont = pFontCode
				}
				selectObject.Call(printerDC, spanFont)
				spanU := utf16From(spanText)
				var sz [8]byte
				getTextExtentPoint32W.Call(printerDC,
					uintptr(unsafe.Pointer(&spanU[0])), uintptr(len(spanU)-1),
					uintptr(unsafe.Pointer(&sz[0])))
				sw := *(*int32)(unsafe.Pointer(&sz[0]))
				if curX+sw > maxX && curX > padding+indent+codePad {
					curX = padding + indent + codePad
					curY += lineH
				}
				textOutW.Call(printerDC, uintptr(curX), uintptr(curY),
					uintptr(unsafe.Pointer(&spanU[0])), uintptr(len(spanU)-1))
				curX += sw
				if span.Italic {
					curX += int32(2 * scaleX)
				}
				pos = span.Start + span.Length
			}
			// Remaining text after last span
			if pos < len(textBytes) {
				chunk := string(textBytes[pos:])
				if chunk != "" {
					selectObject.Call(printerDC, pFont)
					chunkU := utf16From(chunk)
					textOutW.Call(printerDC, uintptr(curX), uintptr(curY),
						uintptr(unsafe.Pointer(&chunkU[0])), uintptr(len(chunkU)-1))
				}
			}
		} else {
			var drawRC [16]byte
			*(*int32)(unsafe.Pointer(&drawRC[0])) = padding + indent + codePad
			*(*int32)(unsafe.Pointer(&drawRC[4])) = y + codePad
			*(*int32)(unsafe.Pointer(&drawRC[8])) = int32(pageW) - padding - codePad
			*(*int32)(unsafe.Pointer(&drawRC[12])) = y + codePad + textH
			drawTextW.Call(printerDC, uintptr(unsafe.Pointer(&txt[0])),
				uintptr(len(txt)-1), uintptr(unsafe.Pointer(&drawRC[0])),
				DT_WORDBREAK|DT_NOPREFIX|DT_EXPANDTABS|DT_EDITCONTROL)
		}

		selectObject.Call(printerDC, oldF)
		y += textH + codePad*2
	}

	endPage.Call(printerDC)
	endDoc.Call(printerDC)
}

func main() {
	runtime.LockOSThread()

	var initialFile string
	autoPrint := false
	for _, arg := range os.Args[1:] {
		if arg == "--print" {
			autoPrint = true
		} else if initialFile == "" {
			initialFile = arg
		}
	}

	hInstance, _, _ = getModuleHandleW.Call(0)

	// Register main window class
	className := utf16From("TinyMDD2D")
	cursor, _, _ := loadCursorW.Call(0, IDC_ARROW)

	var wc [80]byte
	*(*uint32)(unsafe.Pointer(&wc[0])) = 80
	*(*uint32)(unsafe.Pointer(&wc[4])) = CS_DBLCLKS // so the splitter can be double-clicked
	*(*uintptr)(unsafe.Pointer(&wc[8])) = syscall.NewCallback(mainWndProc)
	*(*uintptr)(unsafe.Pointer(&wc[24])) = hInstance
	*(*uintptr)(unsafe.Pointer(&wc[40])) = cursor
	// The panes cover the whole client area except the splitter strip, so the class
	// background is what draws the splitter. COLOR_BTNFACE reads as a divider.
	*(*uintptr)(unsafe.Pointer(&wc[48])) = 16 // COLOR_BTNFACE+1
	*(*uintptr)(unsafe.Pointer(&wc[64])) = uintptr(unsafe.Pointer(&className[0]))
	registerClassExW.Call(uintptr(unsafe.Pointer(&wc[0])))

	// Center on screen
	w, h := uintptr(1400), uintptr(900)
	screenW, _, _ := getSystemMetrics.Call(SM_CXSCREEN)
	screenH, _, _ := getSystemMetrics.Call(SM_CYSCREEN)
	x := (screenW - w) / 2
	y := (screenH - h) / 2

	titleU := utf16From(windowTitle())

	mainHwnd, _, _ = createWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(&className[0])),
		uintptr(unsafe.Pointer(&titleU[0])),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE|WS_CLIPCHILDREN,
		x, y, w, h,
		0, 0, hInstance, 0,
	)

	showWindowProc.Call(mainHwnd, 5)
	updateWindowProc.Call(mainHwnd)

	// Set initial content
	if initialFile != "" {
		if err := loadFile(initialFile); err != nil {
			showFileOpenError(initialFile, err)
		}
	}

	setFocus.Call(editorHwnd)

	if autoPrint {
		fmt.Println("[D2D] --print flag detected, auto-printing...")
		printFormatted()
		fmt.Println("[D2D] print done, exiting")
		return
	}

	// Message loop — intercept shortcuts before dispatch since
	// the EDIT control has focus and mainWndProc never sees WM_KEYDOWN.
	var msgBuf [48]byte
	for {
		ret, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&msgBuf[0])), 0, 0, 0)
		if ret == 0 || int32(ret) == -1 {
			break
		}
		msgID := *(*uint32)(unsafe.Pointer(&msgBuf[8]))
		wParam := *(*uintptr)(unsafe.Pointer(&msgBuf[16]))
		if msgID == WM_KEYDOWN {
			if handleShortcut(wParam) {
				continue
			}
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msgBuf[0])))
		dispatchMessageW.Call(uintptr(unsafe.Pointer(&msgBuf[0])))
	}
}

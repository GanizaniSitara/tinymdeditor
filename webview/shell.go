package main

// The native side of the window: the menu bar, remembered settings and recent
// files, the modified marker in the title, the save prompt on close, and keeping a
// file's line endings when it is saved.

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"github.com/jchv/go-webview2"
)

//go:embed page.html
var pageHTML string

//go:embed app.js
var appJS string

// marked.js (MIT) is embedded rather than fetched: outbound traffic from WebView2
// can be blocked, and a local copy also renders sooner.
//
//go:embed marked.min.js
var markedJS string

const (
	idFileNew         = 1008
	idFileCopyPath    = 1009
	idFileRecentBase  = 1500 // maxRecent ids from here
	idEditRedo        = 1106
	idEditFind        = 1107
	idEditFindNext    = 1108
	idViewSource      = 1301
	idViewRendered    = 1302
	idViewSplit       = 1303
	idViewZoomIn      = 1304
	idViewZoomOut     = 1305
	idViewZoomReset   = 1306
	idViewWideMargins = 1307
	idFmtBold         = 1401
	idFmtItalic       = 1402
	idFmtHeading      = 1403
	idFmtList         = 1404
	idFmtLink         = 1405
	idFmtCode         = 1406

	maxRecent     = 10
	wmClose       = 0x0010
	mfGrayed      = 0x0001
	mfChecked     = 0x0008
	mfByPosition  = 0x0400
	mbYesNoCancel = 0x0003
	mbIconWarning = 0x0030
	mbIconError   = 0x0010
	idYes         = 6
	idNo          = 7
)

var (
	checkMenuItem    = user32.NewProc("CheckMenuItem")
	deleteMenu       = user32.NewProc("DeleteMenu")
	getMenuItemCount = user32.NewProc("GetMenuItemCount")
	postMessageW     = user32.NewProc("PostMessageW")
)

var (
	mainHwnd   uintptr
	recentMenu uintptr
	viewMenu   uintptr

	// dirty mirrors the page's modified flag, for the title and the close prompt.
	dirty bool
	// forceClose lets a close through once the user has answered the save prompt.
	forceClose bool
	// fileCRLF records that the open file used CRLF; the editor works in LF and the
	// file is written back the way it came.
	fileCRLF bool

	settings = pageSettings{View: "split", Zoom: 1, Split: 50}
	// settingsFrozen keeps the self-test from rewriting the real settings file.
	settingsFrozen bool
	recent         []string
)

// pageSettings is what the page remembers between runs.
type pageSettings struct {
	View  string  `json:"view"`
	Zoom  float64 `json:"zoom"`
	Wide  bool    `json:"wide"`
	Split float64 `json:"split"`
}

func settingsPath() string {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "TinyMD", "settings.ini")
}

func loadSettings() {
	file, err := os.Open(settingsPath())
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found {
			continue
		}
		switch key {
		case "view":
			if value == "source" || value == "rendered" || value == "split" {
				settings.View = value
			}
		case "zoom":
			if z, err := strconv.ParseFloat(value, 64); err == nil && z >= 0.5 && z <= 3 {
				settings.Zoom = z
			}
		case "wide_margins":
			settings.Wide = value == "1"
		case "split":
			if s, err := strconv.ParseFloat(value, 64); err == nil && s >= 10 && s <= 90 {
				settings.Split = s
			}
		case "recent":
			if value != "" && len(recent) < maxRecent {
				recent = append(recent, value)
			}
		}
	}
}

func saveSettings() {
	if settingsFrozen {
		return
	}
	var sb strings.Builder
	sb.WriteString("view=" + settings.View + "\n")
	sb.WriteString("zoom=" + strconv.FormatFloat(settings.Zoom, 'f', 2, 64) + "\n")
	wide := "0"
	if settings.Wide {
		wide = "1"
	}
	sb.WriteString("wide_margins=" + wide + "\n")
	sb.WriteString("split=" + strconv.FormatFloat(settings.Split, 'f', 1, 64) + "\n")
	for _, path := range recent {
		sb.WriteString("recent=" + path + "\n")
	}
	path := settingsPath()
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(sb.String()), 0o644)
}

func settingsJSON() string {
	b, _ := json.Marshal(settings)
	return string(b)
}

// addRecent puts a file at the top of File > Open Recent.
func addRecent(path string) {
	if path == "" {
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	list := []string{path}
	for _, p := range recent {
		if !strings.EqualFold(p, path) && len(list) < maxRecent {
			list = append(list, p)
		}
	}
	recent = list
	rebuildRecentMenu()
	saveSettings()
}

func rebuildRecentMenu() {
	if recentMenu == 0 {
		return
	}
	for {
		count, _, _ := getMenuItemCount.Call(recentMenu)
		if int32(count) <= 0 {
			break
		}
		deleteMenu.Call(recentMenu, 0, mfByPosition)
	}
	if len(recent) == 0 {
		appendMenu(recentMenu, mfString|mfGrayed, idFileRecentBase, "(none)")
		return
	}
	for i, path := range recent {
		// An ampersand in a path would otherwise underline the next letter.
		label := strings.ReplaceAll(path, "&", "&&")
		if i < 9 {
			label = "&" + strconv.Itoa(i+1) + "  " + label
		}
		appendMenu(recentMenu, mfString, uintptr(idFileRecentBase+i), label)
	}
}

func installMainMenu(w webview2.WebView) {
	hwnd := uintptr(w.Window())
	menu, _, _ := createMenu.Call()
	fileMenu, _, _ := createPopupMenu.Call()
	editMenu, _, _ := createPopupMenu.Call()
	viewMenu, _, _ = createPopupMenu.Call()
	formatMenu, _, _ := createPopupMenu.Call()
	helpMenu, _, _ := createPopupMenu.Call()
	recentMenu, _, _ = createPopupMenu.Call()

	appendMenu(fileMenu, mfString, idFileNew, "&New\tCtrl+N")
	appendMenu(fileMenu, mfString, idFileOpen, "&Open...\tCtrl+O")
	appendMenu(fileMenu, mfPopup, recentMenu, "Open &Recent")
	appendMenu(fileMenu, mfSeparator, 0, "")
	appendMenu(fileMenu, mfString, idFileSave, "&Save\tCtrl+S")
	appendMenu(fileMenu, mfString, idFileSaveAs, "Save &As...\tCtrl+Shift+S")
	appendMenu(fileMenu, mfSeparator, 0, "")
	appendMenu(fileMenu, mfString, idFileReveal, "Open Containing &Folder\tCtrl+E")
	appendMenu(fileMenu, mfString, idFileCopyPath, "Copy File &Path\tCtrl+Shift+C")
	appendMenu(fileMenu, mfSeparator, 0, "")
	appendMenu(fileMenu, mfString, idFilePrint, "&Print...\tCtrl+P")
	appendMenu(fileMenu, mfSeparator, 0, "")
	appendMenu(fileMenu, mfString, idFileExit, "E&xit")

	appendMenu(editMenu, mfString, idEditUndo, "&Undo\tCtrl+Z")
	appendMenu(editMenu, mfString, idEditRedo, "&Redo\tCtrl+Y")
	appendMenu(editMenu, mfSeparator, 0, "")
	appendMenu(editMenu, mfString, idEditCut, "Cu&t\tCtrl+X")
	appendMenu(editMenu, mfString, idEditCopy, "&Copy\tCtrl+C")
	appendMenu(editMenu, mfString, idEditPaste, "&Paste\tCtrl+V")
	appendMenu(editMenu, mfSeparator, 0, "")
	appendMenu(editMenu, mfString, idEditAll, "Select &All\tCtrl+A")
	appendMenu(editMenu, mfSeparator, 0, "")
	appendMenu(editMenu, mfString, idEditFind, "&Find...\tCtrl+F")
	appendMenu(editMenu, mfString, idEditFindNext, "Find &Next\tF3")

	appendMenu(viewMenu, mfString, idViewSource, "&Markdown\tCtrl+1")
	appendMenu(viewMenu, mfString, idViewSplit, "S&plit\tCtrl+2")
	appendMenu(viewMenu, mfString, idViewRendered, "&Rendered\tCtrl+3")
	appendMenu(viewMenu, mfSeparator, 0, "")
	appendMenu(viewMenu, mfString, idViewZoomIn, "Zoom &In\tCtrl++")
	appendMenu(viewMenu, mfString, idViewZoomOut, "Zoom &Out\tCtrl+-")
	appendMenu(viewMenu, mfString, idViewZoomReset, "Reset &Zoom")
	appendMenu(viewMenu, mfSeparator, 0, "")
	appendMenu(viewMenu, mfString, idViewWideMargins, "&Wide Margins")

	appendMenu(formatMenu, mfString, idFmtBold, "&Bold\tCtrl+B")
	appendMenu(formatMenu, mfString, idFmtItalic, "&Italic\tCtrl+I")
	appendMenu(formatMenu, mfString, idFmtCode, "&Code\tCtrl+Shift+K")
	appendMenu(formatMenu, mfString, idFmtLink, "&Link...\tCtrl+K")
	appendMenu(formatMenu, mfSeparator, 0, "")
	appendMenu(formatMenu, mfString, idFmtHeading, "&Heading (cycles)\tCtrl+Shift+H")
	appendMenu(formatMenu, mfString, idFmtList, "Bulleted Li&st\tCtrl+Shift+L")

	appendMenu(helpMenu, mfString, idHelpAbout, "&About TinyMD")

	appendMenu(menu, mfPopup, fileMenu, "&File")
	appendMenu(menu, mfPopup, editMenu, "&Edit")
	appendMenu(menu, mfPopup, viewMenu, "&View")
	appendMenu(menu, mfPopup, formatMenu, "F&ormat")
	appendMenu(menu, mfPopup, helpMenu, "&Help")
	setMenu.Call(hwnd, menu)
	rebuildRecentMenu()
	updateViewChecks()
	drawMenuBar.Call(hwnd)
	installMenuWndProc(w)
}

func updateViewChecks() {
	check := func(id uintptr, on bool) {
		flags := uintptr(0)
		if on {
			flags = mfChecked
		}
		checkMenuItem.Call(viewMenu, id, flags)
	}
	check(idViewSource, settings.View == "source")
	check(idViewRendered, settings.View == "rendered")
	check(idViewSplit, settings.View == "split")
	check(idViewWideMargins, settings.Wide)
}

// menuCommands maps menu ids onto the page's command names.
var menuCommands = map[uintptr]string{
	idFileNew: "new", idFileOpen: "open", idFileSave: "save", idFileSaveAs: "saveAs",
	idFilePrint: "print", idFileReveal: "reveal",
	idEditUndo: "undo", idEditRedo: "redo", idEditCut: "cut", idEditCopy: "copy",
	idEditPaste: "paste", idEditAll: "selectAll", idEditFind: "find", idEditFindNext: "findNext",
	idViewSource: "viewSource", idViewRendered: "viewRendered", idViewSplit: "split",
	idViewZoomIn: "zoomIn", idViewZoomOut: "zoomOut", idViewZoomReset: "zoomReset",
	idViewWideMargins: "wide",
	idFmtBold:         "bold", idFmtItalic: "italic", idFmtHeading: "heading", idFmtList: "list",
	idFmtLink: "link", idFmtCode: "code",
}

func handleMenuCommand(w webview2.WebView, id uintptr) bool {
	var script string
	switch {
	case id >= idFileRecentBase && id < idFileRecentBase+maxRecent:
		script = "window.tinyMdMenuCommand && window.tinyMdMenuCommand('recent', " + strconv.Itoa(int(id-idFileRecentBase)) + ");"
	case id == idFileExit:
		postMessageW.Call(uintptr(w.Window()), wmClose, 0, 0)
		return true
	case id == idFileCopyPath:
		showCopyPathResult(uintptr(w.Window()))
		return true
	case id == idHelpAbout:
		showAboutDialog(uintptr(w.Window()))
		return true
	default:
		name, ok := menuCommands[id]
		if !ok {
			return false
		}
		script = "window.tinyMdMenuCommand && window.tinyMdMenuCommand('" + name + "');"
	}
	w.Dispatch(func() {
		w.Eval(script)
	})
	return true
}

// handleClose asks about unsaved changes before the window goes. It reports
// whether the close should go ahead now.
func handleClose(w webview2.WebView) bool {
	if forceClose || !dirty {
		return true
	}
	name := "Untitled"
	if currentFile != "" {
		name = filepath.Base(currentFile)
	}
	switch messageBox(uintptr(w.Window()), "Save changes to "+name+"?", "TinyMD", mbYesNoCancel|mbIconWarning) {
	case idYes:
		// The page holds the text; it saves, then asks to close again.
		w.Dispatch(func() {
			w.Eval("window.tinyMdMenuCommand && window.tinyMdMenuCommand('saveThenClose');")
		})
		return false
	case idNo:
		return true
	}
	return false
}

func messageBox(hwnd uintptr, text, title string, flags uintptr) uintptr {
	body := utf16From(text)
	caption := utf16From(title)
	ret, _, _ := messageBoxW.Call(hwnd, uintptr(unsafe.Pointer(&body[0])), uintptr(unsafe.Pointer(&caption[0])), flags)
	return ret
}

func copyCurrentPath(hwnd uintptr) string {
	if currentFile == "" {
		return "Save the file first"
	}
	path, err := filepath.Abs(currentFile)
	if err != nil {
		return err.Error()
	}
	if err := writeClipboardText(hwnd, path); err != nil {
		return err.Error()
	}
	return "Path copied"
}

func showCopyPathResult(hwnd uintptr) {
	if result := copyCurrentPath(hwnd); result != "Path copied" {
		messageBox(hwnd, result, "Copy File Path", mbOk|mbIconError)
	}
}

// readDocument loads a file for the editor and notes its line endings.
func readDocument(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	fileCRLF = strings.Contains(content, "\r\n")
	return content, nil
}

// documentBytes is the editor's text as the file should hold it.
func documentBytes(content string) []byte {
	if fileCRLF {
		content = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\n", "\r\n")
	}
	return []byte(content)
}

// bindShell adds the page functions behind New, Open Recent, the modified marker,
// the save prompt and settings.
func bindShell(w webview2.WebView) {
	w.Bind("goSetDirty", func(isDirty bool) {
		dirty = isDirty
		w.SetTitle(windowTitle())
	})
	w.Bind("goNewFile", func() string {
		currentFile = ""
		fileCRLF = false
		dirty = false
		w.SetTitle(windowTitle())
		return "ok"
	})
	w.Bind("goConfirmDiscard", func() string {
		name := "Untitled"
		if currentFile != "" {
			name = filepath.Base(currentFile)
		}
		switch messageBox(uintptr(w.Window()), "Save changes to "+name+"?", "TinyMD", mbYesNoCancel|mbIconWarning) {
		case idYes:
			return "yes"
		case idNo:
			return "no"
		}
		return "cancel"
	})
	w.Bind("goOpenRecent", func(index int) map[string]string {
		if index < 0 || index >= len(recent) {
			return map[string]string{"error": "no such recent file"}
		}
		path := recent[index]
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
	w.Bind("goCloseWindow", func() {
		forceClose = true
		postMessageW.Call(uintptr(w.Window()), wmClose, 0, 0)
	})
	w.Bind("goCopyPath", func() string {
		return copyCurrentPath(uintptr(w.Window()))
	})
	w.Bind("goSaveSettings", func(raw string) {
		var next pageSettings
		if json.Unmarshal([]byte(raw), &next) != nil {
			return
		}
		settings = next
		updateViewChecks()
		saveSettings()
	})
}

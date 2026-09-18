package main

import (
	"runtime"
	"testing"
	"unsafe"
)

// childRect returns a child window's position relative to its parent.
func childRect(test *testing.T, child, parent uintptr) (x, y, w, h int32) {
	test.Helper()
	var rc [16]byte
	getWindowRect := user32.NewProc("GetWindowRect")
	getWindowRect.Call(child, uintptr(unsafe.Pointer(&rc[0])))
	left := *(*int32)(unsafe.Pointer(&rc[0]))
	top := *(*int32)(unsafe.Pointer(&rc[4]))
	right := *(*int32)(unsafe.Pointer(&rc[8]))
	bottom := *(*int32)(unsafe.Pointer(&rc[12]))

	var prc [16]byte
	getWindowRect.Call(parent, uintptr(unsafe.Pointer(&prc[0])))
	return left - *(*int32)(unsafe.Pointer(&prc[0])),
		top - *(*int32)(unsafe.Pointer(&prc[4])),
		right - left, bottom - top
}

// setupPanes builds a parent with the two panes as children, the arrangement
// layoutPanes is responsible for.
func setupPanes(test *testing.T) uintptr {
	test.Helper()
	runtime.LockOSThread()
	test.Cleanup(runtime.UnlockOSThread)

	class := utf16From("STATIC")
	parent, _, err := createWindowExW.Call(0, uintptr(unsafe.Pointer(&class[0])), 0,
		WS_OVERLAPPEDWINDOW, 0, 0, 1000, 600, 0, 0, 0, 0)
	if parent == 0 {
		test.Fatal(err)
	}
	test.Cleanup(func() { destroyWindowProc.Call(parent) })

	for _, target := range []*uintptr{&editorHwnd, &previewHwnd} {
		child, _, childErr := createWindowExW.Call(0, uintptr(unsafe.Pointer(&class[0])), 0,
			WS_CHILD|WS_VISIBLE, 0, 0, 10, 10, parent, 0, 0, 0)
		if child == 0 {
			test.Fatal(childErr)
		}
		*target = child
		test.Cleanup(func() { destroyWindowProc.Call(child) })
	}
	previous, previousRestore := splitRatio, restoreSplitRatio
	test.Cleanup(func() { splitRatio, restoreSplitRatio = previous, previousRestore })
	return parent
}

// TestLayoutPanesPositionsBothPanes is the regression test for a window that opened
// showing nothing: layoutPanes read the mainHwnd global, which is only assigned once
// CreateWindowEx returns, while WM_SIZE arrives during creation. With no window to
// measure it returned early and neither pane was ever given a size.
func TestLayoutPanesPositionsBothPanes(test *testing.T) {
	parent := setupPanes(test)
	splitRatio = 0.5

	layoutPanes(parent)

	_, _, editorW, editorH := childRect(test, editorHwnd, parent)
	previewX, _, previewW, _ := childRect(test, previewHwnd, parent)
	if editorW <= 0 || editorH <= 0 {
		test.Fatalf("editor was not sized: %dx%d", editorW, editorH)
	}
	if previewW <= 0 {
		test.Fatalf("preview was not sized: width %d", previewW)
	}
	if previewX <= editorW {
		test.Fatalf("preview at x=%d overlaps an editor %d wide", previewX, editorW)
	}
}

func TestLayoutPanesBeforeTheWindowGlobalIsSet(test *testing.T) {
	parent := setupPanes(test)
	splitRatio = 0.5

	saved := mainHwnd
	mainHwnd = 0 // as it is while CreateWindowEx is still running
	defer func() { mainHwnd = saved }()

	layoutPanes(parent)

	if _, _, editorW, _ := childRect(test, editorHwnd, parent); editorW <= 0 {
		test.Fatal("panes were not laid out when only the message's window was available")
	}
}

// paneIsVisible reads the window's own WS_VISIBLE bit. IsWindowVisible would be
// false for every child here, because the test's parent window is never shown.
func paneIsVisible(hwnd uintptr) bool {
	const (
		GWL_STYLE  = ^uintptr(15) // -16
		WS_VISIBLE = 0x10000000
	)
	getWindowLongPtrW := user32.NewProc("GetWindowLongPtrW")
	style, _, _ := getWindowLongPtrW.Call(hwnd, GWL_STYLE)
	return style&WS_VISIBLE != 0
}

func TestLayoutPanesHidesACollapsedPane(test *testing.T) {
	parent := setupPanes(test)

	splitRatio = 0 // editor collapsed
	layoutPanes(parent)
	if paneIsVisible(editorHwnd) {
		test.Fatal("a collapsed editor was left visible")
	}
	if !paneIsVisible(previewHwnd) {
		test.Fatal("the preview was hidden when the editor collapsed")
	}

	splitRatio = 1 // preview collapsed
	layoutPanes(parent)
	if paneIsVisible(previewHwnd) {
		test.Fatal("a collapsed preview was left visible")
	}
	if !paneIsVisible(editorHwnd) {
		test.Fatal("the editor was hidden when the preview collapsed")
	}
}

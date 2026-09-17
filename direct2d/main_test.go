package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	extast "github.com/yuin/goldmark/extension/ast"
)

func TestAbsoluteFilePathRejectsUntitledDocument(t *testing.T) {
	if _, err := absoluteFilePath(""); err == nil {
		t.Fatal("expected an error for an untitled document")
	}
}

func setupPreviewTest(test *testing.T) {
	test.Helper()
	runtime.LockOSThread()
	test.Cleanup(runtime.UnlockOSThread)
	for _, window := range []struct {
		class  string
		handle *uintptr
	}{
		{"EDIT", &editorHwnd},
		{"STATIC", &previewHwnd},
	} {
		class := utf16From(window.class)
		handle, _, err := createWindowExW.Call(0, uintptr(unsafe.Pointer(&class[0])), 0,
			WS_OVERLAPPEDWINDOW|ES_MULTILINE, 0, 0, 700, 800, 0, 0, 0, 0)
		if handle == 0 {
			test.Fatal(err)
		}
		*window.handle = handle
		test.Cleanup(func() { destroyWindowProc.Call(handle) })
	}
	if !initD2D(previewHwnd) {
		test.Fatal("Direct2D initialization failed")
	}
	test.Cleanup(func() {
		discardDeviceResources()
		comRelease(res.dwFactory)
		comRelease(res.factory)
		ole32.NewProc("CoUninitialize").Call()
		res = d2dResources{}
		currentBlocks = nil
		scrollY = 0
		totalHeight = 0
		currentFile = ""
		lineEnding = "\n"
	})
}

func TestPreviewRemainsVisibleAfterDocumentShrinks(test *testing.T) {
	setupPreviewTest(test)
	for _, content := range []string{strings.Repeat("Paragraph\r\n\r\n", 300), "# Short document\r\n\r\nStill visible."} {
		encoded := utf16From(content)
		sendMessageW.Call(editorHwnd, WM_SETTEXT, 0, uintptr(unsafe.Pointer(&encoded[0])))
		updatePreview()
		renderPreview(previewHwnd)
		if totalHeight > 800 {
			scrollY = totalHeight - 800
		}
	}
	if scrollY != 0 {
		test.Fatalf("short document is blank: scroll offset %v exceeds document height %v", scrollY, totalHeight)
	}
	if len(currentBlocks) != 2 || currentBlocks[0].Height <= 0 {
		test.Fatal("short document did not receive a text layout")
	}
}

func TestLoadFilePreservesLinesAndSaveBytes(test *testing.T) {
	setupPreviewTest(test)
	for _, ending := range []string{"\n", "\r\n"} {
		content := strings.Join([]string{"# **Heading** — 日本語", "", "Source → destination.", "", "| Name | Value |", "| --- | --- |", "| café | **bold** |", ""}, ending)
		path := filepath.Join(test.TempDir(), "日本語 document.md")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			test.Fatal(err)
		}
		scrollY = 8000
		if err := loadFile(path); err != nil {
			test.Fatal(err)
		}
		if scrollY != 0 {
			test.Fatal("opening a file retained the previous document's scroll offset")
		}
		lines, _, _ := sendMessageW.Call(editorHwnd, 0x00BA, 0, 0)
		if lines != 8 {
			test.Fatalf("editor displays %d lines, want 8", lines)
		}
		renderPreview(previewHwnd)
		if len(currentBlocks) != 3 || currentBlocks[2].Type != blockTable || currentBlocks[0].Height <= 0 {
			test.Fatalf("Markdown did not render as heading, paragraph and table: %+v", currentBlocks)
		}
		saveFile()
		saved, err := os.ReadFile(path)
		if err != nil || string(saved) != content {
			test.Fatalf("saving changed document bytes: %q, %v", saved, err)
		}
	}
}

func TestLoadFileFailurePreservesDocument(test *testing.T) {
	setupPreviewTest(test)
	path := filepath.Join(test.TempDir(), "valid.md")
	if err := os.WriteFile(path, []byte("# Keep this\n"), 0600); err != nil {
		test.Fatal(err)
	}
	if err := loadFile(path); err != nil {
		test.Fatal(err)
	}
	invalidPath := filepath.Join(test.TempDir(), "invalid.md")
	if err := os.WriteFile(invalidPath, []byte("bad\x00text"), 0600); err != nil {
		test.Fatal(err)
	}
	for _, failedPath := range []string{path + ".missing", invalidPath} {
		if err := loadFile(failedPath); err == nil {
			test.Fatalf("expected an explicit error for %s", failedPath)
		}
		if currentFile != path || documentText() != "# Keep this\n" {
			test.Fatal("failed open replaced the existing document")
		}
	}
}

func TestLoadFileLargeAndEmptyDocuments(test *testing.T) {
	setupPreviewTest(test)
	for _, content := range []string{strings.Repeat("## Heading — **bold**\n\nSource → destination.\n\n", 400), ""} {
		path := filepath.Join(test.TempDir(), "document.md")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			test.Fatal(err)
		}
		if err := loadFile(path); err != nil {
			test.Fatal(err)
		}
		if documentText() != content {
			test.Fatal("document was truncated or altered")
		}
		renderPreview(previewHwnd)
		if content == "" && (len(currentBlocks) != 0 || scrollY != 0) {
			test.Fatal("empty document retained old content or scrolling")
		}
		if content != "" && (len(currentBlocks) != 800 || totalHeight <= 800) {
			test.Fatalf("large document failed to render: %d blocks, height %v", len(currentBlocks), totalHeight)
		}
	}
}

func TestAbsoluteFilePathPreservesUnicodeAndSpaces(t *testing.T) {
	input := filepath.Join("test data", "Příliš žluťoučký kůň.md")
	got, err := absoluteFilePath(input)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("path is not absolute: %q", got)
	}
	if !strings.HasSuffix(got, input) {
		t.Fatalf("path lost Unicode or spaces: %q", got)
	}
}

func TestUTF16ClipboardPayloadRoundTripsMultilineMarkdown(t *testing.T) {
	input := "# Přehled\r\n\r\n- café ☕\r\n- 日本語"
	encoded := utf16From(input)
	if got := syscall.UTF16ToString(encoded); got != input {
		t.Fatalf("round trip mismatch: %q", got)
	}
}

func totalWidth(colWidths []float32) float32 {
	var total float32
	for _, width := range colWidths {
		total += width
	}
	return total
}

func TestFitColumnsToWidthLeavesTablesThatAlreadyFit(t *testing.T) {
	colWidths := []float32{100, 150, 80}
	fitColumnsToWidth(colWidths, 600, 8)
	for index, want := range []float32{100, 150, 80} {
		if colWidths[index] != want {
			t.Fatalf("column %d was resized to %v, expected %v", index, colWidths[index], want)
		}
	}
}

func TestFitColumnsToWidthShrinksTablesThatOverflow(t *testing.T) {
	// A prose table measured at 1400pt in a 600pt pane: without this it ran off
	// the right of the preview and the text was clipped.
	colWidths := []float32{200, 1200}
	fitColumnsToWidth(colWidths, 600, 8)
	if got := totalWidth(colWidths); got > 600.5 {
		t.Fatalf("table still overflows: total %v for an available width of 600", got)
	}
}

func TestFitColumnsToWidthKeepsNarrowColumnsNarrow(t *testing.T) {
	// The wide prose column should give up the space, not the short label column.
	colWidths := []float32{60, 1200}
	fitColumnsToWidth(colWidths, 600, 8)
	if colWidths[0] != 60 {
		t.Fatalf("narrow column was squeezed to %v, expected it to keep 60", colWidths[0])
	}
	if colWidths[1] >= 1200 {
		t.Fatalf("wide column was not shrunk: %v", colWidths[1])
	}
	if got := totalWidth(colWidths); got > 600.5 {
		t.Fatalf("table still overflows: total %v", got)
	}
}

func TestFitColumnsToWidthSharesSpaceBetweenEquallyWideColumns(t *testing.T) {
	colWidths := []float32{900, 900}
	fitColumnsToWidth(colWidths, 600, 8)
	if difference := colWidths[0] - colWidths[1]; difference > 0.5 || difference < -0.5 {
		t.Fatalf("columns of equal demand were given unequal widths: %v and %v",
			colWidths[0], colWidths[1])
	}
}

func TestFitColumnsToWidthSurvivesAVeryNarrowPane(t *testing.T) {
	// Dragging the splitter almost shut must not produce zero or negative widths.
	colWidths := []float32{300, 300, 300, 300}
	fitColumnsToWidth(colWidths, 40, 8)
	if got := totalWidth(colWidths); got > 40.5 {
		t.Fatalf("table overflows a 40pt pane: total %v", got)
	}
	for index, width := range colWidths {
		if width <= 0 {
			t.Fatalf("column %d collapsed to %v", index, width)
		}
	}
}

func TestFitColumnsToWidthIgnoresAnEmptyTable(t *testing.T) {
	fitColumnsToWidth(nil, 600, 8)       // must not panic
	fitColumnsToWidth([]float32{}, 0, 8) // nor with no width to give
}

func TestTextAlignmentForMapsMarkdownAlignment(t *testing.T) {
	for _, testCase := range []struct {
		alignment extast.Alignment
		want      uint32
	}{
		{extast.AlignLeft, 0},
		{extast.AlignRight, 1},
		{extast.AlignCenter, 2},
		{extast.AlignNone, 0},
	} {
		if got := textAlignmentFor(testCase.alignment); got != testCase.want {
			t.Fatalf("alignment %v mapped to %d, expected %d", testCase.alignment, got, testCase.want)
		}
	}
}

func TestTableLayoutCapturesColumnAlignment(t *testing.T) {
	blocks := markdownToLayout([]byte("| Left | Centre | Right |\n|:-----|:------:|------:|\n| a | b | c |\n"))
	var table *TableData
	for index := range blocks {
		if blocks[index].Type == blockTable {
			table = blocks[index].Table
			break
		}
	}
	if table == nil {
		t.Fatal("no table block was produced")
	}
	for index, want := range []uint32{0, 2, 1} {
		if table.Aligns[index] != want {
			t.Fatalf("column %d alignment %d, expected %d", index, table.Aligns[index], want)
		}
	}
}

func TestPaneGeometrySplitsEvenly(t *testing.T) {
	editorW, dividerX, previewX, previewW := paneGeometry(1000, 800, 0.5)
	if editorW != 500 || dividerX != 500 || previewX != 500+dividerWidth {
		t.Fatalf("even split placed panes at editor=%d divider=%d preview=%d",
			editorW, dividerX, previewX)
	}
	if editorW+dividerWidth+previewW != 1000 {
		t.Fatalf("panes and divider do not fill the window: %d + %d + %d",
			editorW, dividerWidth, previewW)
	}
}

func TestPaneGeometryCollapsesTheEditor(t *testing.T) {
	editorW, _, previewX, previewW := paneGeometry(1000, 800, 0)
	if editorW != 0 || previewX != 0 || previewW != 1000 {
		t.Fatalf("collapsing the editor left editor=%d preview=%d wide at x=%d",
			editorW, previewW, previewX)
	}
}

func TestPaneGeometryCollapsesThePreview(t *testing.T) {
	editorW, _, _, previewW := paneGeometry(1000, 800, 1)
	if editorW != 1000 || previewW != 0 {
		t.Fatalf("collapsing the preview left editor=%d preview=%d", editorW, previewW)
	}
}

func TestPaneGeometryKeepsTheDividerReachable(t *testing.T) {
	// A drag to the far edge must not push the splitter out of the window, or it
	// could never be grabbed again.
	editorW, dividerX, _, _ := paneGeometry(1000, 800, 0.999)
	if dividerX > 1000-dividerWidth {
		t.Fatalf("divider at %d is off the right edge of a 1000pt window", dividerX)
	}
	if editorW < 0 {
		t.Fatalf("negative editor width %d", editorW)
	}
}

func TestPaneGeometryHandlesAZeroWidthWindow(t *testing.T) {
	editorW, _, _, previewW := paneGeometry(0, 0, 0.5) // minimised
	if editorW != 0 || previewW != 0 {
		t.Fatalf("zero width window produced editor=%d preview=%d", editorW, previewW)
	}
}

func TestSplitRatioAtSnapsToACollapseNearTheEdges(t *testing.T) {
	if got := splitRatioAt(4, 1000); got != 0 {
		t.Fatalf("dragging to the left edge gave %v, expected a collapsed editor", got)
	}
	if got := splitRatioAt(996, 1000); got != 1 {
		t.Fatalf("dragging to the right edge gave %v, expected a collapsed preview", got)
	}
	if got := splitRatioAt(500, 1000); got != 0.5 {
		t.Fatalf("dragging to the middle gave %v, expected 0.5", got)
	}
}

func TestToggleCollapseRestoresThePreviousSplit(t *testing.T) {
	defer func() { splitRatio, restoreSplitRatio = 0.5, 0.5 }()

	setSplitRatio(0.7)
	toggleCollapse(true) // collapse the editor
	if splitRatio != 0 {
		t.Fatalf("editor did not collapse: ratio %v", splitRatio)
	}
	toggleCollapse(true) // same command again brings it back
	if splitRatio != 0.7 {
		t.Fatalf("restoring gave %v, expected the previous 0.7", splitRatio)
	}

	toggleCollapse(false) // collapse the preview
	if splitRatio != 1 {
		t.Fatalf("preview did not collapse: ratio %v", splitRatio)
	}
	toggleCollapse(false)
	if splitRatio != 0.7 {
		t.Fatalf("restoring gave %v, expected the previous 0.7", splitRatio)
	}
}

func TestSetSplitRatioClampsOutOfRangeValues(t *testing.T) {
	defer func() { splitRatio, restoreSplitRatio = 0.5, 0.5 }()
	setSplitRatio(-2)
	if splitRatio != 0 {
		t.Fatalf("negative ratio became %v", splitRatio)
	}
	setSplitRatio(4)
	if splitRatio != 1 {
		t.Fatalf("oversized ratio became %v", splitRatio)
	}
}

func TestOverDividerFindsTheGrabStrip(t *testing.T) {
	defer func() { splitRatio, restoreSplitRatio = 0.5, 0.5 }()

	splitRatio = 0.5
	if !overDivider(502, 1000) {
		t.Fatal("the splitter was not grabbable at its own position")
	}
	if overDivider(200, 1000) || overDivider(800, 1000) {
		t.Fatal("clicking inside a pane was treated as grabbing the splitter")
	}

	// Collapsed panes still need a strip to drag back out.
	splitRatio = 0
	if !overDivider(2, 1000) {
		t.Fatal("a collapsed editor left no grab strip at the left edge")
	}
	splitRatio = 1
	if !overDivider(998, 1000) {
		t.Fatal("a collapsed preview left no grab strip at the right edge")
	}
}

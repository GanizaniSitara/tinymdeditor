package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"
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

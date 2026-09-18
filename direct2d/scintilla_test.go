package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// setupScintilla unpacks the embedded control and creates one, skipping the test if
// the DLLs cannot be loaded on this machine.
func setupScintilla(test *testing.T) {
	test.Helper()
	runtime.LockOSThread()
	test.Cleanup(runtime.UnlockOSThread)

	if err := loadScintilla(); err != nil {
		test.Skip("Scintilla unavailable:", err)
	}
	class := utf16From("Scintilla")
	hwnd, _, err := createWindowExW.Call(0, uintptr(unsafe.Pointer(&class[0])), 0,
		WS_OVERLAPPEDWINDOW, 0, 0, 700, 800, 0, 0, 0, 0)
	if hwnd == 0 {
		test.Fatal("could not create a Scintilla window:", err)
	}
	previousHwnd, previousFlag := editorHwnd, usingScintilla
	editorHwnd, usingScintilla = hwnd, true
	test.Cleanup(func() {
		destroyWindowProc.Call(hwnd)
		editorHwnd, usingScintilla = previousHwnd, previousFlag
	})
	configureEditor()
}

func TestScintillaRoundTripsUnicodeAndLineEndings(test *testing.T) {
	setupScintilla(test)
	// Scintilla stores UTF-8 and keeps whatever line endings it is given, which is
	// what lets saving return the document's original bytes.
	for _, content := range []string{
		"# Heading\n\nParagraph with café, 日本語 and an em dash —.\n",
		"Line one\r\nLine two\r\n",
	} {
		setScintillaText([]byte(content))
		if got := string(scintillaText()); got != content {
			test.Fatalf("round trip changed the document:\n want %q\n  got %q", content, got)
		}
	}
}

func TestScintillaReportsAnEmptyDocument(test *testing.T) {
	setupScintilla(test)
	setScintillaText(nil)
	if got := scintillaText(); len(got) != 0 {
		test.Fatalf("empty document came back as %q", got)
	}
}

func TestScintillaShowsLineNumbers(test *testing.T) {
	setupScintilla(test)
	if width := sci(SCI_GETMARGINWIDTHN, 0, 0); width <= 0 {
		test.Fatalf("the line number margin has no width: %d", width)
	}
	if kind := sci(SCI_GETMARGINTYPEN, 0, 0); kind != SC_MARGIN_NUMBER {
		test.Fatalf("margin 0 is type %d, expected a line number margin", kind)
	}
}

func TestScintillaAppliesTheMarkdownLexer(test *testing.T) {
	setupScintilla(test)
	if createLexer == 0 {
		test.Skip("Lexilla did not expose CreateLexer")
	}
	name := sci(SCI_GETLEXERLANGUAGE, 0, 0)
	if name == 0 {
		test.Skip("this Scintilla does not report the lexer language")
	}
	buf := make([]byte, name+1)
	sci(SCI_GETLEXERLANGUAGE, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	if got := strings.TrimRight(string(buf), "\x00"); got != "markdown" {
		test.Fatalf("lexer is %q, expected markdown", got)
	}
}

func TestEditorTextGoesThroughScintillaWhenItIsInUse(test *testing.T) {
	setupScintilla(test)
	setDocument("# Through the shared path\n")
	if got := editorText(); got != "# Through the shared path\n" {
		test.Fatalf("editorText returned %q", got)
	}
}

func TestSavingThroughScintillaPreservesTheDocumentBytes(test *testing.T) {
	setupScintilla(test)
	previousFile, previousEnding := currentFile, lineEnding
	test.Cleanup(func() { currentFile, lineEnding = previousFile, previousEnding })

	for _, ending := range []string{"\n", "\r\n"} {
		content := strings.Join([]string{"# Heading — 日本語", "", "Body with café.", ""}, ending)
		path := filepath.Join(test.TempDir(), "document.md")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			test.Fatal(err)
		}
		if err := loadFile(path); err != nil {
			test.Fatal(err)
		}
		saveFile()
		saved, err := os.ReadFile(path)
		if err != nil {
			test.Fatal(err)
		}
		if string(saved) != content {
			test.Fatalf("saving changed the bytes:\n want %q\n  got %q", content, saved)
		}
	}
}

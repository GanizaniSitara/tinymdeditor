package main

import (
	"os"
	"strings"
	"testing"
	"unsafe"
)

// sampleDocument builds a document of roughly the size the editor is actually used
// on: long prose with headings, lists, code and tables.
func sampleDocument(sections int) string {
	var builder strings.Builder
	for i := 0; i < sections; i++ {
		builder.WriteString("## Section heading that runs on a little\n\n")
		builder.WriteString("Prose paragraph with **bold**, *italic* and `inline code` in it, long ")
		builder.WriteString("enough to wrap across the pane more than once in normal use.\n\n")
		builder.WriteString("- First bullet\n- Second bullet\n- Third bullet\n\n")
		builder.WriteString("| Column | Drives |\n|---|---|\n")
		builder.WriteString("| `StatusRAG` | red, amber or green, colouring the Strategy layer |\n")
		builder.WriteString("| `Resilience` | 0-4, colouring the Resilience layer |\n\n")
		builder.WriteString("```go\nfunc main() { fmt.Println(\"hello\") }\n```\n\n")
	}
	return builder.String()
}

func BenchmarkMarkdownToLayout(b *testing.B) {
	document := []byte(sampleDocument(40))
	b.SetBytes(int64(len(document)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		currentBlocks = markdownToLayout(document)
	}
}

// BenchmarkRenderPreview measures one repaint, which is what every keystroke,
// scroll notch and resize costs.
func BenchmarkRenderPreview(b *testing.B) {
	setupPreviewTest(b)
	encoded := utf16From(sampleDocument(40))
	sendMessageW.Call(editorHwnd, WM_SETTEXT, 0, uintptr(unsafe.Pointer(&encoded[0])))
	updatePreview()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderPreview(previewHwnd)
	}
}

// BenchmarkRenderPreviewRealDocument uses a document from the task corpus when it is
// present, so the numbers reflect a real file rather than a generated one.
func BenchmarkRenderPreviewRealDocument(b *testing.B) {
	path := os.Getenv("TINYMD_BENCH_DOC")
	if path == "" {
		b.Skip("set TINYMD_BENCH_DOC to a Markdown file to benchmark it")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		b.Skip(err)
	}
	setupPreviewTest(b)
	encoded := utf16From(strings.ReplaceAll(string(content), "\n", "\r\n"))
	sendMessageW.Call(editorHwnd, WM_SETTEXT, 0, uintptr(unsafe.Pointer(&encoded[0])))
	updatePreview()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderPreview(previewHwnd)
	}
}

// BenchmarkRenderPreviewTinyDocument establishes the fixed cost of a repaint: if a
// three-line document costs the same as a long one, the time is in presenting the
// frame rather than in laying out the content.
func BenchmarkRenderPreviewTinyDocument(b *testing.B) {
	setupPreviewTest(b)
	encoded := utf16From("# Title\r\n\r\nOne short paragraph.\r\n")
	sendMessageW.Call(editorHwnd, WM_SETTEXT, 0, uintptr(unsafe.Pointer(&encoded[0])))
	updatePreview()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderPreview(previewHwnd)
	}
}

package main

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestAbsoluteFilePathRejectsUntitledDocument(t *testing.T) {
	if _, err := absoluteFilePath(""); err == nil {
		t.Fatal("expected an error for an untitled document")
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

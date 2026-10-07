package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenLink(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "link.log")
	t.Setenv("TINYMD_LINK_LOG", logPath)

	oldCurrent := currentFile
	t.Cleanup(func() {
		currentFile = oldCurrent
	})
	currentFile = filepath.Join(tempDir, "current.md")
	if err := os.WriteFile(currentFile, []byte("# Current file\n"), 0644); err != nil {
		t.Fatalf("failed to create current file: %v", err)
	}

	otherMd := filepath.Join(tempDir, "other.md")
	if err := os.WriteFile(otherMd, []byte("# Other\n"), 0644); err != nil {
		t.Fatalf("failed to create other.md: %v", err)
	}

	subDir := filepath.Join(tempDir, "sub dir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}
	subXMd := filepath.Join(subDir, "x.md")
	if err := os.WriteFile(subXMd, []byte("# Sub X\n"), 0644); err != nil {
		t.Fatalf("failed to create sub dir/x.md: %v", err)
	}

	notesTxt := filepath.Join(tempDir, "notes.txt")
	if err := os.WriteFile(notesTxt, []byte("notes text\n"), 0644); err != nil {
		t.Fatalf("failed to create notes.txt: %v", err)
	}

	tests := []struct {
		name          string
		href          string
		wantStatus    string
		wantLogType   string // "open", "tinymd", or ""
		wantLogTarget string
	}{
		{
			name:          "https URL",
			href:          "https://example.com/docs",
			wantStatus:    "opened",
			wantLogType:   "open",
			wantLogTarget: "https://example.com/docs",
		},
		{
			name:          "MAILTO",
			href:          "MAILTO:test@example.com",
			wantStatus:    "opened",
			wantLogType:   "open",
			wantLogTarget: "MAILTO:test@example.com",
		},
		{
			name:        "javascript scheme",
			href:        "javascript:alert(1)",
			wantStatus:  "blocked",
			wantLogType: "",
		},
		{
			name:        "data scheme",
			href:        "data:text/html,x",
			wantStatus:  "blocked",
			wantLogType: "",
		},
		{
			name:        "empty href",
			href:        "",
			wantStatus:  "blocked",
			wantLogType: "",
		},
		{
			name:          "other.md",
			href:          "other.md",
			wantStatus:    "opened",
			wantLogType:   "tinymd",
			wantLogTarget: otherMd,
		},
		{
			name:          "sub%20dir/x.md",
			href:          "sub%20dir/x.md",
			wantStatus:    "opened",
			wantLogType:   "tinymd",
			wantLogTarget: subXMd,
		},
		{
			name:          "other.md#section",
			href:          "other.md#section",
			wantStatus:    "opened",
			wantLogType:   "tinymd",
			wantLogTarget: otherMd,
		},
		{
			name:        "missing.md",
			href:        "missing.md",
			wantStatus:  "not found",
			wantLogType: "",
		},
		{
			name:        "notes.txt",
			href:        "notes.txt",
			wantStatus:  "blocked",
			wantLogType: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(logPath)

			got := openLink(tc.href)
			if got != tc.wantStatus {
				t.Errorf("openLink(%q) = %q; want %q", tc.href, got, tc.wantStatus)
			}

			switch tc.wantLogType {
			case "":
				if data, err := os.ReadFile(logPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
					t.Errorf("openLink(%q) expected no log line, got: %q", tc.href, string(data))
				}
			case "open":
				data, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatalf("openLink(%q) expected log file, got error: %v", tc.href, err)
				}
				lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
				if len(lines) != 1 {
					t.Errorf("openLink(%q) expected 1 log line, got %d: %q", tc.href, len(lines), string(data))
				}
				line := strings.TrimRight(lines[0], "\r")
				wantLine := "open " + tc.wantLogTarget
				if !strings.EqualFold(line, wantLine) {
					t.Errorf("openLink(%q) logged %q; want %q", tc.href, line, wantLine)
				}
			case "tinymd":
				data, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatalf("openLink(%q) expected log file, got error: %v", tc.href, err)
				}
				lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
				if len(lines) != 1 {
					t.Errorf("openLink(%q) expected 1 log line, got %d: %q", tc.href, len(lines), string(data))
				}
				line := strings.TrimRight(lines[0], "\r")
				prefix := "tinymd "
				if !strings.HasPrefix(strings.ToLower(line), prefix) {
					t.Fatalf("openLink(%q) logged %q; want prefix %q", tc.href, line, prefix)
				}
				gotPath := strings.TrimSpace(line[len(prefix):])
				cleanGot := filepath.Clean(gotPath)
				cleanWant := filepath.Clean(tc.wantLogTarget)
				if !strings.EqualFold(cleanGot, cleanWant) {
					t.Errorf("openLink(%q) logged path %q (cleaned: %q); want %q (cleaned: %q)",
						tc.href, gotPath, cleanGot, tc.wantLogTarget, cleanWant)
				}
			}
		})
	}
}

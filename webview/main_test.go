package main

import "testing"

func TestHookedEditorShortcutLeavesCopyToWebView(t *testing.T) {
	if key, ok := hookedEditorShortcut('C'); ok {
		t.Fatalf("Ctrl+C must reach WebView for rendered selections; got %q", key)
	}
}

func TestHookedEditorShortcutClaimsEditorMutations(t *testing.T) {
	for virtualKey, want := range map[uintptr]string{'A': "a", 'V': "v", 'X': "x"} {
		if got, ok := hookedEditorShortcut(virtualKey); !ok || got != want {
			t.Errorf("hookedEditorShortcut(%q) = %q, %v; want %q, true", virtualKey, got, ok, want)
		}
	}
}

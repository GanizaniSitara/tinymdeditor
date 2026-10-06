package main

// --selftest OUT runs the rendered-editing checks in selftest.js inside the real
// WebView2 page, with the window hidden, writes the results to OUT and exits. It
// needs no input devices and never takes the foreground.

import (
	_ "embed"
	"os"
	"time"

	"github.com/jchv/go-webview2"
)

//go:embed selftest.js
var selftestJS string

func startSelftest(w webview2.WebView, out string) {
	settingsFrozen = true
	showWindowProc.Call(uintptr(w.Window()), 0) // SW_HIDE
	w.Bind("goSelftestDone", func(result string) {
		os.WriteFile(out, []byte(result), 0o644)
		w.Dispatch(w.Terminate)
	})
	w.Init(selftestJS)
	go func() {
		time.Sleep(90 * time.Second)
		os.WriteFile(out, []byte(`{"error":"self-test timed out"}`), 0o644)
		os.Exit(2)
	}()
}

# TinyMD — Markdown Editor for Windows

A small Markdown editor for Windows built on WebView2, with a source pane, a
rendered pane you can edit directly, or both side by side. The code lives in
[`webview/`](webview/); see [`webview/README.md`](webview/README.md) for building,
features and shortcuts.

## History

TinyMD started as four prototypes comparing rendering back ends: WebView2,
Direct2D/DirectWrite, GDI and RichEdit. The WebView2 version gave by far the best
rendering and is the only one kept; the native prototypes have been retired and
remain in the git history. [`docs/README.md`](docs/README.md) is the original
comparison, kept for reference.

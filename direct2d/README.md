# TinyMD Direct2D

The deployed Windows executable is `C:\tools\tinymd.exe`.

File loading reports unreadable paths and unsupported NUL-containing content.
The native editor receives CRLF line breaks so LF Markdown remains readable;
saving retains the document's LF or CRLF convention. Opening another file resets
the preview position. Editing or resizing clamps scrolling to the rendered
document, and device loss requests a repaint.

Run the Windows tests and build a release with the existing Go toolchain:

```
go test ./...
go build -ldflags="-s -w -H windowsgui" -trimpath -o tinymd-d2d.exe .
```

Tests exercise native EDIT controls and Direct2D; they require a Windows desktop
session. They cover scroll recovery, Markdown layout, large and empty documents,
Unicode paths, error handling, and byte-preserving LF/CRLF saves.

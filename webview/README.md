# TinyMD

A Markdown editor for Windows built on WebView2.

- **Three views**, switched from the toolbar icons or the View menu: Markdown,
  Split, Rendered.
- **Edit in the rendered view.** Click into the formatted text and type. Each edit
  is applied to the Markdown source as a splice, so everything you did not touch
  (tables, links, code fences, line endings) stays byte-for-byte the same.
- **Wide text.** The rendered text starts near the left edge and uses the whole
  pane; View > Wide Margins gives a roomier page, and zoom scales both panes.
- **Standard menus.** File (New, Open, Open Recent, Save, Save As, Open Containing
  Folder, Copy File Path, Print, Exit), Edit, View, Format, Help.
- A modified file shows `*` in the title, and closing or opening another file asks
  before discarding changes.
- Opening a folder (`tinymd.exe C:\notes`) shows a tree of its `.md` files.
- marked.js (MIT) is embedded, so rendering needs no network.

## Build

Requires Go 1.23+ and the Edge WebView2 Runtime (ships with Windows 11).

```
go build -ldflags="-s -w -H windowsgui" -trimpath -o tinymd-webview.exe .
```

## Usage

```
tinymd.exe                  # empty document
tinymd.exe path/to/file.md  # open a file
tinymd.exe path/to/folder   # browse the folder's Markdown files
```

## Keyboard shortcuts

| Key | Action |
|-----|--------|
| Ctrl+N / Ctrl+O | New / Open |
| Ctrl+S / Ctrl+Shift+S | Save / Save As |
| Ctrl+E | Open containing folder |
| Ctrl+Shift+C | Copy file path |
| Ctrl+P | Print the rendered document |
| Ctrl+Z / Ctrl+Y (or Ctrl+Shift+Z) | Undo / Redo |
| Ctrl+F, then Enter | Find |
| F3 | Find next |
| Ctrl+1 / Ctrl+2 / Ctrl+3 | Markdown / Split / Rendered view |
| Ctrl+= / Ctrl+- / Ctrl+wheel | Zoom |
| Ctrl+B / Ctrl+I | Bold / Italic |
| Ctrl+K | Link |
| Ctrl+Shift+K | Inline code (a code block when several lines are selected) |
| Ctrl+Shift+H | Heading: cycles H1, H2, H3, plain text |
| Ctrl+Shift+L | Bulleted list on or off |

## Self-test

`tinymd-webview.exe --selftest out.json` runs the rendered-editing checks in
`selftest.js` inside a hidden window and writes the results to `out.json`.

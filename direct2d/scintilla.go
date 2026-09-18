package main

// Scintilla is the editing control (the one inside Notepad++). It replaces the bare
// Windows EDIT box, which cannot show line numbers or style its text.
//
// The two DLLs are compiled into this executable and written to a per-version cache
// directory on first run, so TinyMD still ships as a single file.

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

//go:embed scintilla/Scintilla.dll
var scintillaDLL []byte

//go:embed scintilla/Lexilla.dll
var lexillaDLL []byte

//go:embed scintilla/License.txt
var scintillaLicense string

// Scintilla messages. The full set is in Scintilla.h; these are the ones TinyMD uses.
const (
	SCI_ADDTEXT                = 2001
	SCI_CLEARALL               = 2004
	SCI_GETLENGTH              = 2006
	SCI_GETTEXT                = 2182
	SCI_SETTEXT                = 2181
	SCI_SETSAVEPOINT           = 2014
	SCI_GETMODIFY              = 2159
	SCI_UNDO                   = 2176
	SCI_CUT                    = 2177
	SCI_COPY                   = 2178
	SCI_PASTE                  = 2179
	SCI_SELECTALL              = 2013
	SCI_SETEOLMODE             = 2031
	SCI_SETCODEPAGE            = 2037
	SCI_SETMARGINWIDTHN        = 2242
	SCI_SETMARGINTYPEN         = 2240
	SCI_GETMARGINWIDTHN        = 2243
	SCI_GETMARGINTYPEN         = 2241
	SCI_GETLEXERLANGUAGE       = 4012
	SCI_STYLESETFONT           = 2056
	SCI_STYLESETSIZE           = 2055
	SCI_STYLESETFORE           = 2051
	SCI_STYLESETBOLD           = 2053
	SCI_STYLESETITALIC         = 2054
	SCI_STYLECLEARALL          = 2050
	SCI_SETILEXER              = 4033
	SCI_SETWRAPMODE            = 2268
	SCI_SETSCROLLWIDTH         = 2274
	SCI_SETSCROLLWIDTHTRACKING = 2516
	SCI_GETFIRSTVISIBLELINE    = 2152
	SCI_SETFIRSTVISIBLELINE    = 2613
	SCI_GETLINECOUNT           = 2154
	SCI_SETFOCUS               = 2380
	SCI_GRABFOCUS              = 2400
	SCI_SETTABWIDTH            = 2036
	SCI_SETUSETABS             = 2124
	SCI_SETMARGINLEFT          = 2155
	SCI_SETCARETLINEVISIBLE    = 2096
	SCI_SETCARETLINEBACK       = 2098
	SCI_SETEXTRAASCENT         = 2525
	SCI_SETEXTRADESCENT        = 2527

	SC_EOL_CRLF = 0
	SC_EOL_LF   = 2
	SC_CP_UTF8  = 65001

	SC_MARGIN_NUMBER = 1
	STYLE_DEFAULT    = 32
	STYLE_LINENUMBER = 33

	// Notifications arrive by WM_NOTIFY.
	SCN_MODIFIED = 2008
	SCN_UPDATEUI = 2007

	SC_MOD_INSERTTEXT = 0x01
	SC_MOD_DELETETEXT = 0x02

	SC_UPDATE_V_SCROLL = 0x04

	// Markdown lexer styles, from LexMarkdown.cxx.
	SCE_MARKDOWN_DEFAULT    = 0
	SCE_MARKDOWN_LINE_BEGIN = 1
	SCE_MARKDOWN_STRONG1    = 2
	SCE_MARKDOWN_STRONG2    = 3
	SCE_MARKDOWN_EM1        = 4
	SCE_MARKDOWN_EM2        = 5
	SCE_MARKDOWN_HEADER1    = 6
	SCE_MARKDOWN_HEADER2    = 7
	SCE_MARKDOWN_HEADER3    = 8
	SCE_MARKDOWN_HEADER4    = 9
	SCE_MARKDOWN_HEADER5    = 10
	SCE_MARKDOWN_HEADER6    = 11
	SCE_MARKDOWN_PRECHAR    = 12
	SCE_MARKDOWN_ULIST_ITEM = 13
	SCE_MARKDOWN_OLIST_ITEM = 14
	SCE_MARKDOWN_BLOCKQUOTE = 15
	SCE_MARKDOWN_STRIKEOUT  = 16
	SCE_MARKDOWN_HRULE      = 17
	SCE_MARKDOWN_LINK       = 18
	SCE_MARKDOWN_CODE       = 19
	SCE_MARKDOWN_CODE2      = 20
	SCE_MARKDOWN_CODEBK     = 21
)

var (
	lexillaHandle syscall.Handle
	createLexer   uintptr
)

// cacheDLL writes an embedded DLL to a cache directory under a name carrying a hash
// of its contents, so a new build never loads a stale copy and a running instance
// never has its file replaced underneath it.
func cacheDLL(dir, name string, content []byte) (string, error) {
	sum := sha256.Sum256(content)
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.dll", name, hex.EncodeToString(sum[:6])))

	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(content)) {
		return path, nil
	}
	// Write under a temporary name and rename, so a half-written file is never loaded.
	temp, err := os.CreateTemp(dir, name+"-*.tmp")
	if err != nil {
		return "", err
	}
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		os.Remove(temp.Name())
		return "", err
	}
	temp.Close()
	if err := os.Rename(temp.Name(), path); err != nil {
		os.Remove(temp.Name())
		// Another instance starting at the same time may have won the race.
		if _, statErr := os.Stat(path); statErr != nil {
			return "", err
		}
	}
	return path, nil
}

// loadScintilla unpacks and loads the editing control. It returns an error rather
// than exiting so the caller can fall back to the plain EDIT control.
func loadScintilla() error {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "TinyMD")
	if dir == "TinyMD" { // LOCALAPPDATA unset
		dir = filepath.Join(os.TempDir(), "TinyMD")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	scintillaPath, err := cacheDLL(dir, "Scintilla", scintillaDLL)
	if err != nil {
		return err
	}
	lexillaPath, err := cacheDLL(dir, "Lexilla", lexillaDLL)
	if err != nil {
		return err
	}
	// The licence travels with the binaries it covers.
	os.WriteFile(filepath.Join(dir, "Scintilla-License.txt"), []byte(scintillaLicense), 0o644)

	if _, err := syscall.LoadLibrary(scintillaPath); err != nil {
		return fmt.Errorf("loading Scintilla: %w", err)
	}
	handle, err := syscall.LoadLibrary(lexillaPath)
	if err != nil {
		return fmt.Errorf("loading Lexilla: %w", err)
	}
	lexillaHandle = handle
	createLexer, _ = syscall.GetProcAddress(handle, "CreateLexer")
	return nil
}

// sci sends a message to the editing control.
func sci(msg uintptr, wParam, lParam uintptr) uintptr {
	result, _, _ := sendMessageW.Call(editorHwnd, msg, wParam, lParam)
	return result
}

// applyMarkdownStyling turns on the markdown lexer and dresses the styles to match
// the preview: the same families and weights, so the two halves look related.
func applyMarkdownStyling() {
	if createLexer != 0 {
		name := append([]byte("markdown"), 0)
		lexer, _, _ := syscall.SyscallN(createLexer, uintptr(unsafe.Pointer(&name[0])))
		if lexer != 0 {
			sci(SCI_SETILEXER, 0, lexer)
		}
	}

	consolas := append([]byte("Consolas"), 0)
	sci(SCI_STYLESETFONT, STYLE_DEFAULT, uintptr(unsafe.Pointer(&consolas[0])))
	sci(SCI_STYLESETSIZE, STYLE_DEFAULT, 11)
	sci(SCI_STYLECLEARALL, 0, 0) // propagate the default to every style first

	const (
		ink      = 0x333333
		grey     = 0x808080
		blue     = 0xD67001 // Scintilla takes colours as BGR
		green    = 0x3B8C3B
		headline = 0x8B3A2F
	)
	sci(SCI_STYLESETFORE, STYLE_DEFAULT, ink)
	sci(SCI_STYLESETFORE, STYLE_LINENUMBER, grey)

	for _, header := range []uintptr{
		SCE_MARKDOWN_HEADER1, SCE_MARKDOWN_HEADER2, SCE_MARKDOWN_HEADER3,
		SCE_MARKDOWN_HEADER4, SCE_MARKDOWN_HEADER5, SCE_MARKDOWN_HEADER6,
	} {
		sci(SCI_STYLESETFORE, header, headline)
		sci(SCI_STYLESETBOLD, header, 1)
	}
	for _, strong := range []uintptr{SCE_MARKDOWN_STRONG1, SCE_MARKDOWN_STRONG2} {
		sci(SCI_STYLESETBOLD, strong, 1)
	}
	for _, emphasis := range []uintptr{SCE_MARKDOWN_EM1, SCE_MARKDOWN_EM2} {
		sci(SCI_STYLESETITALIC, emphasis, 1)
	}
	for _, code := range []uintptr{SCE_MARKDOWN_CODE, SCE_MARKDOWN_CODE2, SCE_MARKDOWN_CODEBK} {
		sci(SCI_STYLESETFORE, code, green)
	}
	sci(SCI_STYLESETFORE, SCE_MARKDOWN_LINK, blue)
	sci(SCI_STYLESETFORE, SCE_MARKDOWN_BLOCKQUOTE, grey)
	sci(SCI_STYLESETITALIC, SCE_MARKDOWN_BLOCKQUOTE, 1)
	for _, list := range []uintptr{SCE_MARKDOWN_ULIST_ITEM, SCE_MARKDOWN_OLIST_ITEM} {
		sci(SCI_STYLESETFORE, list, blue)
	}
	sci(SCI_STYLESETFORE, SCE_MARKDOWN_HRULE, grey)
}

// configureEditor sets up the control once it exists.
func configureEditor() {
	sci(SCI_SETCODEPAGE, SC_CP_UTF8, 0)
	sci(SCI_SETEOLMODE, SC_EOL_LF, 0)

	// Line numbers in the left margin, and a little air before the text.
	sci(SCI_SETMARGINTYPEN, 0, SC_MARGIN_NUMBER)
	sci(SCI_SETMARGINWIDTHN, 0, 44)
	sci(SCI_SETMARGINWIDTHN, 1, 0) // no symbol margin
	sci(SCI_SETMARGINLEFT, 0, 6)

	// Markdown is prose: wrap it rather than scrolling sideways.
	sci(SCI_SETWRAPMODE, 1, 0)
	sci(SCI_SETTABWIDTH, 4, 0)
	sci(SCI_SETEXTRAASCENT, 2, 0)
	sci(SCI_SETEXTRADESCENT, 2, 0)

	applyMarkdownStyling()
}

// editorText reads the whole document. Scintilla stores UTF-8, which is what the
// Markdown parser wants, so no conversion is needed here.
func scintillaText() []byte {
	length := sci(SCI_GETLENGTH, 0, 0)
	if length == 0 {
		return nil
	}
	buf := make([]byte, length+1)
	sci(SCI_GETTEXT, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return buf[:length]
}

// setEditorText replaces the document.
func setScintillaText(content []byte) {
	buf := append(append([]byte{}, content...), 0)
	sci(SCI_SETTEXT, 0, uintptr(unsafe.Pointer(&buf[0])))
	sci(SCI_SETSAVEPOINT, 0, 0)
}

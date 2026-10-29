// Package term connects the escape-sequence parser to the screen.
package term

import (
	"bytes"
	"io"
	"runtime"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
)

// Applier is a vt.Handler that drives a Screen.
type Applier struct {
	s     *screen.Screen
	reply io.Writer // terminal responses to status queries

	modes     Modes
	syncSince time.Time
	title     string
	cwd       string
	cwdSeq    uint64
	promptSeq uint64
	goos      string           // replaceable so tests can check both path styles
	now       func() time.Time // replaceable so tests can drive the sync timeout
}

func NewApplier(s *screen.Screen, reply io.Writer) *Applier {
	return &Applier{s: s, reply: reply, modes: Modes{CursorVisible: true}, now: time.Now, goos: runtime.GOOS}
}

func (a *Applier) Print(r rune) { a.s.Print(r) }

// Execute handles the C0 controls it knows and ignores the rest (NUL, BEL, ...).
func (a *Applier) Execute(b byte) {
	s := a.s
	switch b {
	case 0x08:
		c, r := s.Cursor()
		s.SetCursor(c-1, r)
	case 0x09:
		s.TabForward(1)
	case 0x0A, 0x0B, 0x0C:
		s.LineFeed()
	case 0x0D:
		s.CarriageReturn()
	}
}

// param returns params[i], or def when it is missing or zero.
func param(params []int, i, def int) int {
	if i >= len(params) || params[i] <= 0 {
		return def
	}
	return params[i]
}

// selector is the erase mode, where a missing parameter means 0.
func selector(params []int) int {
	if len(params) == 0 || params[0] < 0 {
		return 0
	}
	return params[0]
}

func (a *Applier) CSI(params []int, intermediates []byte, private byte, final byte) {
	a.CSISub(params, 0, intermediates, private, final)
}

// CSISub is CSI plus the colon mask, which only SGR reads.
func (a *Applier) CSISub(params []int, colon uint32, intermediates []byte, private byte, final byte) {
	if private == '?' {
		a.privateCSI(params, intermediates, final)
		return
	}
	if private != 0 || len(intermediates) != 0 {
		return
	}
	s := a.s
	col, row := s.Cursor()
	_, rows := s.Size()
	n := param(params, 0, 1)
	switch final {
	case 'A':
		s.CursorUp(n)
	case 'B':
		s.CursorDown(n)
	case 'C':
		s.SetCursor(col+n, row)
	case 'D':
		s.SetCursor(col-n, row)
	case 'E':
		s.CursorDown(n)
		_, r := s.Cursor()
		s.SetCursor(0, r)
	case 'F':
		s.CursorUp(n)
		_, r := s.Cursor()
		s.SetCursor(0, r)
	case 'G':
		s.SetCursor(n-1, row)
	case 'd':
		s.SetCursor(col, n-1)
	case 'H', 'f':
		s.SetCursor(param(params, 1, 1)-1, param(params, 0, 1)-1)
	case 'J':
		s.EraseInDisplay(selector(params))
	case 'K':
		s.EraseInLine(selector(params))
	case 'L':
		s.InsertLines(n)
	case 'M':
		s.DeleteLines(n)
	case 'P':
		s.DeleteChars(n)
	case '@':
		s.InsertChars(n)
	case 'X':
		s.EraseChars(n)
	case 'S':
		s.ScrollUp(n)
	case 'T':
		s.ScrollDown(n)
	case 'r':
		s.SetScrollRegion(param(params, 0, 1)-1, param(params, 1, rows)-1)
	case 's':
		s.SaveCursor()
	case 'u':
		s.RestoreCursor()
	case 'm':
		a.sgr(params, colon)
	case 'n':
		a.deviceStatus(params)
	case 'c':
		if param(params, 0, 0) == 0 {
			a.replyf("\x1b[?62;22c")
		}
	}
}

func (a *Applier) ESC(intermediates []byte, final byte) {
	if len(intermediates) != 0 {
		return
	}
	s := a.s
	switch final {
	case '7':
		s.SaveCursor()
	case '8':
		s.RestoreCursor()
	case 'D':
		s.LineFeed()
	case 'E':
		s.CarriageReturn()
		s.LineFeed()
	case 'M':
		s.ReverseIndex()
	case 'c':
		s.Reset()
		a.modes = Modes{CursorVisible: true}
		a.syncSince = time.Time{}
	}
}

// OSC handles window titles (0 and 2), the folder report (7) and the
// prompt-start mark (133;A); the rest are ignored.
func (a *Applier) OSC(data []byte) {
	cmd, text, ok := bytes.Cut(data, []byte{';'})
	if ok && (string(cmd) == "0" || string(cmd) == "2") {
		a.title = string(text) // copy: parser buffers are reused
		return
	}
	if ok && string(cmd) == "7" {
		if p, good := parseOSC7(data, a.goos); good { // string conversion copies
			a.cwd = p
			a.cwdSeq++
		}
		return
	}
	if ok && string(cmd) == "133" && isPromptStart(text) {
		a.promptSeq++
	}
}

func (a *Applier) privateCSI(params []int, intermediates []byte, final byte) {
	switch {
	case len(intermediates) == 0 && final == 'h':
		a.setPrivateMode(params, true)
	case len(intermediates) == 0 && final == 'l':
		a.setPrivateMode(params, false)
	case len(intermediates) == 1 && intermediates[0] == '$' && final == 'p':
		a.queryPrivateMode(param(params, 0, 0))
	}
}

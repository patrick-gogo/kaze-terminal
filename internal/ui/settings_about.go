package ui

import (
	"image"
	"image/color"
	"io"
	"os/exec"
	"runtime"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/patrick-gogo/kaze-terminal/internal/version"
)

const aboutRepository = "https://github.com/patrick-gogo/kaze-terminal"

// The name under the logo: kaze (風) is Japanese for "wind".
const (
	aboutName    = "風・\"wind\""
	aboutTagline = "Light, fast, and out of the way."
)

func aboutSection() settingsSection {
	return settingsSection{title: "About", draw: drawAbout, act: aboutAct}
}

func aboutAct(win *window, id string, _ int) {
	switch id {
	case "about_copy":
		win.set.aboutCopy = "kaze " + version.Number + "\n" + win.set.build + "\n" + runtime.GOOS + "/" + runtime.GOARCH
	case "about_github":
		if err := openAboutRepository(); err != nil {
			win.note("Could not open GitHub: " + err.Error())
		}
	}
}

// Start the platform URL handler without blocking the UI while the browser runs.
func openAboutRepository() error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", aboutRepository)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", aboutRepository)
	default:
		cmd = exec.Command("xdg-open", aboutRepository)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func drawAbout(p *page, r image.Rectangle, win *window) int {
	gtx, ops := p.gtx, p.gtx.Ops
	if text := win.set.aboutCopy; text != "" {
		win.set.aboutCopy = ""
		if win.toClipboard != nil {
			win.toClipboard(text)
		} else {
			gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
		}
		win.note("Build details copied")
	}
	y := r.Min.Y + max(gtx.Dp(24), (r.Dy()-gtx.Dp(300))/2)
	logoPx, wordSize := gtx.Dp(94), unit.Sp(52)
	if r.Dx() < gtx.Dp(300) {
		logoPx, wordSize = gtx.Dp(52), 30
	}
	word, sz := p.text("kaze", wordSize, p.col.title)
	gap := gtx.Dp(8)
	x := r.Min.X + (r.Dx()-logoPx-gap-sz.X)/2
	if mark, img := markImage(logoPx); img != nil {
		off := op.Offset(image.Pt(x, y)).Push(ops)
		mark.Add(ops)
		paint.PaintOp{}.Add(ops)
		off.Pop()
	}
	place(ops, word, image.Pt(x+logoPx+gap, y+(logoPx-sz.Y)/2))
	y += logoPx + gtx.Dp(12)
	textIn := func(s string, size unit.Sp, col color.NRGBA) {
		for _, line := range p.wrap(s, size, max(r.Dx(), 1)) {
			call, sz := p.text(line, size, col)
			place(ops, call, image.Pt(r.Min.X+(r.Dx()-sz.X)/2, y))
			y += sz.Y
		}
	}
	text := func(s string, size unit.Sp) { textIn(s, size, p.col.fg) }
	textIn(aboutName, 14, p.col.muted)
	y += gtx.Dp(2)
	textIn(aboutTagline, 11, p.col.muted)
	y += gtx.Dp(22)
	text("Version "+version.Number, 14)
	y += gtx.Dp(6)
	text(runtime.GOOS+" / "+runtime.GOARCH+" · Go + Gio", 11)
	y += gtx.Dp(12)
	text("MIT License", 11)
	y += gtx.Dp(20)
	for _, button := range []ctrl{{kind: ctlButton, id: "about_copy", text: "Copy build info"}, {kind: ctlButton, id: "about_github", text: "View on GitHub"}} {
		call, sz, from := p.ctrl(button, 0)
		at := image.Pt(r.Min.X+(r.Dx()-sz.X)/2, y)
		p.st.shift(from, len(p.st.controls), at)
		place(ops, call, at)
		y += sz.Y + gtx.Dp(8)
	}
	return y + gtx.Dp(16)
}

package ui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

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

func aboutAct(win *window, id string, arg int) {
	switch id {
	case "about_copy":
		win.set.aboutCopy = "kaze " + version.Number + "\n" + win.set.build + "\n" + runtime.GOOS + "/" + runtime.GOARCH
	case "about_github":
		if err := openURL(aboutRepository); err != nil {
			win.note("Could not open GitHub: " + err.Error())
		}
	case "check_updates":
		win.opt.Config.CheckUpdates = arg == 1
		win.upd.next = time.Time{} // turning it back on checks soon, not a day later
		win.saveSetting("check_updates", arg == 1)
	case "upd_check":
		win.checkUpdates()
	case "upd_install":
		win.upd.confirm = true
	case "upd_cancel":
		win.upd.confirm = false
	case "upd_confirm":
		win.installUpdate()
	case "upd_page":
		if err := win.upd.open(win.upd.rel.Page); err != nil {
			win.note("Could not open the release page: " + err.Error())
		}
	}
}

// aboutUpdate is the About page's update block: a status line, its buttons
// and any reason the update can't install in place.
func (win *window) aboutUpdate() (status string, accent bool, buttons []ctrl, reason string) {
	u := &win.upd
	switch {
	case !win.opt.Config.CheckUpdates:
		return "Update checks are off", false, nil, ""
	case u.busy != "":
		return u.busy, false, nil, ""
	case u.confirm:
		sessions := fmt.Sprintf("your %d sessions", len(win.mgr.Tabs()))
		if len(win.mgr.Tabs()) == 1 {
			sessions = "your session"
		}
		return "Kaze will close " + sessions + " and restart in " + u.rel.Version + ".", false,
			[]ctrl{{kind: ctlButton, id: "upd_confirm", text: "Update now", strong: true}, {kind: ctlButton, id: "upd_cancel", text: "Cancel"}}, ""
	case u.newer:
		status = "Kaze " + u.rel.Version + " is available"
		if ok, why := win.updInstallable(); !ok {
			return status, true, []ctrl{{kind: ctlButton, id: "upd_page", text: "Download"}}, "Can't update in place: " + why
		}
		buttons = []ctrl{{kind: ctlButton, id: "upd_install", text: "Update and restart", strong: true}, {kind: ctlButton, id: "upd_page", text: "What's new"}}
		return status, true, buttons, u.err
	case u.err != "":
		status = u.err
	case u.checking:
		status = "Checking for updates…"
	case u.checked:
		status = "Up to date"
	}
	return status, false, []ctrl{{kind: ctlButton, id: "upd_check", text: "Check now"}}, ""
}

// openURL starts the platform URL handler without blocking the UI while the browser runs.
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
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
	status, accent, upButtons, reason := win.aboutUpdate()
	if status != "" {
		col := p.col.muted
		if accent {
			col = p.col.accent
		}
		textIn(status, 12, col)
		y += gtx.Dp(8)
	}
	if len(upButtons) > 0 {
		type laid struct {
			call     op.CallOp
			size     image.Point
			from, to int
		}
		var row []laid
		w, gap := 0, gtx.Dp(8)
		for _, b := range upButtons {
			call, sz, from := p.ctrl(b, 0)
			row = append(row, laid{call, sz, from, len(p.st.controls)})
			w += sz.X + gap
		}
		x := r.Min.X + (r.Dx()-(w-gap))/2
		h := 0
		for _, l := range row {
			at := image.Pt(x, y)
			p.st.shift(l.from, l.to, at)
			place(ops, l.call, at)
			x += l.size.X + gap
			h = max(h, l.size.Y)
		}
		y += h + gtx.Dp(8)
	}
	if reason != "" {
		textIn(reason, 11, p.col.muted)
		y += gtx.Dp(8)
	}
	{
		lc, ls := p.text("Check for updates", 12, p.col.fg)
		call, sz, from := p.ctrl(ctrl{kind: ctlToggle, id: "check_updates", on: win.opt.Config.CheckUpdates}, 0)
		gap := gtx.Dp(10)
		x := r.Min.X + (r.Dx()-(ls.X+gap+sz.X))/2
		h := max(ls.Y, sz.Y)
		place(ops, lc, image.Pt(x, y+(h-ls.Y)/2))
		at := image.Pt(x+ls.X+gap, y+(h-sz.Y)/2)
		p.st.shift(from, len(p.st.controls), at)
		place(ops, call, at)
		y += h + gtx.Dp(20)
	}
	for _, button := range []ctrl{{kind: ctlButton, id: "about_copy", text: "Copy build info"}, {kind: ctlButton, id: "about_github", text: "View on GitHub"}} {
		call, sz, from := p.ctrl(button, 0)
		at := image.Pt(r.Min.X+(r.Dx()-sz.X)/2, y)
		p.st.shift(from, len(p.st.controls), at)
		place(ops, call, at)
		y += sz.Y + gtx.Dp(8)
	}
	return y + gtx.Dp(16)
}

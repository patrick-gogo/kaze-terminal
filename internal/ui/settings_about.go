package ui

import (
	"image"
	"io"
	"os/exec"
	"runtime"
	"strings"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/op"
	"gioui.org/op/clip"
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
		if err := openURL(aboutRepository); err != nil {
			win.note("Could not open GitHub: " + err.Error())
		}
	case "about_issue":
		if err := openURL(aboutRepository + "/issues/new"); err != nil {
			win.note("Could not open GitHub: " + err.Error())
		}
	}
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

// platformName is the OS as people say it.
func platformName(goos string) string {
	switch goos {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	}
	return goos
}

// drawAbout is one roomy card: the logo with its byline, the name and tagline,
// a version pill, a row of facts and the page's buttons.
func drawAbout(p *page, r image.Rectangle, win *window) int {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	if text := win.set.aboutCopy; text != "" {
		win.set.aboutCopy = ""
		if win.toClipboard != nil {
			win.toClipboard(text)
		} else {
			gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
		}
		win.note("Build details copied")
	}
	rec := op.Record(ops)
	top := r.Min.Y
	y := top + gtx.Dp(40)

	// The lockup: the mark, "kaze" and "by Patrick" under its right edge.
	logoPx, wordSize := gtx.Dp(86), unit.Sp(64)
	if r.Dx() < gtx.Dp(420) {
		logoPx, wordSize = gtx.Dp(52), 38
	}
	word, ws := p.mono("kaze", wordSize, col.title)
	by, bs := p.mono("by Patrick", 13, col.muted)
	gap := gtx.Dp(18)
	w := logoPx + gap + max(ws.X, bs.X)
	x := r.Min.X + (r.Dx()-w)/2
	if mark, img := markImage(logoPx); img != nil {
		off := op.Offset(image.Pt(x, y)).Push(ops)
		mark.Add(ops)
		paint.PaintOp{}.Add(ops)
		off.Pop()
	}
	textY := y + (logoPx-ws.Y-bs.Y)/2
	place(ops, word, image.Pt(x+logoPx+gap, textY))
	place(ops, by, image.Pt(x+logoPx+gap+ws.X-bs.X, textY+ws.Y))
	y += logoPx + gtx.Dp(20)

	center := func(call op.CallOp, size image.Point) {
		place(ops, call, image.Pt(r.Min.X+(r.Dx()-size.X)/2, y))
		y += size.Y
	}
	// The name in the bundled CJK font's reach, then the tagline.
	nc, ns := p.text(aboutName, 14, col.muted)
	dc, ds := p.text("  ·  ", 14, col.dim)
	tc, ts := p.text(aboutTagline, 14, col.muted)
	lineW := ns.X + ds.X + ts.X
	lx := r.Min.X + (r.Dx()-lineW)/2
	place(ops, nc, image.Pt(lx, y))
	place(ops, dc, image.Pt(lx+ns.X, y))
	place(ops, tc, image.Pt(lx+ns.X+ds.X, y))
	y += max(ns.Y, ts.Y) + gtx.Dp(16)

	// The version pill, green when up to date and accent when a newer one is out.
	state, dot := "", col.done
	switch {
	case win.upd.newer:
		state, dot = " · "+win.upd.rel.Version+" available", col.accent
	case win.upd.checked && win.upd.err == "":
		state = " · up to date"
	}
	pc, ps := p.mono("Version "+version.Number+state, 12, col.title)
	d, padX := gtx.Dp(7), gtx.Dp(12)
	ph := ps.Y + gtx.Dp(10)
	pw := padX + d + gtx.Dp(8) + ps.X + padX
	px := r.Min.X + (r.Dx()-pw)/2
	box := image.Rect(px, y, px+pw, y+ph)
	paint.FillShape(ops, col.raised, clip.UniformRRect(box, ph/2).Op(ops))
	stroke(ops, box, ph/2, col.line2)
	paint.FillShape(ops, dot, clip.Ellipse(image.Rect(px+padX, y+(ph-d)/2, px+padX+d, y+(ph-d)/2+d)).Op(ops))
	place(ops, pc, image.Pt(px+padX+d+gtx.Dp(8), y+(ph-ps.Y)/2))
	y += ph + gtx.Dp(30)

	// The facts row: four cells split by hairlines.
	paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Min.X, y, r.Max.X, y+1)).Op())
	facts := [][2]string{
		{"VERSION", version.Number},
		{"PLATFORM", platformName(runtime.GOOS)},
		{"CHIP", runtime.GOARCH},
		{"LICENSE", "MIT License"},
	}
	cellW := r.Dx() / len(facts)
	cellH := 0
	for i, f := range facts {
		cx := r.Min.X + i*cellW
		if i > 0 {
			paint.FillShape(ops, col.line, clip.Rect(image.Rect(cx, y, cx+1, y+gtx.Dp(62))).Op())
		}
		lc, ls := p.textW(f[0], 10, col.dim, font.SemiBold)
		vc, vs := p.mono(f[1], 12.5, col.title)
		place(ops, lc, image.Pt(cx+gtx.Dp(18), y+gtx.Dp(14)))
		place(ops, vc, image.Pt(cx+gtx.Dp(18), y+gtx.Dp(14)+ls.Y+gtx.Dp(3)))
		cellH = max(cellH, gtx.Dp(14)+ls.Y+gtx.Dp(3)+vs.Y+gtx.Dp(14))
	}
	y += cellH
	paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Min.X, y, r.Max.X, y+1)).Op())
	y += gtx.Dp(18)

	// The buttons, centered.
	type laid struct {
		call     op.CallOp
		size     image.Point
		from, to int
	}
	var row []laid
	bw, bgap := 0, gtx.Dp(10)
	for _, b := range []ctrl{
		{kind: ctlButton, id: "about_copy", text: "Copy build info"},
		{kind: ctlButton, id: "about_github", text: "View on GitHub"},
		{kind: ctlButton, id: "about_issue", text: "Report an issue", ghost: true},
	} {
		call, sz, from := p.ctrl(b, 0)
		row = append(row, laid{call, sz, from, len(p.st.controls)})
		bw += sz.X + bgap
	}
	bx := r.Min.X + (r.Dx()-(bw-bgap))/2
	bh := 0
	for _, l := range row {
		at := image.Pt(bx, y)
		p.st.shift(l.from, l.to, at)
		place(ops, l.call, at)
		bx += l.size.X + bgap
		bh = max(bh, l.size.Y)
	}
	y += bh + gtx.Dp(18)
	call := rec.Stop()

	card := image.Rect(r.Min.X, top, r.Max.X, y)
	paint.FillShape(ops, col.card, clip.UniformRRect(card, gtx.Dp(12)).Op(ops))
	call.Add(ops)
	stroke(ops, card, gtx.Dp(12), col.line)
	p.st.cards = append(p.st.cards, card)
	y += gtx.Dp(18)
	cc, cs := p.text("Made with Go and Gio, for the long session.", 12, col.dim)
	center(cc, cs)
	return y + gtx.Dp(16)
}

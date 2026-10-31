package ui

import (
	"fmt"
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/patrick-gogo/kaze-terminal/internal/version"
)

const releasesPage = aboutRepository + "/releases"

func updatesSection() settingsSection {
	return settingsSection{
		title: "Updates", hint: "New versions from GitHub, installed in place.",
		icon: "updates", group: "Kaze",
		draw: drawUpdates, act: updatesAct,
		badge: func(win *window) string {
			if win.opt.Config.CheckUpdates && win.upd.newer && win.upd.busy == "" {
				return "1"
			}
			return ""
		},
	}
}

func updatesAct(win *window, id string, arg int) {
	switch id {
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
	case "upd_releases":
		if err := win.upd.open(releasesPage); err != nil {
			win.note("Could not open the releases: " + err.Error())
		}
	}
}

// updHero is the status card's content for the updater's state.
type updHero struct {
	icon, title, sub string
	buttons          []ctrl
	reason           string // why the update can't install in place, under the card's text
	progress         int    // 0..100 while downloading, -1 otherwise
	confirm          string // the restart question, with its buttons in a strip under the card
}

func (win *window) updHero(now time.Time) updHero {
	u := &win.upd
	h := updHero{icon: "check", progress: -1}
	switch {
	case !win.opt.Config.CheckUpdates:
		h.icon, h.title, h.sub = "updates", "Update checks are off", "Turn them on below to hear about new versions."
	case u.busy != "":
		h.icon, h.title, h.sub, h.progress = "download", u.busy, "Kaze restarts by itself when it's done.", u.pct
	case u.newer:
		h.icon, h.title = "download", "Kaze "+u.rel.Version+" is available"
		h.sub = "You have " + version.Number
		if ok, why := win.updInstallable(); !ok {
			h.buttons, h.reason = []ctrl{{kind: ctlButton, id: "upd_page", text: "Download", strong: true}}, "Can't update in place: "+why
			break
		}
		if u.confirm {
			sessions := fmt.Sprintf("your %d sessions", len(win.mgr.Tabs()))
			if len(win.mgr.Tabs()) == 1 {
				sessions = "your session"
			}
			h.confirm = "Kaze will close " + sessions + " and restart in " + u.rel.Version + "."
			break
		}
		h.buttons = []ctrl{
			{kind: ctlButton, id: "upd_page", text: "What's new", ghost: true},
			{kind: ctlButton, id: "upd_install", text: "Update and restart", strong: true},
		}
		h.reason = u.err
	case u.err != "":
		h.icon, h.title, h.sub = "alert", "Couldn't check for updates", u.err
		h.buttons = []ctrl{{kind: ctlButton, id: "upd_check", text: "Try again"}}
	case u.checking:
		h.icon, h.title, h.sub = "updates", "Checking for updates…", "Asking GitHub for the latest release."
	case u.checked:
		h.title, h.sub = "Kaze is up to date", "Version "+version.Number+" · checked "+ago(now.Sub(u.checkedAt))
		h.buttons = []ctrl{{kind: ctlButton, id: "upd_check", text: "Check now"}}
	default:
		h.icon, h.title, h.sub = "updates", "Kaze "+version.Number, "Not checked yet."
		h.buttons = []ctrl{{kind: ctlButton, id: "upd_check", text: "Check now"}}
	}
	return h
}

// ago is a short "how long ago" for the last check.
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < 2*time.Minute:
		return "a minute ago"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 2*time.Hour:
		return "an hour ago"
	}
	return fmt.Sprintf("%d hours ago", int(d.Hours()))
}

// drawUpdates is the status card, then the setting and the install facts.
func drawUpdates(p *page, r image.Rectangle, win *window) int {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	sec := updatesSection()
	y := p.heading(r, sec.title, sec.hint)
	h := win.updHero(time.Now())

	rec := op.Record(ops)
	top := y
	pad := gtx.Dp(24)
	y += pad
	// The icon tile.
	tile := gtx.Dp(64)
	tb := image.Rect(r.Min.X+pad, y, r.Min.X+pad+tile, y+tile)
	paint.FillShape(ops, tint(col.accent, 0x24), clip.UniformRRect(tb, gtx.Dp(18)).Op(ops))
	stroke(ops, tb, gtx.Dp(18), tint(col.accent, 0x60))
	is := gtx.Dp(26)
	p.c.icon(gtx, h.icon, image.Rect(tb.Min.X+(tile-is)/2, tb.Min.Y+(tile-is)/2, tb.Min.X+(tile-is)/2+is, tb.Min.Y+(tile-is)/2+is), col.title)

	// The buttons, right-aligned and centered on the tile.
	bx := r.Max.X - pad
	for i := len(h.buttons) - 1; i >= 0; i-- {
		call, sz, from := p.ctrl(h.buttons[i], 0)
		bx -= sz.X
		at := image.Pt(bx, y+(tile-sz.Y)/2)
		p.st.shift(from, len(p.st.controls), at)
		place(ops, call, at)
		bx -= gtx.Dp(8)
	}
	tx := tb.Max.X + gtx.Dp(20)
	tc, ts := p.textW(h.title, 18, col.title, font.SemiBold)
	sc, ss := p.text(h.sub, 13, col.muted)
	ty := y + (tile-ts.Y-ss.Y)/2
	if h.progress >= 0 {
		ty = y + (tile-ts.Y-ss.Y-gtx.Dp(18))/2
	}
	place(ops, tc, image.Pt(tx, ty))
	place(ops, sc, image.Pt(tx, ty+ts.Y+gtx.Dp(2)))
	if h.progress >= 0 {
		bar := image.Rect(tx, ty+ts.Y+ss.Y+gtx.Dp(12), bx, ty+ts.Y+ss.Y+gtx.Dp(18))
		paint.FillShape(ops, col.line, clip.UniformRRect(bar, bar.Dy()/2).Op(ops))
		done := bar
		done.Max.X = bar.Min.X + bar.Dx()*min(h.progress, 100)/100
		if done.Dx() > 0 {
			paint.FillShape(ops, col.accent, clip.UniformRRect(done, bar.Dy()/2).Op(ops))
		}
	}
	y += tile + pad
	if h.reason != "" {
		for _, line := range p.wrap(h.reason, 12, r.Dx()-2*pad) {
			call, size := p.text(line, 12, col.muted)
			place(ops, call, image.Pt(r.Min.X+pad, y-gtx.Dp(8)))
			y += size.Y
		}
		y += gtx.Dp(8)
	}
	if h.confirm != "" {
		strip := image.Rect(r.Min.X+pad, y-gtx.Dp(4), r.Max.X-pad, y-gtx.Dp(4)+gtx.Dp(56))
		paint.FillShape(ops, tint(col.needs, 0x14), clip.UniformRRect(strip, gtx.Dp(10)).Op(ops))
		stroke(ops, strip, gtx.Dp(10), tint(col.needs, 0x50))
		qc, qs := p.text(h.confirm, 13, col.title)
		place(ops, qc, image.Pt(strip.Min.X+gtx.Dp(16), strip.Min.Y+(strip.Dy()-qs.Y)/2))
		x := strip.Max.X - gtx.Dp(12)
		for _, b := range []ctrl{
			{kind: ctlButton, id: "upd_confirm", text: "Update now", strong: true},
			{kind: ctlButton, id: "upd_cancel", text: "Cancel", ghost: true},
		} {
			call, sz, from := p.ctrl(b, 0)
			x -= sz.X
			at := image.Pt(x, strip.Min.Y+(strip.Dy()-sz.Y)/2)
			p.st.shift(from, len(p.st.controls), at)
			place(ops, call, at)
			x -= gtx.Dp(8)
		}
		y = strip.Max.Y + gtx.Dp(20)
	}
	call := rec.Stop()
	card := image.Rect(r.Min.X, top, r.Max.X, y)
	paint.FillShape(ops, col.card, clip.UniformRRect(card, gtx.Dp(12)).Op(ops))
	call.Add(ops)
	stroke(ops, card, gtx.Dp(12), col.line)
	p.st.cards = append(p.st.cards, card)
	y += gtx.Dp(26)

	where := win.upd.exe
	can, tone := "Can update in place", "ok"
	if win.upd.target.Path != "" {
		where = win.upd.target.Path
	}
	if ok, _ := win.updInstallable(); !ok && win.upd.targetErr != nil {
		can, tone = "Download only", "off"
	}
	rows := []settingRow{
		{heading: "Settings"},
		{label: "Check for updates automatically", hint: "Once a day. Off means kaze never contacts GitHub.",
			ctrls: []ctrl{{kind: ctlToggle, id: "check_updates", on: win.opt.Config.CheckUpdates}}},
		{heading: "Release"},
		{label: "Installed from", hint: truncateMiddle(where, 64), monoHint: true, pill: can, tone: tone},
		{label: "Release notes and downloads", hint: "Every version on GitHub.",
			ctrls: []ctrl{{kind: ctlButton, id: "upd_releases", text: "Open releases", ghost: true}}},
	}
	return p.rows(r, y, rows)
}

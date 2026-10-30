package ui

import (
	"image"
	"io"
	"runtime"
	"strings"

	"gioui.org/io/clipboard"
)

// cmdLine is one line of Settings > Commands' command-line list; Copy "" has no button.
type cmdLine struct{ Example, Copy, Desc string }

var commandLines = []cmdLine{
	{"kaze --workspace <name>", `kaze --workspace "<name>"`, "Start kaze with a workspace"},
	{"kaze --version", "kaze --version", "Print the version and build"},
	{"kaze notify done / needs / prompt", "", "Used by Claude Code hooks; you don't run these yourself"},
	{"kaze statusline", "", "Used by Claude Code's status line; you don't run this yourself"},
}

// mouseLines are selection gestures listed with the shortcuts; they have no key to change.
var mouseLines = [][2]string{
	{"Select a word / a line", "double / triple click"},
	{"Select in mouse apps", "Shift + drag"},
}

func commandsSection() settingsSection {
	return settingsSection{
		title: "Commands",
		hint:  "Read-only for now. Change a shortcut under [keys] in config.toml.",
		draw:  drawCommands,
		act:   commandsAct,
	}
}

func commandsAct(win *window, id string, arg int) {
	if id == "cmd_copy" && arg >= 0 && arg < len(commandLines) {
		win.set.cmdCopy = commandLines[arg].Copy
	}
}

// drawCommands lists kaze's shortcuts as this machine has them, overrides
// included, and kaze's command line.
func drawCommands(p *page, r image.Rectangle, win *window) int {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	if text := win.set.cmdCopy; text != "" {
		win.set.cmdCopy = ""
		if win.toClipboard != nil {
			win.toClipboard(text)
		} else {
			gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
		}
		win.note("Copied: " + text)
	}
	sec := commandsSection()
	y := p.heading(r, sec.title, sec.hint)
	keyX := r.Min.X + min(r.Dx()*45/100, gtx.Dp(300))
	cfgX := keyX + min(r.Dx()*25/100, gtx.Dp(160))
	row := func(label, keyText, cfg string) {
		lc, ls := p.text(p.fit(label, 12, keyX-r.Min.X-gtx.Dp(12)), 12, col.fg)
		place(ops, lc, image.Pt(r.Min.X, y))
		kc, _ := p.text(p.fit(keyText, 12, cfgX-keyX-gtx.Dp(12)), 12, col.title)
		place(ops, kc, image.Pt(keyX, y))
		if cfg != "" {
			cc, _ := p.text(p.fit(cfg, 11, max(r.Max.X-cfgX, 1)), 11, col.dim)
			place(ops, cc, image.Pt(cfgX, y+gtx.Dp(1)))
		}
		y += ls.Y + gtx.Dp(8)
	}
	header := func(s string) {
		call, sz := p.text(s, 11, col.dim)
		place(ops, call, image.Pt(r.Min.X, y))
		y += sz.Y + gtx.Dp(10)
	}

	header("SHORTCUTS")
	for _, c := range kazeCommands {
		keyText := "none"
		if sc, ok := win.binds[c.Config]; ok {
			keyText = formatShortcut(runtime.GOOS, sc)
		}
		row(c.Label, keyText, c.Config)
	}
	for _, m := range mouseLines {
		row(m[0], m[1], "")
	}

	y += gtx.Dp(14)
	header("COMMAND LINE")
	for i, l := range commandLines {
		ec, es := p.text(l.Example, 12, col.title)
		place(ops, ec, image.Pt(r.Min.X, y))
		bh := 0
		if l.Copy != "" {
			call, sz, from := p.ctrl(ctrl{kind: ctlButton, id: "cmd_copy", arg: i, text: "Copy"}, 0)
			at := image.Pt(max(r.Max.X-sz.X, r.Min.X+es.X+gtx.Dp(12)), y-gtx.Dp(4))
			p.st.shift(from, len(p.st.controls), at)
			place(ops, call, at)
			bh = sz.Y - gtx.Dp(4)
		}
		y += es.Y + gtx.Dp(2)
		for _, line := range p.wrap(l.Desc, 11, max(r.Dx(), 1)) {
			dc, ds := p.text(line, 11, col.dim)
			place(ops, dc, image.Pt(r.Min.X, y))
			y += ds.Y
		}
		y = max(y, y-es.Y+bh) + gtx.Dp(12)
	}
	return y + gtx.Dp(16)
}

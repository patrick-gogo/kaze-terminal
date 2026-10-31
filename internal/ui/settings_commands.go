package ui

import (
	"image"
	"io"
	"runtime"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
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

// shortcutGroups sorts kaze's actions into the page's cards; an action not
// listed lands in Other.
var shortcutGroups = []struct {
	title   string
	configs []string
}{
	{"Sessions", []string{"new_tab", "close_tab", "next_tab", "prev_tab", "next_waiting"}},
	{"Kaze", []string{"palette", "scratch_shell", "toggle_sidebar", "open_settings"}},
	{"Editing and mouse", []string{"copy", "paste"}},
}

func commandsSection() settingsSection {
	return settingsSection{
		title: "Shortcuts",
		hint:  "Change any of them under [keys] in config.toml.",
		draw:  drawCommands,
		act:   commandsAct,
	}
}

func commandsAct(win *window, id string, arg int) {
	if id == "cmd_copy" && arg >= 0 && arg < len(commandLines) {
		win.set.cmdCopy = commandLines[arg].Copy
	}
}

// shortcutRow is one line of a shortcuts card.
type shortcutRow struct{ label, keys, config string }

// drawCommands lists kaze's shortcuts as this machine has them, overrides
// included, in grouped cards with key caps, then kaze's command line.
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
	p.headerActions(r, []ctrl{{kind: ctlText, id: "cmd_filter", val: win.set.cmdFilter, placeholder: "Search shortcuts", width: 220,
		set: func(s string) { win.set.cmdFilter = s }}})

	q := strings.ToLower(strings.TrimSpace(win.set.cmdFilter))
	match := func(r shortcutRow) bool {
		return q == "" || strings.Contains(strings.ToLower(r.label+" "+r.keys+" "+r.config), q)
	}
	keysOf := func(config string) string {
		if sc, ok := win.binds[config]; ok {
			return formatShortcut(runtime.GOOS, sc)
		}
		return "none"
	}
	grouped := map[string]bool{}
	var groups [][]shortcutRow
	var titles []string
	for _, g := range shortcutGroups {
		var rows []shortcutRow
		for _, cfg := range g.configs {
			grouped[cfg] = true
			for _, c := range kazeCommands {
				if c.Config == cfg {
					rows = append(rows, shortcutRow{c.Label, keysOf(cfg), cfg})
				}
			}
		}
		if g.title == "Editing and mouse" {
			for _, m := range mouseLines {
				rows = append(rows, shortcutRow{label: m[0], keys: m[1]})
			}
		}
		groups, titles = append(groups, rows), append(titles, g.title)
	}
	var other []shortcutRow
	for _, c := range kazeCommands {
		if !grouped[c.Config] {
			other = append(other, shortcutRow{c.Label, keysOf(c.Config), c.Config})
		}
	}
	if len(other) > 0 {
		groups, titles = append(groups, other), append(titles, "Other")
	}

	shown := false
	for gi, rows := range groups {
		var keep []shortcutRow
		for _, row := range rows {
			if match(row) {
				keep = append(keep, row)
			}
		}
		if len(keep) == 0 {
			continue
		}
		if shown {
			y += gtx.Dp(26)
		}
		y = p.sectionLabel(r, y, titles[gi])
		y = p.shortcutCard(r, y, keep)
		shown = true
	}
	if !shown {
		call, size := p.text("No shortcut matches \""+win.set.cmdFilter+"\".", 13, col.muted)
		place(ops, call, image.Pt(r.Min.X+gtx.Dp(2), y))
		y += size.Y
	}

	y += gtx.Dp(26)
	y = p.sectionLabel(r, y, "Command line")
	rec := op.Record(ops)
	top := y
	pad := gtx.Dp(18)
	for i, l := range commandLines {
		if i > 0 {
			paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Min.X, y, r.Max.X, y+1)).Op())
		}
		ec, es := p.mono(l.Example, 12.5, col.title)
		dc, ds := p.text(l.Desc, 12, col.muted)
		h := max(es.Y+ds.Y+gtx.Dp(26), gtx.Dp(58))
		ty := y + (h-es.Y-ds.Y)/2
		place(ops, ec, image.Pt(r.Min.X+pad, ty))
		place(ops, dc, image.Pt(r.Min.X+pad, ty+es.Y+gtx.Dp(2)))
		if l.Copy != "" {
			call, sz, from := p.ctrl(ctrl{kind: ctlButton, id: "cmd_copy", arg: i, text: "Copy", ghost: true}, 0)
			at := image.Pt(r.Max.X-pad-sz.X, y+(h-sz.Y)/2)
			p.st.shift(from, len(p.st.controls), at)
			place(ops, call, at)
		}
		y += h
	}
	call := rec.Stop()
	card := image.Rect(r.Min.X, top, r.Max.X, y)
	paint.FillShape(ops, col.card, clip.UniformRRect(card, gtx.Dp(12)).Op(ops))
	call.Add(ops)
	stroke(ops, card, gtx.Dp(12), col.line)
	p.st.cards = append(p.st.cards, card)
	return y + gtx.Dp(16)
}

// shortcutCard draws rows of label, key caps and config name in one card.
func (p *page) shortcutCard(r image.Rectangle, y int, rows []shortcutRow) int {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	rec := op.Record(ops)
	top := y
	pad := gtx.Dp(18)
	idW := gtx.Dp(130)
	for i, row := range rows {
		if i > 0 {
			paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Min.X, y, r.Max.X, y+1)).Op())
		}
		h := gtx.Dp(46)
		lc, ls := p.text(row.label, 13, col.title)
		place(ops, lc, image.Pt(r.Min.X+pad, y+(h-ls.Y)/2))
		right := r.Max.X - pad
		if row.config != "" {
			cc, cs := p.mono(row.config, 11, col.dim)
			place(ops, cc, image.Pt(right-cs.X, y+(h-cs.Y)/2))
		}
		right -= idW
		if row.config == "" {
			// A mouse gesture: plain words, no caps.
			kc, ks := p.mono(row.keys, 11.5, col.fg)
			place(ops, kc, image.Pt(right-ks.X, y+(h-ks.Y)/2))
		} else {
			caps := strings.Split(row.keys, "+")
			if strings.HasSuffix(row.keys, "++") {
				caps = append(strings.Split(strings.TrimSuffix(row.keys, "++"), "+"), "+")
			}
			type cap struct {
				call op.CallOp
				size image.Point
			}
			var cs []cap
			w := 0
			for _, k := range caps {
				call, size := p.keyCap(k)
				cs = append(cs, cap{call, size})
				w += size.X + gtx.Dp(4)
			}
			x := right - w + gtx.Dp(4)
			for _, c := range cs {
				place(ops, c.call, image.Pt(x, y+(h-c.size.Y)/2))
				x += c.size.X + gtx.Dp(4)
			}
		}
		y += h
	}
	call := rec.Stop()
	card := image.Rect(r.Min.X, top, r.Max.X, y)
	paint.FillShape(ops, col.card, clip.UniformRRect(card, gtx.Dp(12)).Op(ops))
	call.Add(ops)
	stroke(ops, card, gtx.Dp(12), col.line)
	p.st.cards = append(p.st.cards, card)
	return y
}

// keyCap draws one key as a small cap at the origin, a thicker edge at its bottom.
func (p *page) keyCap(k string) (op.CallOp, image.Point) {
	gtx, col := p.gtx, p.col
	rec := op.Record(gtx.Ops)
	tc, ts := p.mono(k, 11.5, col.title)
	h := ts.Y + gtx.Dp(8)
	w := max(ts.X+gtx.Dp(14), gtx.Dp(26))
	box := image.Rect(0, 0, w, h)
	radius := gtx.Dp(6)
	paint.FillShape(gtx.Ops, col.line2, clip.UniformRRect(box, radius).Op(gtx.Ops))
	face := image.Rect(0, 0, w, h-gtx.Dp(2))
	paint.FillShape(gtx.Ops, col.raised, clip.UniformRRect(face, radius).Op(gtx.Ops))
	stroke(gtx.Ops, box, radius, col.line2)
	place(gtx.Ops, tc, image.Pt((w-ts.X)/2, (face.Dy()-ts.Y)/2))
	return rec.Stop(), box.Size()
}

package ui

import (
	"image"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gioui.org/font"
	"github.com/patrick-gogo/kaze-terminal/internal/update"
)

func TestUpdateConfirmationWrapsInsideCard(t *testing.T) {
	for _, width := range []int{320, 360, 560, 1000} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			win, _ := alertWindow(t, 3)
			win.opt.Config.CheckUpdates = true
			win.upd.newer, win.upd.confirm = true, true
			win.upd.rel.Version = "0.19.1"
			win.upd.goos, win.upd.goarch = "darwin", "arm64"
			win.upd.rel.Assets = map[string]string{update.AssetName("darwin", "arm64"): "unused", update.SumsName: "unused"}
			// An owned restore store adds the longer restart explanation.
			win.restorePath = filepath.Join(t.TempDir(), "sessions.json")
			win.opt.Config.RestoreSessions = true
			win.initRestore()
			t.Cleanup(win.closeRestore)
			gtx := pageContext(width, 2000)
			p := &page{gtx: gtx, c: win.ui, st: &win.set, col: win.ui.pageColors()}
			drawUpdates(p, image.Rect(0, 0, width, 2000), win)
			cancel, confirm := controlsOf(win, "upd_cancel"), controlsOf(win, "upd_confirm")
			if len(cancel) != 1 || len(confirm) != 1 {
				t.Fatal("confirmation buttons missing")
			}
			card := win.set.cards[0]
			for _, c := range []control{cancel[0], confirm[0]} {
				if !c.rect.In(card) || c.rect.Empty() {
					t.Fatalf("button %v outside card %v", c.rect, card)
				}
			}
			if cancel[0].rect.Overlaps(confirm[0].rect) {
				t.Fatal("buttons overlap")
			}
			full := win.updHero(gtx.Now).confirm
			stripBox := image.Rect(24, 100, width-24, 100)
			buttons := image.Pt(cancel[0].rect.Dx()+8+confirm[0].rect.Dx(), confirm[0].rect.Dy())
			l := p.layoutUpdateConfirm(stripBox, full, buttons)
			if !l.text.In(l.strip) || !l.buttons.In(l.strip) || l.text.Overlaps(l.buttons) {
				t.Fatalf("text %v and buttons %v must fit separately inside %v", l.text, l.buttons, l.strip)
			}
			if width == 360 && l.buttons.Min.Y <= l.text.Max.Y {
				t.Fatal("narrow strip must put buttons below text")
			}
			if width == 1000 && l.text.Max.X >= l.buttons.Min.X {
				t.Fatal("wide strip must reserve space to the left of buttons")
			}
			var lines []string
			for _, s := range win.set.texts {
				if s != "" && strings.Contains(full, s) && s != "Cancel" && s != "Update now" {
					lines = append(lines, s)
				}
			}
			if strings.Join(lines, " ") != full || len(lines) < 2 {
				t.Fatalf("confirmation did not wrap completely: %q", lines)
			}
			for _, line := range lines {
				_, size := p.c.sans(gtx, line, 13, p.col.title, font.Normal)
				if size.X > width-80 {
					t.Fatalf("line %q is %dpx wide in a %dpx card", line, size.X, width)
				}
			}
		})
	}
}

package ui

import (
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
)

// sizedFrame runs one headless frame of a w x h window fed by r.
func sizedFrame(t *testing.T, win *window, r *input.Router, w, h int) {
	t.Helper()
	gtx := pageContext(w, h)
	gtx.Source = r.Source()
	gtx.Now = time.Unix(1000, 0)
	if err := win.frame(gtx); err != nil {
		t.Fatal(err)
	}
	r.Frame(gtx.Ops)
}

// A theme change and a tab switch in one batch: the terminal is drawn the same
// frame the page set the renderer up for a rebuild.
func TestSettingChangeThenTabSwitchInOneBatch(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.opt.Config = config.Default()
	win.cfgPath = filepath.Join(t.TempDir(), "config.toml")
	var r input.Router
	sizedFrame(t, win, &r, 800, 600)
	win.openSettings()
	sizedFrame(t, win, &r, 800, 600) // lays the page out and focuses it
	tideAt := themeIndex(t, "tide")
	win.set.focus = -1
	for i, c := range win.set.controls {
		if c.id == "theme" && c.arg == tideAt {
			win.set.focus = i
		}
	}
	if win.set.focus < 0 {
		t.Fatalf("no Tide swatch in %+v", win.set.controls)
	}
	r.Queue(
		key.Event{Name: key.NameReturn, State: key.Press},
		key.Event{Name: key.NameTab, Modifiers: key.ModCtrl, State: key.Press},
	)
	sizedFrame(t, win, &r, 800, 600)
	if win.set.shown || win.opt.Config.Theme != "tide" {
		t.Fatalf("after Enter, Ctrl+Tab: shown %v theme %q", win.set.shown, win.opt.Config.Theme)
	}
	sizedFrame(t, win, &r, 800, 600)
	tide, _ := theme.ByKey("tide")
	if got := win.rend.Palette().BG; got != tide.Term.BG {
		t.Fatalf("renderer background %v, want Tide's %v", got, tide.Term.BG)
	}
}

func TestSettingsPageSkipsTheRendererButResizesTabs(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	win.ui = newChrome()
	win.focused = true
	win.opt.Config = config.Default()
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	var r input.Router
	sizedFrame(t, win, &r, 800, 600)
	if win.rend.Stats().FramesDrawn == 0 {
		t.Fatal("the terminal frame did not lay the renderer out")
	}
	win.openSettings()
	sizedFrame(t, win, &r, 800, 600)
	drawn := win.rend.Stats().FramesDrawn
	before := st.ptys[0].lastSize()
	sizedFrame(t, win, &r, 1000, 700)
	if got := win.rend.Stats().FramesDrawn; got != drawn {
		t.Fatalf("the renderer drew %d frames under settings", got-drawn)
	}
	after := st.ptys[0].lastSize()
	cw, ch := win.rend.CellSize()
	want := [2]int{}
	want[0], want[1] = gridSize(984, 630, cw, ch) // the grid inside a 1000x700 window
	if after == before || after != want {
		t.Fatalf("tab sized %v under settings after a resize from %v, want %v", after, before, want)
	}
}

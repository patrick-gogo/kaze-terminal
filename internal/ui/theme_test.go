package ui

import (
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/render"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
	"github.com/patrick-gogo/kaze-terminal/internal/usage"
)

func mustTheme(t *testing.T, k string) theme.Theme {
	t.Helper()
	th, ok := theme.ByKey(k)
	if !ok {
		t.Fatalf("no theme %q", k)
	}
	return th
}

func TestNewChromeIsMidnight(t *testing.T) {
	c := newChrome()
	if c.th != theme.Default().UI || c.shape != theme.Rounded {
		t.Fatal("a new chrome is not Midnight with rounded tabs")
	}
}

func TestApplyThemeRepaintsChrome(t *testing.T) {
	win := testWindow(t, &recStarter{})
	win.ui = newChrome()
	win.rend = render.NewRenderer(14, win.pal)
	win.applyTheme(mustTheme(t, "sumi"))
	if win.ui.th.Accent != rgb(0xe2553d) || win.ui.shape != theme.Underline {
		t.Fatalf("chrome accent %v shape %v, want Sumi", win.ui.th.Accent, win.ui.shape)
	}
	if win.pal.BG != rgb(0x111111) {
		t.Fatalf("terminal bg %v, want #111111", win.pal.BG)
	}
	if win.rend != nil {
		t.Fatal("the renderer kept the old palette")
	}
	win.opt.Config.Colors = map[string]string{"bg": "#203040"}
	win.applyTheme(mustTheme(t, "sumi"))
	if win.pal.BG != rgb(0x203040) || win.pal.FG != rgb(0xe4e1da) {
		t.Fatalf("[colors] bg did not win over the theme: %v on %v", win.pal.FG, win.pal.BG)
	}
}

func TestToastAndUsageUseTheme(t *testing.T) {
	c := newChrome()
	c.setTheme(mustTheme(t, "washi"))
	if got := c.usageColor(usage.Warn); got != rgb(0x9a6a00) {
		t.Fatalf("warn usage %v, want #9a6a00", got)
	}
	if got := c.usageColor(usage.High); got != rgb(0xc0392b) {
		t.Fatalf("high usage %v, want #c0392b", got)
	}
	if got, ok := c.dotColor(false, markDone); !ok || got != rgb(0x2e7d4f) {
		t.Fatalf("done dot %v, want #2e7d4f", got)
	}
	if got, _ := c.captionBG(capMinimize, captionState{hover: capMinimize}); got != rgb(0xcfc7b7) {
		t.Fatalf("caption hover %v, want #cfc7b7", got)
	}
}

func TestPillTintIsFaintAccent(t *testing.T) {
	a := rgb(0x40e0d0)
	got := pillTint(a)
	if got.R != a.R || got.G != a.G || got.B != a.B || got.A != 0x21 {
		t.Fatalf("pill tint %v, want the accent at 13%%", got)
	}
}

// Every tab shape draws over the same layout, so clicks land the same way.
func TestTabShapesKeepTheLayout(t *testing.T) {
	var want []tabHit
	for _, k := range []string{"midnight", "sumi", "tide"} {
		win, _ := alertWindow(t, 3)
		win.ui.setTheme(mustTheme(t, k))
		win.ui.layoutTabBar(barContext(1000), win.mgr.Tabs(), win.mgr.Active().ID, nil, "pwsh", 38, 900)
		got := append([]tabHit(nil), win.ui.hits.tabs...)
		if len(got) != 3 {
			t.Fatalf("%s: %d tabs drawn, want 3", k, len(got))
		}
		if want == nil {
			want = got
			continue
		}
		for i := range got {
			if got[i].rect != want[i].rect || got[i].close != want[i].close {
				t.Fatalf("%s: tab %d at %v, want %v", k, i, got[i], want[i])
			}
		}
	}
}

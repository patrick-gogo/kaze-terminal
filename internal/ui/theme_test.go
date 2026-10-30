package ui

import (
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
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
	if c.th != theme.Default().UI {
		t.Fatal("a new chrome is not Midnight")
	}
}

func TestApplyThemeRepaintsChrome(t *testing.T) {
	win := testWindow(t, &recStarter{})
	win.ui = newChrome()
	win.rend = render.NewRenderer(14, win.pal, render.Options{})
	win.applyTheme(mustTheme(t, "sumi"))
	if win.ui.th.Accent != rgb(0xe2553d) {
		t.Fatalf("chrome accent %v, want Sumi", win.ui.th.Accent)
	}
	if win.pal.BG != rgb(0x111111) {
		t.Fatalf("terminal bg %v, want #111111", win.pal.BG)
	}
	if win.rend == nil || !win.rendStale {
		t.Fatal("the renderer kept the old palette, or was dropped mid-frame")
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

func TestStartThemeFollowsConfig(t *testing.T) {
	if got := startTheme(config.Config{Theme: "sumi"}); got.UI.Accent != rgb(0xe2553d) {
		t.Fatalf("accent %v, want Sumi's #e2553d", got.UI.Accent)
	}
	if got := startTheme(config.Config{Theme: "nope"}); got.UI != theme.Default().UI {
		t.Fatal("an unknown theme did not fall back to Midnight")
	}
}

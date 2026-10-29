package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
	"github.com/patrick-gogo/kaze-terminal/internal/version"
	"image"
	"slices"
	"strings"
	"testing"
)

func TestAboutSectionShowsBuildAcrossThemes(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.openSettings()
	i := slices.IndexFunc(win.sections(), func(s settingsSection) bool { return s.title == "About" })
	if i < 0 {
		t.Fatal("missing About section")
	}
	win.selectSection(i)
	win.set.build = "build abc1234 · Oct 29, 4:30 PM + local changes"
	for _, th := range theme.All() {
		win.applyTheme(th)
		win.drawSettings(pageContext(800, 600), image.Rect(0, 38, 800, 576))
		text := strings.Join(win.set.texts, "\n")
		for _, want := range []string{"kaze", "Version " + version.Number, win.set.build, "MIT License"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", th.Key, want)
			}
		}
	}
}

func TestAboutCopyUsesDisplayedVersionAndBuild(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.openSettings()
	win.set.build = "build test123"
	var copied string
	win.toClipboard = func(s string) { copied = s }
	aboutAct(win, "about_copy", 0)
	drawAbout(&page{gtx: pageContext(800, 600), c: win.ui, st: &win.set, col: win.ui.pageColors()}, image.Rect(218, 60, 772, 576), win)
	if !strings.Contains(copied, "kaze "+version.Number) || !strings.Contains(copied, "build test123") {
		t.Fatalf("copy %q", copied)
	}
}

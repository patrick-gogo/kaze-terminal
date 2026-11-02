package ui

import (
	"strings"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/resources"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// Claude Code starts its title with a status mark (✳ idle, a braille spinner
// while working). The card's font has neither, so it drew a box (#95).
func TestResourceNameDropsAgentStatusMark(t *testing.T) {
	win := &window{defTitle: "zsh"}
	for title, want := range map[string]string{
		"✳ MARK DOWN READER":    "MARK DOWN READER",
		"⠹ Read plan":           "Read plan",
		"⠂⠐ two marks":          "two marks",
		"plain title":           "plain title",
		"keeps ✳ in the middle": "keeps ✳ in the middle",
	} {
		if got := win.tabResourceName(&tabs.Tab{Title: title}); got != want {
			t.Errorf("%q: got %q, want %q", title, got, want)
		}
	}
}

func TestResourceFooterIsShort(t *testing.T) {
	got := resourceFooter(resources.Snapshot{})
	if got != "Kaze, its shells, agents and local servers. Updates every 2 s." {
		t.Fatalf("footer %q", got)
	}
	for _, s := range []string{"bytes", "MB =", "GB =", "cores"} {
		if strings.Contains(got, s) {
			t.Fatalf("footer explains units: %q", got)
		}
	}
	if p := resourceFooter(resources.Snapshot{Partial: true}); !strings.HasPrefix(p, "* Partial totals. ") {
		t.Fatalf("partial footer %q", p)
	}
}

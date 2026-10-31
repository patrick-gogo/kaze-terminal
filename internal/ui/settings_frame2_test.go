package ui

import (
	"slices"
	"strings"
	"testing"
)

func TestSettingsMenuHasNoLaterLine(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.openSettings()
	win.drawSettings(pageContext(1100, 700), pageRect)
	if text := strings.Join(win.set.texts, "\n"); strings.Contains(text, "Later") || strings.Contains(text, "coming next") {
		t.Fatalf("the menu still shows a later line:\n%s", text)
	}
}

func TestSettingsMenuHasATitleAndTheKazeGroup(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.openSettings()
	win.drawSettings(pageContext(1100, 700), pageRect)
	texts := win.set.texts
	title, group, about := slices.Index(texts, "Settings"), slices.Index(texts, "KAZE"), slices.Index(texts, "About")
	if title < 0 || group < 0 || about < 0 || !(title < group && group < about) {
		t.Fatalf("menu order: Settings %d, KAZE %d, About %d in %q", title, group, about, texts)
	}
}

func TestHeadingRowsStartCards(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.set.sections = []settingsSection{{
		title: "Kit",
		rows: func(*window) []settingRow {
			return []settingRow{
				{label: "One", ctrls: []ctrl{{kind: ctlToggle, id: "a"}}},
				{label: "Two", ctrls: []ctrl{{kind: ctlToggle, id: "b"}}},
				{heading: "More"},
				{label: "Three", ctrls: []ctrl{{kind: ctlToggle, id: "c"}}},
			}
		},
	}}
	win.openSettings()
	win.drawSettings(pageContext(1100, 700), pageRect)
	if len(win.set.cards) != 2 {
		t.Fatalf("%d cards, want 2", len(win.set.cards))
	}
	if !slices.Contains(win.set.texts, "MORE") {
		t.Fatalf("no section label in %q", win.set.texts)
	}
	in := func(id string, card int) bool {
		c := controlsOf(win, id)
		return len(c) == 1 && c[0].rect.In(win.set.cards[card])
	}
	if !in("a", 0) || !in("b", 0) || !in("c", 1) {
		t.Fatalf("controls not inside their cards: cards %v", win.set.cards)
	}
}

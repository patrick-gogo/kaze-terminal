package ui

import (
	"testing"

	"gioui.org/font"
)

func TestSettingsTextUsesTheSansFont(t *testing.T) {
	c := newChrome()
	gtx := pageContext(800, 600)
	_, regular := c.sans(gtx, "Appearance Settings", 13, c.th.ActiveFG, font.Normal)
	_, semi := c.sans(gtx, "Appearance Settings", 13, c.th.ActiveFG, font.SemiBold)
	_, mono := c.label(gtx, "Appearance Settings", 13, c.th.ActiveFG)
	if regular.X == 0 || semi.X <= regular.X {
		t.Fatalf("regular %v, semibold %v: the weights are not distinct faces", regular, semi)
	}
	if regular.X >= mono.X {
		t.Fatalf("sans %v is not narrower than mono %v: the sans face did not load", regular, mono)
	}
}

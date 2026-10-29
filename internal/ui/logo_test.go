package ui

import (
	"testing"
)

func TestMarkIsScaledOnceAndHasTheViolet(t *testing.T) {
	op1, img := markImage(18)
	if img == nil {
		t.Fatal("no mark image")
	}
	if b := img.Bounds(); b.Dx() != 18 || b.Dy() != 18 {
		t.Fatalf("mark is %v, want 18x18", b)
	}
	violet := false
	for y := 0; y < 18 && !violet; y++ {
		for x := 0; x < 18; x++ {
			c := img.NRGBAAt(x, y)
			if c.A > 200 && c.B > 200 && c.R > 100 && c.G < c.B {
				violet = true
				break
			}
		}
	}
	if !violet {
		t.Fatal("scaled mark has no opaque violet pixel")
	}
	op2, img2 := markImage(18)
	if img2 != img || op1 != op2 {
		t.Fatal("same size was decoded and scaled again")
	}
}

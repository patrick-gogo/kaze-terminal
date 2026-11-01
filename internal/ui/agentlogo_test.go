package ui

import (
	"image/color"
	"testing"
)

// The agent logos are the real marks, tinted to the asked colour, cached per size.
func TestAgentLogo(t *testing.T) {
	coral := color.NRGBA{R: 0xd9, G: 0x77, B: 0x57, A: 0xff}
	for _, name := range []string{"claude", "codex"} {
		op1, img := agentLogo(name, 14, coral)
		if img == nil || img.Bounds().Dx() != 14 || img.Bounds().Dy() != 14 {
			t.Fatalf("%s: image %v", name, img)
		}
		solid := 0
		for i := 0; i < len(img.Pix); i += 4 {
			if a := img.Pix[i+3]; a > 0 {
				if a >= 0x80 {
					solid++
				}
				if img.Pix[i] != coral.R || img.Pix[i+1] != coral.G || img.Pix[i+2] != coral.B {
					t.Fatalf("%s: pixel not tinted: %v", name, img.Pix[i:i+4])
				}
			}
		}
		if solid < 10 {
			t.Fatalf("%s: only %d solid pixels", name, solid)
		}
		if op2, img2 := agentLogo(name, 14, coral); img2 != img || op2 != op1 {
			t.Fatalf("%s: not cached", name)
		}
	}
	if _, img := agentLogo("other", 14, coral); img != nil {
		t.Fatal("unknown agent got a logo")
	}
}

// Claude's logo keeps its coral; the OpenAI logo follows the row's text colour.
func TestAgentLogoColor(t *testing.T) {
	fg := color.NRGBA{R: 1, G: 2, B: 3, A: 0xff}
	if got := agentLogoColor("claude", fg); got != claudeCoral {
		t.Fatalf("claude %v", got)
	}
	if got := agentLogoColor("codex", fg); got != fg {
		t.Fatalf("codex %v", got)
	}
}

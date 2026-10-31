package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBoardSetupHintOnUnreadableSettings(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{{"missing", nil}, {"empty", []byte{}}, {"invalid", []byte(`{`)}, {"wrong shape", []byte(`[]`)}} {
		t.Run(tc.name, func(t *testing.T) {
			win, _, _ := liveWindow(t, 1)
			win.claudePath = filepath.Join(t.TempDir(), "settings.json")
			if tc.data != nil {
				if err := os.WriteFile(win.claudePath, tc.data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			win.openBoard(0)
			win.drawBoard(pageContext(1200, 760), boardRect)
			if !win.board.missing || win.board.setup.Empty() {
				t.Fatal("missing setup hint/action")
			}
			win.boardPress(center(win.board.setup).Round())
			if !win.set.shown || win.sections()[win.set.section].title != "Agents" {
				t.Fatal("setup action did not open Agents")
			}
		})
	}
}

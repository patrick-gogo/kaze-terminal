package tabs_test

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStateObserverTracksStructureSelectionAndBackgroundCwd(t *testing.T) {
	m, starter := newManager(t)
	changes := make(chan struct{}, 32)
	m.SetStateObserver(func() {
		_, _ = m.State()
		select {
		case changes <- struct{}{}:
		default:
		}
	})
	a, err := m.New()
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.New()
	if err != nil {
		t.Fatal(err)
	}
	m.SetActive(a.ID)
	st, active := m.State()
	if len(st) != 2 || active != a.ID {
		t.Fatalf("%+v %v", st, active)
	}
	// The background shell changes folder without any UI frame.
	dir := filepath.Join(os.TempDir(), "background")
	u := url.URL{Path: "/" + strings.TrimPrefix(filepath.ToSlash(dir), "/")}
	go starter.fakes[1].outW.Write([]byte("\x1b]7;file://localhost" + u.EscapedPath() + "\x07"))
	waitFor(t, "background cwd", func() bool {
		st, _ := m.State()
		return len(st) == 2 && st[1].Dir == dir && st[1].Seq > 0
	})

	m.Close(b.ID)
	st, _ = m.State()
	if len(st) != 1 {
		t.Fatal("close not observed")
	}
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("no observer event")
	}
}

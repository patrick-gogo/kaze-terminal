package tabs_test

import (
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
	go starter.fakes[1].outW.Write([]byte("\x1b]7;file://localhost/tmp/background\x07"))
	waitFor(t, "background cwd", func() bool {
		st, _ := m.State()
		return len(st) == 2 && st[1].Dir == "/tmp/background" && st[1].Seq > 0
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

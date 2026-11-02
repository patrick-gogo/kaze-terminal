package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPaneTranscriptLiveAndReportCopy(t *testing.T) {
	win, _, r, a, _, _, _ := paneWindow(t)
	win.liveMgr.Store(win.mgr)
	main := filepath.Join(t.TempDir(), "s.jsonl")
	path := subagents.TranscriptPath(main, "x")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"Now the sidebar.\"},{\"type\":\"tool_use\",\"name\":\"Grep\"},{\"type\":\"thinking\",\"thinking\":\"hidden\"}]}}\n"), 0644)
	win.onNotify(notify.Msg{Tab: int(a.ID), Kind: notify.KindAgentStart, Session: "s", AgentID: "x", Transcript: main})
	floatFrames(t, win, r)
	k := subagents.Key{Session: "s", ID: "x"}
	win.openAgentPane(a.ID, k)
	floatFrames(t, win, r)
	p := win.fl.set(a.ID).find(win.fl.agentID(k))
	joined := func() string {
		var out []string
		for _, l := range p.wrapped {
			out = append(out, l.text)
		}
		return strings.Join(out, "|")
	}
	if got := joined(); !strings.Contains(got, "Now the sidebar.") || strings.Contains(got, "hidden") {
		t.Fatal(got)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"tool_use\",\"name\":\"SubagentHandback\",\"input\":{\"message\":\"The whole report.\"}}]}}\n")
	f.Close()
	win.onNotify(notify.Msg{Tab: int(a.ID), Kind: notify.KindAgentStop, Session: "s", AgentID: "x"})
	floatFrames(t, win, r)
	win.agentTranscriptTick(p, win.subs.Get(k), time.Now().Add(2*time.Second))
	floatFrames(t, win, r)
	if !strings.Contains(joined(), "The whole report.") {
		t.Fatal(joined())
	}
	var copied string
	win.toClipboard = func(s string) { copied = s }
	win.copyReport(pageContext(1000, 600), win.subs.Get(k))
	if copied != "The whole report." {
		t.Fatal(copied)
	}
}
func TestPaneTranscriptIdleAndCodexNeverRead(t *testing.T) {
	win, _, r, a, _, _, _ := paneWindow(t)
	win.liveMgr.Store(win.mgr)
	win.onNotify(notify.Msg{Tab: int(a.ID), Kind: notify.KindAgentStart, Session: "s", AgentID: "x", Agent: "codex", Transcript: filepath.Join(t.TempDir(), "s.jsonl")})
	floatFrames(t, win, r)
	k := subagents.Key{Session: "s", ID: "x"}
	win.openAgentPane(a.ID, k)
	floatFrames(t, win, r)
	p := win.fl.set(a.ID).find(win.fl.agentID(k))
	if _, ok := win.agentTranscriptTick(p, win.subs.Get(k), time.Now()); ok {
		t.Fatal("Codex timer")
	}
	if p.transcript != nil {
		t.Fatal("Codex transcript")
	}
	win.fl.remove(p.id)
	if _, ok := win.agentTranscriptTick(p, win.subs.Get(k), time.Now()); ok {
		t.Fatal("closed timer")
	}
}
func TestEmptyPaneReportSaysNoReport(t *testing.T) {
	if got := paneTexts(&subagents.Agent{State: subagents.Done}); !strings.Contains(got, "No report") {
		t.Fatal(got)
	}
}

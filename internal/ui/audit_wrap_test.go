package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/font"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
)

func TestAgentPaneCacheUsesReportContents(t *testing.T) {
	win := &window{ui: newChrome()}
	gtx := pageContext(800, 600)
	p := &pane{}
	e := subagents.Event{Tab: 1, Src: 1, Session: "s", AgentID: "a", AgentType: "default", At: t0}
	e.Kind = subagents.EvStart
	win.subs.Apply(e)
	e.Kind, e.Report = subagents.EvStop, "alpha"
	win.subs.Apply(e)
	a := win.subs.Get(subagents.Key{Session: "s", ID: "a"})
	win.agentWrapped(gtx, p, a, 400)
	// A whole next turn happens without drawing the hidden pane.
	e.Kind, e.At = subagents.EvStart, t0.Add(time.Minute)
	win.subs.Apply(e)
	e.Kind, e.Report = subagents.EvStop, "bravo"
	win.subs.Apply(e)
	got := win.agentWrapped(gtx, p, a, 400)
	if got[len(got)-1].text != "bravo" {
		t.Fatalf("old report retained: %+v", got)
	}
}

func TestAgentPaneWrapInvalidatesMetricsAndActivity(t *testing.T) {
	win := &window{ui: newChrome()}
	gtx := pageContext(800, 600)
	p := &pane{}
	a := &subagents.Agent{Prompt: strings.Repeat("word ", 40)}
	win.agentWrapped(gtx, p, a, 240)
	n := len(p.promptWrap.lines)
	gtx.Metric.PxPerSp = 2
	win.agentWrapped(gtx, p, a, 240)
	if len(p.promptWrap.lines) <= n {
		t.Fatal("same pixel width at larger font reused old wrapping")
	}
	a.Activity.Commands = 1
	got := win.agentWrapped(gtx, p, a, 240)
	var s strings.Builder
	for _, l := range got {
		s.WriteString(l.text)
	}
	if !strings.Contains(s.String(), "shell command") {
		t.Fatalf("activity not refreshed: %q", s.String())
	}
}

func TestWrappingNeedsNoPaintOperations(t *testing.T) {
	win := &window{ui: newChrome()}
	gtx := pageContext(800, 600)
	gtx.Ops = nil
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("wrapping attempted to record paint operations: %v", r)
		}
	}()
	for _, s := range []string{strings.Repeat("x", 24<<10), strings.Repeat("日本語café ", 20), "abc אבג def"} {
		lines := win.wrapSans(gtx, s, 12, 200)
		for _, l := range lines {
			if !utf8.ValidString(l) {
				t.Fatalf("invalid UTF-8: %q", l)
			}
			if w := win.ui.textWidth(gtx, l, 12, font.Font{Typeface: sansFace}); w > 201 {
				t.Fatalf("line too wide: %d", w)
			}
		}
		if strings.Join(strings.Fields(strings.Join(lines, "")), "") != strings.Join(strings.Fields(win.ui.fit(s)), "") {
			t.Fatalf("text lost: %q", lines)
		}
	}
}

func TestAgentPaneCachedWrapAllocatesNothing(t *testing.T) {
	win := &window{ui: newChrome()}
	gtx := pageContext(800, 600)
	p := &pane{}
	a := &subagents.Agent{State: subagents.Done, Prompt: "one\ntwo", Report: "three\nfour"}
	win.agentWrapped(gtx, p, a, 400)
	if n := testing.AllocsPerRun(5, func() { win.agentWrapped(gtx, p, a, 400) }); n != 0 {
		t.Fatalf("unchanged multiline pane allocated %v times", n)
	}
}

func BenchmarkAgentWrapLongToken(b *testing.B) {
	for _, n := range []int{2048, 8192} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			win := &window{ui: newChrome()}
			gtx := pageContext(800, 600)
			s := strings.Repeat("x", n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				gtx.Ops.Reset()
				win.wrapSans(gtx, s, 12, 400)
			}
		})
	}
}

package subagents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureLine(kind string, blocks ...map[string]any) string {
	b, _ := json.Marshal(map[string]any{"type": kind, "message": map[string]any{"content": blocks}})
	return string(b) + "\n"
}
func textBlock(s string) map[string]any { return map[string]any{"type": "text", "text": s} }
func toolBlock(s string) map[string]any { return map[string]any{"type": "tool_use", "name": s} }
func TestTranscriptIncrementalMessagesAndTools(t *testing.T) {
	p := filepath.Join(t.TempDir(), "agent-a.jsonl")
	first := fixtureLine("assistant", textBlock("Now the sidebar."), map[string]any{"type": "thinking", "thinking": "secret"}, toolBlock("Grep"), toolBlock("Bash"))
	os.WriteFile(p, []byte(first), 0644)
	var tr Transcript
	if e := tr.Read(p); e != nil {
		t.Fatal(e)
	}
	if len(tr.Messages) != 2 || tr.Messages[0].Text != "Now the sidebar." || tr.Messages[1].Tools.Searches != 1 || tr.Messages[1].Tools.Commands != 1 {
		t.Fatalf("%+v", tr.Messages)
	}
	offset, version := tr.Offset, tr.Version
	tr.Read(p)
	if tr.Offset != offset || tr.Version != version {
		t.Fatal("unchanged reread")
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
	next := fixtureLine("assistant", textBlock("Done."))
	f.WriteString(next[:len(next)-3])
	tr.Read(p)
	if len(tr.Messages) != 2 {
		t.Fatal("partial line parsed")
	}
	f.WriteString(next[len(next)-3:])
	f.Close()
	tr.Read(p)
	if len(tr.Messages) != 3 || tr.Report("") != "Done." {
		t.Fatalf("%+v", tr.Messages)
	}
}
func TestTranscriptReportOrderAndCleaning(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jsonl")
	s := fixtureLine("assistant", textBlock("last text"), map[string]any{"type": "tool_use", "name": "SubagentHandback", "input": map[string]any{"message": "whole\nreport\x1b[31m"}}) + fixtureLine("user", map[string]any{"type": "tool_result", "content": "Report delivered to your caller."})
	os.WriteFile(p, []byte(s), 0644)
	var tr Transcript
	tr.Read(p)
	if got := tr.Report(""); got != "whole\nreport[31m" {
		t.Fatal(got)
	}
	if tr.Report("hook") != "hook" {
		t.Fatal("hook priority")
	}
	if (new(Transcript)).Report("") != "No report" {
		t.Fatal("empty report")
	}
}
func TestTranscriptCapsAndOversizedLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jsonl")
	var b strings.Builder
	b.WriteString(strings.Repeat("x", MaxTranscriptLine+100) + "\n")
	for range MaxMessages + 20 {
		b.WriteString(fixtureLine("assistant", textBlock(strings.Repeat("a", 3000))))
	}
	b.WriteString(fixtureLine("assistant", textBlock("after")))
	os.WriteFile(p, []byte(b.String()), 0644)
	var tr Transcript
	for i := 0; i < 20 && tr.Offset < int64(b.Len()); i++ {
		tr.Read(p)
	}
	if tr.Offset != int64(b.Len()) || !tr.Dropped || len(tr.Messages) > MaxMessages || tr.Bytes > MaxTranscriptBytes || tr.Report("") != "after" {
		t.Fatal(len(tr.Messages), tr.Bytes, tr.Offset, b.Len())
	}
}
func TestTranscriptMissingTruncatedAndReplaced(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jsonl")
	var tr Transcript
	if tr.Read(p) == nil {
		t.Fatal("missing")
	}
	os.WriteFile(p, []byte(fixtureLine("assistant", textBlock("old"))), 0644)
	tr.Read(p)
	os.WriteFile(p, []byte(fixtureLine("assistant", textBlock("new"))), 0644)
	q := p + ".new"
	os.WriteFile(q, []byte(fixtureLine("assistant", textBlock("replacement"))), 0644)
	os.Rename(q, p)
	tr.Read(p)
	if tr.Report("") != "replacement" {
		t.Fatal(tr.Report(""))
	}
}
func TestTranscriptPath(t *testing.T) {
	main := filepath.Join(t.TempDir(), "s.jsonl")
	want := filepath.Join(strings.TrimSuffix(main, ".jsonl"), "subagents", "agent-ab12.jsonl")
	if p := TranscriptPath(main, "ab12"); p != want {
		t.Fatal(p)
	}
	for _, id := range []string{"../x", "a/b", "", "a\\b"} {
		if TranscriptPath(main, id) != "" {
			t.Fatal(id)
		}
	}
}

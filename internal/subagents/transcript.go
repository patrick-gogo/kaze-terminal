package subagents

import (
	"encoding/json"
	"fmt"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxMessages          = 128
	MaxTranscriptBytes   = 128 << 10
	MaxTranscriptLine    = 256 << 10
	transcriptReadBudget = 256 << 10
)

// Transcript keeps a bounded tail. Offset counts bytes consumed, including a
// partial record; pending holds only that record until its newline arrives.
// The pane owns this value and calls Read only while visible.
type Transcript struct {
	Messages           []TranscriptMessage
	Offset             int64
	Bytes              int
	Dropped            bool
	Version            uint64
	Available          bool
	pending            []byte
	skipping           bool
	info               os.FileInfo
	handback, lastText string
}
type TranscriptMessage struct {
	Text  string
	Tools ToolCounts
}
type ToolCounts struct{ Searches, Reads, Directories, Commands, Edits, Web, Other int }

func (c *ToolCounts) add(name string) {
	switch name {
	case "Grep", "Glob":
		c.Searches++
	case "Read":
		c.Reads++
	case "LS", "ListDirectory":
		c.Directories++
	case "Bash":
		c.Commands++
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		c.Edits++
	case "WebSearch", "WebFetch":
		c.Web++
	default:
		c.Other++
	}
}
func (c ToolCounts) String() string {
	var parts []string
	add := func(n int, verb, one, many string) {
		if n == 0 {
			return
		}
		if n == 1 {
			parts = append(parts, fmt.Sprintf("%s 1 %s", verb, one))
		} else {
			parts = append(parts, fmt.Sprintf("%s %d %s", verb, n, many))
		}
	}
	add(c.Searches, "searched for", "pattern", "patterns")
	add(c.Reads, "read", "file", "files")
	add(c.Directories, "listed", "directory", "directories")
	add(c.Commands, "ran", "shell command", "shell commands")
	add(c.Edits, "edited", "file", "files")
	add(c.Web, "made", "web request", "web requests")
	add(c.Other, "used", "other tool", "other tools")
	s := strings.Join(parts, ", ")
	if s != "" {
		s = strings.ToUpper(s[:1]) + s[1:]
	}
	return s
}
func TranscriptPath(main, id string) string {
	if !strings.HasSuffix(main, ".jsonl") || !filepath.IsAbs(main) || id == "" || len(id) > 256 {
		return ""
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return ""
		}
	}
	return filepath.Join(strings.TrimSuffix(main, ".jsonl"), "subagents", "agent-"+id+".jsonl")
}
func (tr *Transcript) Read(path string) error {
	f, e := os.Open(path)
	if e != nil {
		tr.Available = false
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() {
		tr.Available = false
		return fmt.Errorf("not a regular file")
	}
	if tr.info != nil && (!os.SameFile(tr.info, st) || st.Size() < tr.Offset) {
		v := tr.Version
		*tr = Transcript{Version: v + 1}
	}
	tr.info = st
	tr.Available = true
	if _, e = f.Seek(tr.Offset, io.SeekStart); e != nil {
		return e
	}
	var buf [32 << 10]byte
	for budget := transcriptReadBudget; budget > 0; {
		n, e := f.Read(buf[:min(len(buf), budget)])
		if n > 0 {
			tr.Offset += int64(n)
			budget -= n
			tr.feed(buf[:n])
		}
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		if n == 0 {
			break
		}
	}
	return nil
}
func (tr *Transcript) feed(data []byte) {
	for _, b := range data {
		if b == '\n' {
			if !tr.skipping && len(tr.pending) > 0 {
				tr.record(tr.pending)
			}
			tr.pending = tr.pending[:0]
			tr.skipping = false
			continue
		}
		if tr.skipping {
			continue
		}
		if len(tr.pending) >= MaxTranscriptLine {
			tr.pending = tr.pending[:0]
			tr.skipping = true
			tr.Dropped = true
			tr.Version++
			continue
		}
		tr.pending = append(tr.pending, b)
	}
}
func (tr *Transcript) record(line []byte) {
	var entry struct {
		Type    string `json:"type"`
		Message struct {
			Content []struct {
				Type, Text, Name string
				Input            struct {
					Message string `json:"message"`
				} `json:"input"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "assistant" {
		return
	}
	for _, b := range entry.Message.Content {
		switch b.Type {
		case "text":
			s := notify.CleanReport(b.Text, notify.MaxReport)
			if strings.TrimSpace(s) == "" {
				continue
			}
			tr.lastText = s
			tr.append(TranscriptMessage{Text: s})
		case "tool_use":
			if b.Name == "SubagentHandback" {
				s := notify.CleanReport(b.Input.Message, notify.MaxReport)
				if strings.TrimSpace(s) != "" {
					tr.handback = s
					tr.Version++
				}
				continue
			}
			if len(tr.Messages) > 0 && tr.Messages[len(tr.Messages)-1].Text == "" {
				tr.Messages[len(tr.Messages)-1].Tools.add(b.Name)
				tr.Version++
			} else {
				m := TranscriptMessage{}
				m.Tools.add(b.Name)
				tr.append(m)
			}
		}
	}
}
func (tr *Transcript) append(m TranscriptMessage) {
	tr.Messages = append(tr.Messages, m)
	tr.Bytes += len(m.Text)
	tr.Version++
	for len(tr.Messages) > MaxMessages || tr.Bytes > MaxTranscriptBytes {
		tr.Bytes -= len(tr.Messages[0].Text)
		copy(tr.Messages, tr.Messages[1:])
		tr.Messages[len(tr.Messages)-1] = TranscriptMessage{}
		tr.Messages = tr.Messages[:len(tr.Messages)-1]
		tr.Dropped = true
	}
}
func (tr *Transcript) Report(hook string) string {
	if s := notify.CleanReport(hook, notify.MaxReport); strings.TrimSpace(s) != "" {
		return s
	}
	if tr.handback != "" {
		return tr.handback
	}
	if tr.lastText != "" {
		return tr.lastText
	}
	return "No report"
}

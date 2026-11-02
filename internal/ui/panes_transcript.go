package ui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"time"
)

const agentTranscriptPoll = time.Second

func (win *window) agentTranscriptPath(a *subagents.Agent) string {
	if a.Engine == "codex" {
		return ""
	}
	main := a.Transcript
	if main == "" {
		state := win.agentOf(tabs.TabID(a.Tab))
		if state.agentName() == "codex" || state.session != a.Key.Session {
			return ""
		}
		main = state.transcript
	}
	return subagents.TranscriptPath(main, a.Key.ID)
}

// Called only from visible-pane drawing. A closed/hidden pane cannot perform
// reads or schedule polling. Each read consumes at most 256 KiB.
func (win *window) agentTranscriptTick(p *pane, a *subagents.Agent, now time.Time) (time.Time, bool) {
	if p.hidden || p.kind != paneAgent || win.fl.owner(p.id) == 0 || win.fl.owner(p.id) != win.paneOwner() {
		return time.Time{}, false
	}
	path := win.agentTranscriptPath(a)
	if path == "" {
		p.transcript = nil
		return time.Time{}, false
	}
	if p.transcript == nil || p.transcriptPath != path || p.transcriptStarted != a.Started {
		p.transcript = &subagents.Transcript{}
		p.transcriptPath = path
		p.transcriptStarted = a.Started
		p.transcriptAt = time.Time{}
	}
	if !now.Before(p.transcriptAt.Add(agentTranscriptPoll)) {
		_ = p.transcript.Read(path)
		p.transcriptAt = now
	}
	return p.transcriptAt.Add(agentTranscriptPoll), true
}
func (win *window) pollAgentPane(gtx layout.Context, p *pane, a *subagents.Agent) {
	if at, ok := win.agentTranscriptTick(p, a, gtx.Now); ok {
		gtx.Execute(op.InvalidateCmd{At: at})
	}
}
func (win *window) agentReport(a *subagents.Agent) string {
	for _, s := range win.fl.sets {
		for _, p := range s.panes {
			if p.kind == paneAgent && p.agent == a.Key && p.transcript != nil && p.transcript.Available {
				return p.transcript.Report(a.Report)
			}
		}
	}
	return new(subagents.Transcript).Report(a.Report)
}
func paneConversation(p *pane, a *subagents.Agent) []agentLine {
	tr := p.transcript
	if tr == nil || !tr.Available || len(tr.Messages) == 0 {
		out := agentPaneLines(a)
		if a.State == subagents.Done && tr != nil && tr.Available {
			out[len(out)-1].text = tr.Report(a.Report)
		}
		return out
	}
	var out []agentLine
	if prompt := trimBlock(a.Prompt); prompt != "" {
		out = append(out, agentLine{linePromptHead, "Prompt"}, agentLine{linePrompt, prompt})
	}
	if tr.Dropped {
		out = append(out, agentLine{lineActivity, "… earlier messages hidden"})
	}
	for _, m := range tr.Messages {
		if m.Text != "" {
			out = append(out, agentLine{lineMessage, m.Text})
		} else if s := m.Tools.String(); s != "" {
			out = append(out, agentLine{lineActivity, s})
		}
	}
	if a.State == subagents.Waiting {
		out = append(out, agentLine{lineWaiting, "Waiting for your approval in the session below"})
	}
	if a.State == subagents.Done {
		out = append(out, agentLine{lineReportHead, "Report"}, agentLine{lineReport, tr.Report(a.Report)})
	}
	return out
}

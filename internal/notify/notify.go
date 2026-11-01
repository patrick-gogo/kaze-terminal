// Package notify is a loopback-only listener that Claude Code hooks (via
// `kaze notify`) use to tell kaze a tab is done or needs the user.
package notify

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

type Kind string

const (
	KindDone  Kind = "done"
	KindNeeds Kind = "needs"
	// KindUsage carries Claude's plan usage from `kaze statusline`; it is not an alert.
	KindUsage Kind = "usage"
	// KindPrompt reports a submitted prompt and the slash command it starts with; not an alert.
	KindPrompt Kind = "prompt"
	// KindInterrupt ends running without raising an alert.
	KindInterrupt Kind = "interrupt"
	// KindSession says an agent session started (or restarted after /clear) in the
	// tab: it names the agent before the first prompt. Never an alert.
	KindSession Kind = "session"
	// The subagent kinds are Claude only and never alerts.
	KindAgentStart Kind = "agent-start"
	KindAgentStop  Kind = "agent-stop"
	KindTool       Kind = "tool" // a step inside a subagent, or the main conversation's Agent call
)

const maxArgsRunes = 120

// Caps for the session identity and the subagent fields.
const (
	MaxSession    = 128      // bytes
	MaxPath       = 1024     // bytes
	MaxAgentField = 128      // runes
	MaxReport     = 24 << 10 // bytes
)

const (
	maxBody      = 32 << 10 // room for a subagent's report
	readDeadline = 2 * time.Second
	maxConns     = 32
	maxBackoff   = time.Second
)

type Msg struct {
	Agent  string `json:"agent,omitempty"` // empty means Claude for old hook commands
	Tab    int    `json:"tab"`
	Kind   Kind   `json:"kind"`
	Reason string `json:"reason,omitempty"`
	Usage  *Usage `json:"usage,omitempty"` // only for KindUsage
	// Command and Args are only for KindPrompt.
	Command string `json:"command,omitempty"`
	Args    string `json:"args,omitempty"`
	// The Claude session the hook ran in; any kind may carry them.
	Session    string `json:"session,omitempty"`
	Transcript string `json:"transcript,omitempty"`
	Cwd        string `json:"cwd,omitempty"`
	// Subagent fields: AgentID also on needs, Report only on agent-stop.
	AgentID     string `json:"agent_id,omitempty"`
	AgentType   string `json:"agent_type,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
	Report      string `json:"report,omitempty"`
	Token       string `json:"token"`
}

// Window is one rate-limit window: percent used and its reset time (Unix seconds, 0 = unknown).
type Window struct {
	Pct    float64 `json:"pct"`
	Resets int64   `json:"resets,omitempty"`
}

// Usage is what Claude Code reported; nil fields were absent.
type Usage struct {
	FiveHour *Window  `json:"five_hour,omitempty"`
	Week     *Window  `json:"week,omitempty"`
	Ctx      *float64 `json:"ctx,omitempty"`
}

type Server struct {
	ln       net.Listener
	token    string
	onMsg    func(Msg)
	deadline time.Duration
	sem      chan struct{}

	mu     sync.Mutex
	closed bool
	conns  map[net.Conn]struct{}
	wg     sync.WaitGroup
}

// Listen binds 127.0.0.1 on an OS-chosen port. onMsg is called from a
// per-connection goroutine, possibly concurrently, and must not block (Close
// waits for it): the UI side should hand off with a non-blocking send.
func Listen(onMsg func(Msg)) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return serve(ln, onMsg, readDeadline), nil
}

func serve(ln net.Listener, onMsg func(Msg), deadline time.Duration) *Server {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err) // crypto/rand failing is unrecoverable
	}
	s := &Server{
		ln: ln, token: hex.EncodeToString(raw[:]), onMsg: onMsg, deadline: deadline,
		sem: make(chan struct{}, maxConns), conns: map[net.Conn]struct{}{},
	}
	s.wg.Add(1)
	go s.accept()
	return s
}

func (s *Server) Addr() string  { return s.ln.Addr().String() }
func (s *Server) Token() string { return s.token }

// Close stops accepting, drops live connections and waits for handlers.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	err := s.ln.Close()
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return err
}

func (s *Server) accept() {
	defer s.wg.Done()
	backoff := 50 * time.Millisecond
	for {
		c, err := s.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || s.isClosed() {
				return
			}
			time.Sleep(backoff) // transient (e.g. fd exhaustion): don't spin
			if backoff *= 2; backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = 50 * time.Millisecond
		select {
		case s.sem <- struct{}{}:
		default:
			c.Close() // over the cap
			continue
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			<-s.sem
			c.Close()
			return
		}
		s.conns[c] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		// Own goroutine so a silent client cannot stall the others.
		go func() {
			defer s.wg.Done()
			defer func() {
				s.mu.Lock()
				delete(s.conns, c)
				s.mu.Unlock()
				<-s.sem
			}()
			s.handle(c)
		}()
	}
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(s.deadline))
	// One byte past the cap lets us tell "exactly the cap" from "over it".
	dec := json.NewDecoder(io.LimitReader(c, maxBody+1))
	var m Msg
	if dec.Decode(&m) != nil || dec.InputOffset() > maxBody {
		return
	}
	if subtle.ConstantTimeCompare([]byte(m.Token), []byte(s.token)) != 1 {
		return
	}
	if m.Agent != "" && m.Agent != "claude" && m.Agent != "codex" {
		return
	}
	if m.Kind == KindInterrupt && m.Agent != "codex" {
		return
	}
	if m.Kind == KindUsage && m.Agent == "codex" {
		return
	}
	m.Session, m.Transcript, m.Cwd = cutBytes(CleanLine(m.Session, MaxSession), MaxSession),
		cutBytes(CleanLine(m.Transcript, MaxPath), MaxPath), cutBytes(CleanLine(m.Cwd, MaxPath), MaxPath)
	m.AgentID, m.AgentType = CleanLine(m.AgentID, MaxAgentField), CleanLine(m.AgentType, MaxAgentField)
	m.Tool, m.Summary = CleanLine(m.Tool, MaxAgentField), CleanLine(m.Summary, maxArgsRunes)
	m.Description = CleanLine(m.Description, maxArgsRunes)
	if m.Kind != KindAgentStop {
		m.Report = ""
	}
	if m.Kind != KindTool {
		m.Tool, m.Summary, m.Description = "", "", ""
	}
	switch m.Kind {
	case KindAgentStart, KindAgentStop, KindTool:
		m.Usage, m.Reason, m.Command, m.Args = nil, "", "", ""
		m.Report = CleanReport(m.Report, MaxReport)
	case KindDone, KindNeeds, KindInterrupt:
		m.Usage = nil
		if m.Kind != KindNeeds {
			m.AgentID, m.AgentType = "", ""
		}
	case KindSession:
		m.Usage, m.Reason, m.Command, m.Args, m.AgentID, m.AgentType = nil, "", "", "", "", ""
	case KindUsage:
		if m.Usage == nil {
			return
		}
	case KindPrompt:
		m.Usage, m.Reason = nil, ""
		if !ValidAgentCommand(m.Command, m.Agent) {
			m.Command = ""
		}
		m.Args = CleanArgs(m.Args)
	default:
		return
	}
	m.Token = ""
	if s.onMsg == nil || s.isClosed() {
		return
	}
	defer func() { recover() }() // a bad callback must not kill the server
	s.onMsg(m)
}

// ValidCommand reports whether c looks like /name with a safe charset, so a
// hostile hook cannot put escape sequences into the UI.
func ValidCommand(c string) bool { return ValidAgentCommand(c, "claude") }

// ValidAgentCommand additionally accepts Codex dollar-prefixed skill names.
func ValidAgentCommand(c, agent string) bool {
	if len(c) < 2 || len(c) > 65 || (c[0] != '/' && !(agent == "codex" && c[0] == '$')) {
		return false
	}
	for i := 1; i < len(c); i++ {
		b := c[i]
		alnum := b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
		if !alnum && (i == 1 || !strings.ContainsRune(":_.-", rune(b))) {
			return false
		}
	}
	return true
}

// CleanArgs drops control, format (bidi) and line/paragraph separator characters and cuts to maxArgsRunes.
func CleanArgs(s string) string { return CleanLine(s, maxArgsRunes) }

// CleanLine is CleanArgs with its own cap in runes.
func CleanLine(s string, max int) string {
	out := make([]rune, 0, min(max, len(s)))
	for _, r := range s {
		if len(out) == max {
			break
		}
		if unsafeRune(r) {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// CleanReport keeps newlines and tabs, drops what CleanLine drops, and cuts
// to max bytes on a rune boundary.
func CleanReport(s string, max int) string {
	var b strings.Builder
	for _, r := range s {
		if r != '\n' && r != '\t' && unsafeRune(r) {
			continue
		}
		if b.Len()+utf8.RuneLen(r) > max {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func unsafeRune(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029 || r == utf8.RuneError
}

// cutBytes cuts s to max bytes on a rune boundary.
func cutBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

// Send delivers one message; used by `kaze notify`. timeout covers dial and
// write together.
func Send(addr, token string, m Msg, timeout time.Duration) error {
	m.Token = token
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	d := time.Now().Add(timeout)
	c, err := (&net.Dialer{Deadline: d}).Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer c.Close()
	c.SetDeadline(d)
	_, err = c.Write(b)
	return err
}

// FitReport cleans a report and cuts it so its JSON encoding fits in max
// bytes: escaping can double a report full of code or HTML, and the server
// drops a body over its cap.
func FitReport(s string, max int) string {
	s = CleanReport(s, max)
	enc := func(s string) int { b, _ := json.Marshal(s); return len(b) }
	if enc(s) <= max {
		return s
	}
	lo, hi := 0, len(s) // the longest prefix that fits
	for lo < hi {
		if mid := (lo + hi + 1) / 2; enc(cutBytes(s, mid)) <= max {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return cutBytes(s, lo)
}

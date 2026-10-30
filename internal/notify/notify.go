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
)

type Kind string

const (
	KindDone  Kind = "done"
	KindNeeds Kind = "needs"
	// KindUsage carries Claude's plan usage from `kaze statusline`; it is not an alert.
	KindUsage Kind = "usage"
	// KindPrompt reports a submitted prompt and the slash command it starts with; not an alert.
	KindPrompt Kind = "prompt"
)

const maxArgsRunes = 120

const (
	maxBody      = 4 * 1024
	readDeadline = 2 * time.Second
	maxConns     = 32
	maxBackoff   = time.Second
)

type Msg struct {
	Tab    int    `json:"tab"`
	Kind   Kind   `json:"kind"`
	Reason string `json:"reason,omitempty"`
	Usage  *Usage `json:"usage,omitempty"` // only for KindUsage
	// Command and Args are only for KindPrompt.
	Command string `json:"command,omitempty"`
	Args    string `json:"args,omitempty"`
	Token   string `json:"token"`
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
	switch m.Kind {
	case KindDone, KindNeeds:
		m.Usage = nil
	case KindUsage:
		if m.Usage == nil {
			return
		}
	case KindPrompt:
		m.Usage, m.Reason = nil, ""
		if !ValidCommand(m.Command) {
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
func ValidCommand(c string) bool {
	if len(c) < 2 || len(c) > 65 || c[0] != '/' {
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
func CleanArgs(s string) string {
	out := make([]rune, 0, maxArgsRunes)
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029 {
			continue
		}
		if out = append(out, r); len(out) == maxArgsRunes {
			break
		}
	}
	return string(out)
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

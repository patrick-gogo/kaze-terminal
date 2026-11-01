package notify

import (
	"encoding/json"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func start(t *testing.T) (*Server, chan Msg) {
	t.Helper()
	ch := make(chan Msg, 8)
	s, err := Listen(func(m Msg) { ch <- m })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, ch
}

func expectNone(t *testing.T, ch chan Msg) {
	t.Helper()
	select {
	case m := <-ch:
		t.Fatalf("unexpected message: %+v", m)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestRoundTrip(t *testing.T) {
	s, ch := start(t)
	err := Send(s.Addr(), s.Token(), Msg{Tab: 3, Kind: KindNeeds, Reason: "permission"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-ch:
		if m.Tab != 3 || m.Kind != KindNeeds || m.Reason != "permission" {
			t.Fatalf("got %+v", m)
		}
		if m.Token != "" {
			t.Fatalf("token leaked: %q", m.Token)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRejectsBadToken(t *testing.T) {
	s, ch := start(t)
	Send(s.Addr(), "deadbeef", Msg{Tab: 1, Kind: KindDone}, time.Second)
	expectNone(t, ch)
}

func TestRejectsOversized(t *testing.T) {
	s, ch := start(t)
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindDone, Reason: strings.Repeat("x", 40*1024)}, time.Second)
	expectNone(t, ch)
}

func TestRejectsUnknownKind(t *testing.T) {
	s, ch := start(t)
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: Kind("bogus")}, time.Second)
	expectNone(t, ch)
}

func TestListensOnLoopbackOnly(t *testing.T) {
	s, _ := start(t)
	if !strings.HasPrefix(s.Addr(), "127.0.0.1:") {
		t.Fatalf("addr %q", s.Addr())
	}
	if len(s.Token()) != 64 {
		t.Fatalf("token len %d", len(s.Token()))
	}
}

func TestSlowClientDoesNotBlockOthers(t *testing.T) {
	s, ch := start(t)
	c, err := net.DialTimeout("tcp", s.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := Send(s.Addr(), s.Token(), Msg{Tab: 2, Kind: KindDone}, time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-ch:
		if m.Tab != 2 {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked by slow client")
	}
}

// fakeLn returns one temporary error before delegating.
type fakeLn struct {
	net.Listener
	once sync.Once
	hit  chan struct{}
}

type tempErr struct{}

func (tempErr) Error() string   { return "temporary" }
func (tempErr) Timeout() bool   { return false }
func (tempErr) Temporary() bool { return true }

func (f *fakeLn) Accept() (net.Conn, error) {
	var first bool
	f.once.Do(func() { first = true })
	if first {
		close(f.hit)
		return nil, tempErr{}
	}
	return f.Listener.Accept()
}

func TestAcceptSurvivesTemporaryError(t *testing.T) {
	real, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan Msg, 1)
	f := &fakeLn{Listener: real, hit: make(chan struct{})}
	s := serve(f, func(m Msg) { ch <- m }, readDeadline)
	defer s.Close()
	select {
	case <-f.hit:
	case <-time.After(time.Second):
		t.Fatal("fake error never served")
	}
	if err := Send(s.Addr(), s.Token(), Msg{Tab: 4, Kind: KindDone}, time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-ch:
		if m.Tab != 4 {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("accept loop died after a temporary error")
	}
}

func TestCloseIsFastWithSilentClient(t *testing.T) {
	s, _ := start(t)
	c, err := net.DialTimeout("tcp", s.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	time.Sleep(50 * time.Millisecond) // let the server register the conn
	t0 := time.Now()
	s.Close()
	if d := time.Since(t0); d > 200*time.Millisecond {
		t.Fatalf("Close took %v", d)
	}
}

func TestCapsConnectionsAndDoesNotLeak(t *testing.T) {
	ch := make(chan Msg, 4)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := serve(ln, func(m Msg) { ch <- m }, 400*time.Millisecond)
	var idle []net.Conn
	for i := 0; i < 40; i++ {
		c, err := net.DialTimeout("tcp", s.Addr(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		idle = append(idle, c)
	}
	defer func() {
		for _, c := range idle {
			c.Close()
		}
	}()
	deadline := time.Now().Add(2500 * time.Millisecond)
	got := false
	for time.Now().Before(deadline) && !got {
		time.Sleep(100 * time.Millisecond)
		Send(s.Addr(), s.Token(), Msg{Tab: 9, Kind: KindDone}, 200*time.Millisecond)
		select {
		case <-ch:
			got = true
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !got {
		t.Fatal("valid message never got through the idle flood")
	}
	s.Close()
	s.mu.Lock()
	n := len(s.conns)
	s.mu.Unlock()
	if n != 0 || len(s.sem) != 0 {
		t.Fatalf("leak: conns=%d sem=%d", n, len(s.sem))
	}
}

func TestSendHonorsOverallTimeout(t *testing.T) {
	const to = 300 * time.Millisecond
	t0 := time.Now()
	Send("10.255.255.1:9", "x", Msg{Tab: 1, Kind: KindDone}, to)
	if d := time.Since(t0); d > to+100*time.Millisecond {
		t.Fatalf("Send took %v", d)
	}
}

func TestDeliversWithoutHalfClose(t *testing.T) {
	s, ch := start(t)
	c, err := net.DialTimeout("tcp", s.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	b, _ := json.Marshal(Msg{Tab: 5, Kind: KindDone, Token: s.Token()})
	c.Write(b) // no CloseWrite, connection stays open
	select {
	case m := <-ch:
		if m.Tab != 5 {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("waited for EOF")
	}
}

func rawSend(t *testing.T, addr string, b []byte) {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write(b)
}

func sized(t *testing.T, token string, n int) []byte {
	t.Helper()
	m := Msg{Tab: 1, Kind: KindDone, Token: token}
	base, _ := json.Marshal(Msg{Tab: 1, Kind: KindDone, Reason: "a", Token: token})
	m.Reason = strings.Repeat("a", n-len(base)+1)
	b, _ := json.Marshal(m)
	if len(b) != n {
		t.Fatalf("built %d bytes, want %d", len(b), n)
	}
	return b
}

func TestBodyCapBoundary(t *testing.T) {
	s, ch := start(t)
	rawSend(t, s.Addr(), sized(t, s.Token(), maxBody))
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("maxBody bytes should be accepted")
	}
	rawSend(t, s.Addr(), sized(t, s.Token(), maxBody+1))
	expectNone(t, ch)
}

func TestOversizedSendReturnsPromptly(t *testing.T) {
	s, ch := start(t)
	t0 := time.Now()
	err := Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindDone, Reason: strings.Repeat("x", 40*1024)}, time.Second)
	if err != nil || time.Since(t0) > 500*time.Millisecond {
		t.Fatalf("Send is fire and forget: err=%v after %v", err, time.Since(t0))
	}
	expectNone(t, ch)
}

func TestReadDeadlineDropsSlowMessage(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	ch := make(chan Msg, 1)
	s := serve(ln, func(m Msg) { ch <- m }, 150*time.Millisecond)
	defer s.Close()
	c, err := net.DialTimeout("tcp", s.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	b, _ := json.Marshal(Msg{Tab: 1, Kind: KindDone, Token: s.Token()})
	c.Write(b[:len(b)/2])
	time.Sleep(300 * time.Millisecond)
	c.Write(b[len(b)/2:])
	expectNone(t, ch)
}

func TestNilAndPanickingOnMsg(t *testing.T) {
	s, err := Listen(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindDone}, time.Second); err != nil {
		t.Fatal(err)
	}
	ch := make(chan Msg, 2)
	n := 0
	p, _ := Listen(func(m Msg) {
		n++
		if n == 1 {
			panic("boom")
		}
		ch <- m
	})
	defer p.Close()
	Send(p.Addr(), p.Token(), Msg{Tab: 1, Kind: KindDone}, time.Second)
	time.Sleep(100 * time.Millisecond)
	Send(p.Addr(), p.Token(), Msg{Tab: 2, Kind: KindDone}, time.Second)
	select {
	case m := <-ch:
		if m.Tab != 2 {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server died after onMsg panic")
	}
}

func TestUsageRoundTrip(t *testing.T) {
	s, ch := start(t)
	ctx := 34.5
	u := &Usage{FiveHour: &Window{Pct: 23.5, Resets: 1738425600}, Week: &Window{Pct: 41.2}, Ctx: &ctx}
	if err := Send(s.Addr(), s.Token(), Msg{Tab: 2, Kind: KindUsage, Usage: u}, time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-ch:
		if m.Kind != KindUsage || m.Usage == nil || m.Usage.FiveHour.Pct != 23.5 || m.Usage.FiveHour.Resets != 1738425600 ||
			m.Usage.Week.Pct != 41.2 || m.Usage.Ctx == nil || *m.Usage.Ctx != 34.5 {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestUsageWithoutPayloadIsDropped(t *testing.T) {
	s, ch := start(t)
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindUsage}, time.Second)
	expectNone(t, ch)
}

func TestAlertsCarryNoUsage(t *testing.T) {
	s, ch := start(t)
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindDone, Usage: &Usage{Week: &Window{Pct: 9}}}, time.Second)
	select {
	case m := <-ch:
		if m.Kind != KindDone || m.Usage != nil {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestServerCleansPrompt(t *testing.T) {
	s, ch := start(t)
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindPrompt, Command: "/x\x1b[31m", Args: "a\nb\x1bc"}, time.Second)
	m := <-ch
	if m.Command != "" || m.Args != "abc" {
		t.Fatalf("got %+v", m)
	}
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindPrompt, Command: "/ship-ticket", Args: strings.Repeat("あ", 300),
		Usage: &Usage{}}, time.Second)
	m = <-ch
	if m.Command != "/ship-ticket" || len([]rune(m.Args)) != 120 || m.Usage != nil {
		t.Fatalf("got %+v", m)
	}
}

func TestServerCleansPromptReasonAndInvisibles(t *testing.T) {
	s, ch := start(t)
	args := "a" + string(rune(0x202e)) + "b" + string(rune(0x2028)) + "c" + string(rune(0x2029)) + "d"
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindPrompt, Command: "/x", Args: args, Reason: "r"}, time.Second)
	m := <-ch
	if m.Args != "abcd" || m.Reason != "" {
		t.Fatalf("got %+v", m)
	}
	if ValidCommand("/"+strings.Repeat("a", 5000)) || !ValidCommand("/ship-ticket") {
		t.Fatal("ValidCommand")
	}
}

func TestAgentProtocolRejectsUnsupportedCombinations(t *testing.T) {
	s, ch := start(t)
	for _, m := range []Msg{{Tab: 1, Kind: KindDone, Agent: "other"}, {Tab: 1, Kind: KindUsage, Agent: "codex", Usage: &Usage{}}, {Tab: 1, Kind: KindInterrupt, Agent: "claude"}} {
		if err := Send(s.Addr(), s.Token(), m, time.Second); err != nil {
			t.Fatal(err)
		}
	}
	expectNone(t, ch)
}

func recv(t *testing.T, ch chan Msg) Msg {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	return Msg{}
}

func TestMsgSubagentFieldsRoundTrip(t *testing.T) {
	s, ch := start(t)
	in := Msg{Tab: 2, Kind: KindTool, Session: "c5b9e894", Transcript: "/p/t.jsonl", Cwd: "/p",
		AgentID: "a98", AgentType: "Explore", Tool: "Grep", Summary: "auth in src", Description: "Find auth"}
	if err := Send(s.Addr(), s.Token(), in, time.Second); err != nil {
		t.Fatal(err)
	}
	m := recv(t, ch)
	in.Token = ""
	if m != in {
		t.Fatalf("got %+v, want %+v", m, in)
	}
}

func TestServerAcceptsLargeReport(t *testing.T) {
	s, ch := start(t)
	rep := strings.Repeat("r", MaxReport)
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindAgentStop, AgentID: "a1", Report: rep}, time.Second)
	if m := recv(t, ch); m.Report != rep {
		t.Fatalf("report len %d", len(m.Report))
	}
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindAgentStop, AgentID: "a1", Report: strings.Repeat("r", 40<<10)}, time.Second)
	expectNone(t, ch)
}

func TestServerCleansSubagentFields(t *testing.T) {
	s, ch := start(t)
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindAgentStart, AgentID: "a\x1b1", AgentType: strings.Repeat("e", 300),
		Report: "x", Summary: "s"}, time.Second)
	m := recv(t, ch)
	if m.AgentID != "a1" || len([]rune(m.AgentType)) != MaxAgentField || m.Report != "" || m.Summary != "" {
		t.Fatalf("got %+v", m)
	}
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindAgentStop, Report: "ok\nfine\x07"}, time.Second)
	if m := recv(t, ch); m.Report != "ok\nfine" {
		t.Fatalf("got %q", m.Report)
	}
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindTool, Agent: "codex", AgentID: "t1", Tool: "Bash", Reason: "x"}, time.Second)
	if m := recv(t, ch); m.Agent != "codex" || m.Tool != "Bash" || m.Reason != "" {
		t.Fatalf("codex step %+v", m)
	}
}

func TestCleanLineCapsAndStrips(t *testing.T) {
	if got := CleanLine("a\x1b[31mb\u202ec\nd", 3); got != "a[3" {
		t.Fatalf("got %q", got)
	}
	if got := CleanLine(strings.Repeat("あ", 50), 10); len([]rune(got)) != 10 {
		t.Fatalf("got %q", got)
	}
}

func TestCleanReportKeepsNewlines(t *testing.T) {
	if got := CleanReport("one\ntwo\x07\u2028\tx", 100); got != "one\ntwo\tx" {
		t.Fatalf("got %q", got)
	}
	got := CleanReport(strings.Repeat("あ", 30<<10), MaxReport)
	if len(got) > MaxReport || !utf8.ValidString(got) || len(got) < MaxReport-3 {
		t.Fatalf("len %d valid %v", len(got), utf8.ValidString(got))
	}
}

func TestFitReportSurvivesEscaping(t *testing.T) {
	s, ch := start(t)
	rep := strings.Repeat("<div>&\"x\"</div>\n", 24<<10/16)
	Send(s.Addr(), s.Token(), Msg{Tab: 1, Kind: KindAgentStop, AgentID: "a1", Report: FitReport(rep, MaxReport)}, time.Second)
	m := recv(t, ch)
	if m.Report == "" || !strings.HasPrefix(rep, m.Report) {
		t.Fatalf("report len %d", len(m.Report))
	}
	b, _ := json.Marshal(FitReport(rep, MaxReport))
	if len(b) > MaxReport {
		t.Fatalf("encoded %d bytes", len(b))
	}
}

func TestSessionKindCarriesOnlyIdentity(t *testing.T) {
	s, ch := start(t)
	m := Msg{Tab: 2, Kind: KindSession, Agent: "codex", Session: "s1", Reason: "x", Command: "/c", Args: "a", Usage: &Usage{}, AgentID: "id"}
	if err := Send(s.Addr(), s.Token(), m, time.Second); err != nil {
		t.Fatal(err)
	}
	got := recv(t, ch)
	if got.Kind != KindSession || got.Agent != "codex" || got.Session != "s1" {
		t.Fatalf("got %+v", got)
	}
	if got.Reason != "" || got.Command != "" || got.Args != "" || got.Usage != nil || got.AgentID != "" {
		t.Fatalf("extra fields kept: %+v", got)
	}
}

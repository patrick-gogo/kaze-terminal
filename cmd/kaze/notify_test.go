package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
)

func startServer(t *testing.T) (*notify.Server, chan notify.Msg) {
	t.Helper()
	got := make(chan notify.Msg, 4)
	srv, err := notify.Listen(func(m notify.Msg) { got <- m })
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv, got
}

func envOf(srv *notify.Server, tab string) func(string) string {
	m := map[string]string{"KAZE_NOTIFY": srv.Addr(), "KAZE_NOTIFY_TOKEN": srv.Token(), "KAZE_TAB_ID": tab}
	return func(k string) string { return m[k] }
}

func wait(t *testing.T, ch chan notify.Msg) notify.Msg {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(3 * time.Second):
		t.Fatal("no message received")
		return notify.Msg{}
	}
}

func TestNotifyNeedsSendsReasonFromHookJSON(t *testing.T) {
	srv, got := startServer(t)
	in := strings.NewReader(`{"hook_event_name":"Notification","message":"Claude needs your permission to use Bash"}`)
	if code := runNotify([]string{"needs"}, in, envOf(srv, "3")); code != 0 {
		t.Fatalf("exit %d", code)
	}
	m := wait(t, got)
	if m.Tab != 3 || m.Kind != notify.KindNeeds || m.Reason != "Claude needs your permission to use Bash" {
		t.Fatalf("got %+v", m)
	}
}

func TestNotifyDoneSends(t *testing.T) {
	srv, got := startServer(t)
	if code := runNotify([]string{"done"}, strings.NewReader(""), envOf(srv, "2")); code != 0 {
		t.Fatalf("exit %d", code)
	}
	m := wait(t, got)
	if m.Tab != 2 || m.Kind != notify.KindDone || m.Reason != "" {
		t.Fatalf("got %+v", m)
	}
}

func TestNotifyReasonTrimmedTo120Runes(t *testing.T) {
	srv, got := startServer(t)
	in := strings.NewReader(`{"message":"` + strings.Repeat("あ", 300) + `"}`)
	runNotify([]string{"needs"}, in, envOf(srv, "1"))
	if n := len([]rune(wait(t, got).Reason)); n != 120 {
		t.Fatalf("reason has %d runes, want 120", n)
	}
}

func TestNotifyWithoutServerExitsQuietly(t *testing.T) {
	srv, _ := startServer(t)
	dead := envOf(srv, "1")
	srv.Close() // the port is now dead
	for name, env := range map[string]func(string) string{
		"no env":    func(string) string { return "" },
		"dead port": dead,
	} {
		start := time.Now()
		if code := runNotify([]string{"done"}, strings.NewReader("{}"), env); code != 0 {
			t.Errorf("%s: exit %d", name, code)
		}
		if d := time.Since(start); d > 1500*time.Millisecond {
			t.Errorf("%s: took %v", name, d)
		}
	}
}

func TestNotifyBadKindExitsQuietly(t *testing.T) {
	srv, got := startServer(t)
	for _, args := range [][]string{nil, {"bogus"}} {
		if code := runNotify(args, strings.NewReader(""), envOf(srv, "1")); code != 0 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	select {
	case m := <-got:
		t.Fatalf("unexpected message %+v", m)
	case <-time.After(200 * time.Millisecond):
	}
}

// A hook whose stdin never closes must not hang kaze notify.
func TestNotifyStdinThatNeverEndsDoesNotBlock(t *testing.T) {
	srv, got := startServer(t)
	pr, pw := io.Pipe()
	defer pw.Close()
	start := time.Now()
	runNotify([]string{"done"}, pr, envOf(srv, "4"))
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Fatalf("took %v", d)
	}
	if m := wait(t, got); m.Tab != 4 {
		t.Fatalf("got %+v", m)
	}
}

func TestReadStdinCapsAt2MB(t *testing.T) {
	b := readStdinLimited(bytes.NewReader(make([]byte, 3<<20)), time.Second)
	if len(b) != 2<<20 {
		t.Fatalf("read %d bytes", len(b))
	}
}

func TestNotifyPanicStillExitsZero(t *testing.T) {
	boom := func(string) string { panic("boom") }
	if code := runNotify([]string{"done"}, strings.NewReader(""), boom); code != 0 {
		t.Fatalf("exit %d, want 0 (exit 2 would block Claude Code)", code)
	}
}

func TestNotifyRejectsNonPositiveTab(t *testing.T) {
	srv, got := startServer(t)
	for _, tab := range []string{"0", "-1"} {
		if code := runNotify([]string{"done"}, strings.NewReader(""), envOf(srv, tab)); code != 0 {
			t.Errorf("tab %s: exit %d", tab, code)
		}
	}
	select {
	case m := <-got:
		t.Fatalf("unexpected message %+v", m)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestNotifyGarbageJSONStillSendsWithoutReason(t *testing.T) {
	srv, got := startServer(t)
	runNotify([]string{"needs"}, strings.NewReader("not json {{{"), envOf(srv, "5"))
	m := wait(t, got)
	if m.Tab != 5 || m.Kind != notify.KindNeeds || m.Reason != "" {
		t.Fatalf("got %+v", m)
	}
}

func TestNotifyWritesNothingToStdout(t *testing.T) {
	srv, _ := startServer(t)
	dead := envOf(srv, "1")
	srv.Close()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = old }()
	runNotify([]string{"done"}, strings.NewReader("{}"), dead)
	runNotify([]string{"bogus"}, nil, dead)
	if fi, _ := f.Stat(); fi.Size() != 0 {
		t.Fatalf("wrote %d bytes to stdout", fi.Size())
	}
}

func TestNotifyNeedsByNotificationType(t *testing.T) {
	cases := []struct {
		name, json string
		send       bool
	}{
		{"permission prompt", `{"message":"m","notification_type":"permission_prompt"}`, true},
		{"elicitation dialog", `{"message":"m","notification_type":"elicitation_dialog"}`, true},
		{"missing type (older Claude Code)", `{"message":"m"}`, true},
		{"unknown type", `{"message":"m","notification_type":"something_new"}`, true},
		{"idle prompt", `{"message":"m","notification_type":"idle_prompt"}`, false},
		{"auth success", `{"message":"m","notification_type":"auth_success"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, got := startServer(t)
			if code := runNotify([]string{"needs"}, strings.NewReader(c.json), envOf(srv, "4")); code != 0 {
				t.Fatalf("exit %d", code)
			}
			if c.send {
				if m := wait(t, got); m.Kind != notify.KindNeeds || m.Tab != 4 {
					t.Fatalf("got %+v", m)
				}
				return
			}
			select {
			case m := <-got:
				t.Fatalf("sent %+v, want nothing", m)
			case <-time.After(300 * time.Millisecond):
			}
		})
	}
}

func TestPromptCommand(t *testing.T) {
	for _, c := range []struct{ in, cmd, args string }{
		{"/ship-ticket V3-3348", "/ship-ticket", "V3-3348"},
		{"  /v3-lap\nmore", "/v3-lap", ""},
		{"fix the bug", "", ""},
		{"/", "", ""},
		{"/a b c", "/a", "b c"},
	} {
		cmd, args := promptCommand(c.in)
		if cmd != c.cmd || args != c.args {
			t.Errorf("promptCommand(%q) = %q, %q; want %q, %q", c.in, cmd, args, c.cmd, c.args)
		}
	}
}

func TestNotifyPromptSendsCommand(t *testing.T) {
	srv, got := startServer(t)
	in := strings.NewReader(`{"prompt":"/review-mine --quality-only"}`)
	var out bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := runNotify([]string{"prompt"}, in, envOf(srv, "4"))
	os.Stdout = old
	w.Close()
	io.Copy(&out, r)
	if code != 0 || out.Len() != 0 {
		t.Fatalf("exit %d, printed %q", code, out.String())
	}
	m := wait(t, got)
	if m.Tab != 4 || m.Kind != notify.KindPrompt || m.Command != "/review-mine" || m.Args != "--quality-only" {
		t.Fatalf("got %+v", m)
	}
}

func TestNotifyPromptPlainTextStillSends(t *testing.T) {
	srv, got := startServer(t)
	runNotify([]string{"prompt"}, strings.NewReader(`{"prompt":"please continue"}`), envOf(srv, "1"))
	m := wait(t, got)
	if m.Kind != notify.KindPrompt || m.Command != "" || m.Args != "" {
		t.Fatalf("got %+v", m)
	}
}

func TestNotifyPromptHugeInputStaysBounded(t *testing.T) {
	srv, got := startServer(t)
	p := "/big " + strings.Repeat("a\x1bb\x07c", 1<<20/5)
	b, _ := json.Marshal(map[string]string{"prompt": p})
	start := time.Now()
	runNotify([]string{"prompt"}, bytes.NewReader(b), envOf(srv, "1"))
	if time.Since(start) > notifyTotal+500*time.Millisecond {
		t.Fatalf("took %v", time.Since(start))
	}
	m := wait(t, got)
	if m.Kind != notify.KindPrompt || len([]rune(m.Args)) > 120 {
		t.Fatalf("got kind %q, %d arg runes", m.Kind, len([]rune(m.Args)))
	}
	for _, r := range m.Command + m.Args {
		if r < 0x20 || r == 0x7f {
			t.Fatalf("control char %q survived", r)
		}
	}
}

func TestNotifyPromptOversizedCommandStillSends(t *testing.T) {
	srv, got := startServer(t)
	b, _ := json.Marshal(map[string]string{"prompt": "/" + strings.Repeat("a", 5000)})
	runNotify([]string{"prompt"}, bytes.NewReader(b), envOf(srv, "1"))
	m := wait(t, got)
	if m.Kind != notify.KindPrompt || m.Command != "" {
		t.Fatalf("got kind %q command len %d", m.Kind, len(m.Command))
	}
}

func TestNotifyPromptCleansArgsUnder64KB(t *testing.T) {
	srv, got := startServer(t)
	p := "/big " + strings.Repeat("a\x1bb\x07c", 1000)
	b, _ := json.Marshal(map[string]string{"prompt": p})
	runNotify([]string{"prompt"}, bytes.NewReader(b), envOf(srv, "1"))
	m := wait(t, got)
	n := len([]rune(m.Args))
	if m.Command != "/big" || n == 0 || n > 120 {
		t.Fatalf("command %q, %d arg runes", m.Command, n)
	}
	for _, r := range m.Args {
		if r < 0x20 || r == 0x7f {
			t.Fatalf("control char %q survived", r)
		}
	}
}

package ui

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/update"
	"github.com/patrick-gogo/kaze-terminal/internal/version"
)

// fakeReleases serves a release lookup for tag and, for the Windows x64 zip,
// a zip holding kaze.exe with body and a SHA256SUMS (sum overrides the real one).
type fakeReleases struct {
	srv     *httptest.Server
	lookups atomic.Int32
}

func newFakeReleases(t *testing.T, tag, body, sum string) *fakeReleases {
	t.Helper()
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	h := &zip.FileHeader{Name: "kaze.exe", Method: zip.Deflate}
	h.SetMode(0o755)
	w, _ := zw.CreateHeader(h)
	w.Write([]byte(body))
	zw.Close()
	zipData := zbuf.Bytes()
	if sum == "" {
		sum = fmt.Sprintf("%x", sha256.Sum256(zipData))
	}
	f := &fakeReleases{}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		f.lookups.Add(1)
		base := "http://" + r.Host
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":%q,"assets":[{"name":"Kaze-Windows-x64.zip","browser_download_url":%q},{"name":"SHA256SUMS","browser_download_url":%q}]}`,
			tag, base+"/page", base+"/zip", base+"/sums")
	})
	mux.HandleFunc("/zip", func(w http.ResponseWriter, r *http.Request) { w.Write(zipData) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "%s  Kaze-Windows-x64.zip\n", sum) })
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// updWindow is a test window whose updater talks to rel and installs over a
// temp kaze.exe (as on Windows), recording relaunches and quits.
func updWindow(t *testing.T, rel *fakeReleases) (win *window, exe string, relaunched, quit *int, opened *[]string) {
	t.Helper()
	win, _ = alertWindow(t, 2)
	win.opt.Config.CheckUpdates = true
	exe = filepath.Join(t.TempDir(), "kaze.exe")
	os.WriteFile(exe, []byte("old"), 0o755)
	relaunched, quit, opened = new(int), new(int), new([]string)
	win.upd = updater{
		url: rel.srv.URL + "/latest", client: rel.srv.Client(), exe: exe, goos: "windows", goarch: "amd64",
		relaunch: func(update.Target) error { *relaunched++; return nil },
		quit:     func() { *quit++ },
		open:     func(u string) error { *opened = append(*opened, u); return nil },
	}
	win.upd.findTarget()
	return
}

// waitUpd drains the updater's results until done holds, for at most 5 seconds.
func waitUpd(t *testing.T, win *window, done func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		win.drainUpdates()
		if done() {
			return
		}
	}
	t.Fatalf("updater state never settled: %+v", win.upd.view())
}

func aboutText(t *testing.T, win *window) string {
	t.Helper()
	win.set.texts = nil
	drawAbout(&page{gtx: pageContext(800, 700), c: win.ui, st: &win.set, col: win.ui.pageColors()}, image.Rect(218, 60, 772, 676), win)
	return strings.Join(win.set.texts, "\n")
}

func aboutButtons(win *window) []string {
	var ids []string
	for _, c := range win.set.controls {
		ids = append(ids, c.id)
	}
	return ids
}

func TestNoUpdateCheckWithoutAURL(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.opt.Config.CheckUpdates = true
	if _, ok := win.updTick(time.Now()); ok {
		t.Fatal("a window with no release URL scheduled a check")
	}
}

func TestUpdateCheckRunsAfterStartThenDaily(t *testing.T) {
	rel := newFakeReleases(t, "v99.0", "new", "")
	win, _, _, _, _ := updWindow(t, rel)
	t0 := time.Now()
	if at, ok := win.updTick(t0); !ok || at != t0.Add(updFirst) {
		t.Fatalf("first wake %v %v", at, ok)
	}
	if rel.lookups.Load() != 0 {
		t.Fatal("checked before the first wake")
	}
	win.updTick(t0.Add(updFirst))
	waitUpd(t, win, func() bool { return win.upd.checked })
	if !win.upd.newer || win.upd.rel.Version != "99.0" {
		t.Fatalf("after the check: %+v", win.upd.view())
	}
	if got := win.statusParts().Update; got != "Update 99.0" {
		t.Fatalf("status bar says %q", got)
	}
	win.updTick(t0.Add(updFirst + time.Hour))
	if at, _ := win.updTick(t0.Add(updFirst + time.Hour)); at != t0.Add(updFirst+updEvery) || rel.lookups.Load() != 1 {
		t.Fatalf("next check at %v after %d lookups", at, rel.lookups.Load())
	}
	win.updTick(t0.Add(updFirst + updEvery))
	waitUpd(t, win, func() bool { return rel.lookups.Load() == 2 && !win.upd.checking })
}

func TestTurningChecksOffMakesNoRequest(t *testing.T) {
	rel := newFakeReleases(t, "v99.0", "new", "")
	win, _, _, _, _ := updWindow(t, rel)
	win.opt.Config.CheckUpdates = false
	t0 := time.Now()
	for _, d := range []time.Duration{0, updFirst, updEvery} {
		if _, ok := win.updTick(t0.Add(d)); ok {
			t.Fatal("a wake was scheduled with checks off")
		}
	}
	aboutAct(win, "upd_check", 0)
	time.Sleep(50 * time.Millisecond)
	if n := rel.lookups.Load(); n != 0 {
		t.Fatalf("%d lookups with checks off", n)
	}
	if !strings.Contains(aboutText(t, win), "Update checks are off") {
		t.Fatal("About does not say checks are off")
	}
}

func TestTheSwitchSavesCheckUpdates(t *testing.T) {
	rel := newFakeReleases(t, "v99.0", "new", "")
	win, _, _, _, _ := updWindow(t, rel)
	win.cfgPath = filepath.Join(t.TempDir(), "config.toml")
	aboutAct(win, "check_updates", 0)
	data, _ := os.ReadFile(win.cfgPath)
	if win.opt.Config.CheckUpdates || !strings.Contains(string(data), "check_updates = false") {
		t.Fatalf("config %q, live %v", data, win.opt.Config.CheckUpdates)
	}
}

func TestSameVersionIsUpToDate(t *testing.T) {
	rel := newFakeReleases(t, "v"+version.Number, "same", "")
	win, _, _, _, _ := updWindow(t, rel)
	aboutAct(win, "upd_check", 0)
	waitUpd(t, win, func() bool { return win.upd.checked })
	if win.upd.newer || win.statusParts().Update != "" {
		t.Fatal("the same version counts as an update")
	}
	if !strings.Contains(aboutText(t, win), "Up to date") {
		t.Fatal("About does not say up to date")
	}
}

func TestStatusBarUpdateOpensAbout(t *testing.T) {
	rel := newFakeReleases(t, "v99.0", "new", "")
	win, _, _, _, _ := updWindow(t, rel)
	win.updClick()
	if !win.set.shown || win.sections()[win.set.section].title != "About" {
		t.Fatalf("settings shown %v at section %d", win.set.shown, win.set.section)
	}
}

func TestUpdateAsksFirstThenInstallsAndRestarts(t *testing.T) {
	rel := newFakeReleases(t, "v99.0", "new", "")
	win, exe, relaunched, quit, _ := updWindow(t, rel)
	aboutAct(win, "upd_check", 0)
	waitUpd(t, win, func() bool { return win.upd.checked })
	text := aboutText(t, win)
	if !strings.Contains(text, "Kaze 99.0 is available") || !slices.Contains(aboutButtons(win), "upd_install") {
		t.Fatalf("About: %q buttons %v", text, aboutButtons(win))
	}
	aboutAct(win, "upd_install", 0)
	if !strings.Contains(aboutText(t, win), "close your 2 sessions") || win.upd.busy != "" {
		t.Fatal("Update did not ask first")
	}
	aboutAct(win, "upd_confirm", 0)
	waitUpd(t, win, func() bool { return *quit == 1 })
	if b, _ := os.ReadFile(exe); string(b) != "new" || *relaunched != 1 {
		t.Fatalf("exe %q, relaunched %d", b, *relaunched)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".kaze-update-*")); len(left) != 0 {
		t.Fatalf("download folder left behind: %v", left)
	}
}

func TestUpdateRefusesABadChecksum(t *testing.T) {
	rel := newFakeReleases(t, "v99.0", "evil", strings.Repeat("0", 64))
	win, exe, relaunched, quit, _ := updWindow(t, rel)
	aboutAct(win, "upd_check", 0)
	waitUpd(t, win, func() bool { return win.upd.checked })
	aboutAct(win, "upd_install", 0)
	aboutAct(win, "upd_confirm", 0)
	waitUpd(t, win, func() bool { return win.upd.busy == "" && win.upd.err != "" })
	if b, _ := os.ReadFile(exe); string(b) != "old" || *relaunched != 0 || *quit != 0 {
		t.Fatalf("exe %q relaunched %d quit %d", b, *relaunched, *quit)
	}
	if !strings.Contains(aboutText(t, win), "checksum") {
		t.Fatalf("the error is not shown: %q", win.upd.err)
	}
}

func TestCancelLeavesKazeAlone(t *testing.T) {
	rel := newFakeReleases(t, "v99.0", "new", "")
	win, exe, _, quit, _ := updWindow(t, rel)
	aboutAct(win, "upd_check", 0)
	waitUpd(t, win, func() bool { return win.upd.checked })
	aboutAct(win, "upd_install", 0)
	aboutAct(win, "upd_cancel", 0)
	time.Sleep(50 * time.Millisecond)
	win.drainUpdates()
	if b, _ := os.ReadFile(exe); string(b) != "old" || *quit != 0 || win.upd.confirm {
		t.Fatalf("after cancel: exe %q quit %d confirm %v", b, *quit, win.upd.confirm)
	}
}

func TestASourceBuildGetsADownloadButton(t *testing.T) {
	rel := newFakeReleases(t, "v99.0", "new", "")
	win, exe, _, _, opened := updWindow(t, rel)
	os.Mkdir(filepath.Join(filepath.Dir(exe), ".git"), 0o755)
	win.upd.findTarget()
	aboutAct(win, "upd_check", 0)
	waitUpd(t, win, func() bool { return win.upd.checked })
	aboutText(t, win)
	ids := aboutButtons(win)
	if slices.Contains(ids, "upd_install") || !slices.Contains(ids, "upd_page") {
		t.Fatalf("buttons %v", ids)
	}
	aboutAct(win, "upd_page", 0)
	if len(*opened) != 1 || !strings.HasSuffix((*opened)[0], "/page") {
		t.Fatalf("opened %v", *opened)
	}
}

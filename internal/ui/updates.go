package ui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"gioui.org/io/system"

	"github.com/patrick-gogo/kaze-terminal/internal/update"
	"github.com/patrick-gogo/kaze-terminal/internal/version"
)

// Update timing and limits (spec 2026-10-31-kaze-updates-design).
const (
	updFirst    = 10 * time.Second // the first check after kaze starts
	updEvery    = 24 * time.Hour
	updTimeout  = 10 * time.Second // the release lookup
	updDownload = 5 * time.Minute
	updMaxZip   = 200 << 20
	updMaxSums  = 64 << 10
	updProgress = 250 * time.Millisecond // at most one progress frame this often
)

// updater is kaze's update check and in-place install. Run fills in the real
// URL, client and hooks; a test window leaves url empty, so it never reaches
// the network unless a test points it at a fake server.
type updater struct {
	url          string
	client       *http.Client
	exe          string // os.Executable
	goos, goarch string
	relaunch     func(update.Target) error
	quit         func()
	open         func(string) error

	target    update.Target
	targetErr error // why this kaze can't be updated in place

	next     time.Time // the next scheduled check
	checking bool
	checked  bool // a check has finished, with or without an error
	rel      update.Release
	newer    bool   // rel is newer than this kaze
	err      string // the last check's or install's failure
	confirm  bool   // the About page is asking before it restarts
	busy     string // "Downloading 40%" while an install runs
	pct      int    // the download's progress, 0..100

	checkedAt time.Time // when the last check finished
}

// updInbox carries the work goroutines' results to the UI goroutine.
type updInbox struct {
	mu sync.Mutex
	fs []func(*window)
}

// updView is the updater's state without its lock and hooks, for test failures.
type updView struct {
	Checking, Checked, Newer, Confirm bool
	Version, Err, Busy                string
}

func (u *updater) view() updView {
	return updView{u.checking, u.checked, u.newer, u.confirm, u.rel.Version, u.err, u.busy}
}

// findTarget works out once whether this kaze can replace itself.
func (u *updater) findTarget() {
	u.target, u.targetErr = update.FindTarget(u.exe, u.goos)
}

// initUpdates sets up the real updater and removes what a past update left.
func (win *window) initUpdates() {
	exe, _ := os.Executable()
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	win.upd = updater{
		url: update.LatestURL, client: &http.Client{}, exe: exe, goos: runtime.GOOS, goarch: runtime.GOARCH,
		relaunch: update.RelaunchWaiting,
		quit:     win.closeWindow,
		open:     openURL,
	}
	win.upd.findTarget()
	if win.upd.targetErr == nil {
		go update.Cleanup(win.upd.target)
	}
}

// post hands f to the UI goroutine and wakes it.
func (win *window) post(f func(*window)) {
	win.updIn.mu.Lock()
	win.updIn.fs = append(win.updIn.fs, f)
	win.updIn.mu.Unlock()
	win.w.Invalidate()
}

// drainUpdates applies the work goroutines' results.
func (win *window) drainUpdates() {
	win.updIn.mu.Lock()
	fs := win.updIn.fs
	win.updIn.fs = nil
	win.updIn.mu.Unlock()
	for _, f := range fs {
		f(win)
	}
}

// updTick starts a due check and returns when the next one is due; ok is
// false when checks are off, so an idle kaze schedules no wake for them.
func (win *window) updTick(now time.Time) (time.Time, bool) {
	u := &win.upd
	if u.url == "" || !win.opt.Config.CheckUpdates {
		return time.Time{}, false
	}
	if u.next.IsZero() {
		u.next = now.Add(updFirst)
	}
	if !now.Before(u.next) {
		u.next = now.Add(updEvery)
		win.checkUpdates()
	}
	return u.next, true
}

// checkUpdates asks for the latest release in the background.
func (win *window) checkUpdates() {
	u := &win.upd
	if u.url == "" || !win.opt.Config.CheckUpdates || u.checking || u.busy != "" {
		return
	}
	u.checking = true
	url, client := u.url, u.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updTimeout)
		defer cancel()
		rel, err := update.Latest(ctx, client, url)
		win.post(func(win *window) {
			u := &win.upd
			u.checking, u.checked, u.checkedAt = false, true, time.Now()
			if err != nil {
				u.err = "Couldn't check for updates: " + err.Error()
				return
			}
			u.err, u.rel = "", rel
			u.newer = update.Compare(rel.Version, version.Number) > 0
		})
	}()
}

// closeWindow closes kaze, as its close button does.
func (win *window) closeWindow() { win.w.Perform(system.ActionClose) }

// updLabel is the status bar's update notice, "" when there is none.
func (win *window) updLabel() string {
	if win.opt.Config.CheckUpdates && win.upd.newer {
		return "Update " + win.upd.rel.Version
	}
	return ""
}

// updFoot is the update state under the version in the settings menu.
func (win *window) updFoot() string {
	u := &win.upd
	switch {
	case !win.opt.Config.CheckUpdates || u.url == "" && !u.checked:
		return ""
	case u.newer:
		return u.rel.Version + " available"
	case u.checked && u.err == "":
		return "Up to date"
	}
	return ""
}

// updClick shows the Updates page.
func (win *window) updClick() {
	win.openSettings()
	if i := slices.IndexFunc(win.sections(), func(s settingsSection) bool { return s.title == "Updates" }); i >= 0 {
		win.selectSection(i)
	}
}

// updInstallable reports whether the update can replace this kaze, and why not.
func (win *window) updInstallable() (bool, string) {
	u := &win.upd
	if u.targetErr != nil {
		return false, u.targetErr.Error()
	}
	asset := update.AssetName(u.goos, u.goarch)
	if asset == "" || u.rel.Assets[asset] == "" || u.rel.Assets[update.SumsName] == "" {
		return false, "this release has no download for this computer"
	}
	return true, ""
}

// installUpdate downloads, checks, unzips and swaps in the new kaze, then
// starts it and quits. Every step runs off the UI goroutine.
func (win *window) installUpdate() {
	u := &win.upd
	if ok, _ := win.updInstallable(); !ok || u.busy != "" {
		return
	}
	u.confirm, u.err, u.busy, u.pct = false, "", "Downloading…", 0
	asset := update.AssetName(u.goos, u.goarch)
	zipURL, sumsURL := u.rel.Assets[asset], u.rel.Assets[update.SumsName]
	client, target, relaunch := u.client, u.target, u.relaunch
	go func() {
		err := func() error {
			// Next to the install, so the swap is a rename on one volume.
			tmp, err := os.MkdirTemp(filepath.Dir(target.Path), ".kaze-update-")
			if err != nil {
				return fmt.Errorf("can't write next to kaze: %w", err)
			}
			defer os.RemoveAll(tmp)
			ctx, cancel := context.WithTimeout(context.Background(), updDownload)
			defer cancel()
			zipFile, sumsFile := filepath.Join(tmp, asset), filepath.Join(tmp, update.SumsName)
			if err := update.Download(ctx, client, sumsURL, sumsFile, updMaxSums, nil); err != nil {
				return err
			}
			var last time.Time
			progress := func(done, total int64) {
				if total <= 0 || time.Since(last) < updProgress {
					return
				}
				last = time.Now()
				pct := int(done * 100 / total)
				win.post(func(win *window) { win.upd.busy, win.upd.pct = fmt.Sprintf("Downloading %d%%", pct), pct })
			}
			if err := update.Download(ctx, client, zipURL, zipFile, updMaxZip, progress); err != nil {
				return err
			}
			sums, err := os.ReadFile(sumsFile)
			if err != nil {
				return err
			}
			if err := update.CheckSum(sums, asset, zipFile); err != nil {
				return err
			}
			win.post(func(win *window) { win.upd.busy = "Installing…" })
			unzipped := filepath.Join(tmp, "new")
			if err := update.Unzip(zipFile, unzipped, updMaxZip*2); err != nil {
				return err
			}
			if err := update.Install(target, unzipped); err != nil {
				return err
			}
			return nil
		}()
		win.post(func(win *window) {
			u := &win.upd
			if err != nil {
				u.busy, u.err = "", "Update failed: "+err.Error()
				return
			}
			win.restartForUpdate(target, relaunch)
		})
	}()
}

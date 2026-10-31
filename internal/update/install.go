package update

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// bundleID marks a Kaze.app built by tools/package-macos.sh.
const bundleID = "com.patrickgogo.kaze"

// Target is the installed kaze an update replaces.
type Target struct {
	Path string // the Kaze.app bundle (macOS) or kaze.exe (Windows)
	Old  string // where the running copy moves during the swap; removed on the next start
	App  bool   // Path is a macOS bundle
}

// rename and start are os.Rename and Cmd.Start; tests replace them.
var (
	rename = os.Rename
	start  = func(c *exec.Cmd) error { return c.Start() }
)

// FindTarget is the installed kaze that exe (os.Executable) belongs to, if it
// can be updated in place: a Kaze.app bundle on macOS, kaze.exe on Windows,
// and in both cases not inside a git checkout, so a source build is never
// replaced.
func FindTarget(exe, goos string) (Target, error) {
	var t Target
	switch goos {
	case "darwin":
		macos := filepath.Dir(exe)
		contents := filepath.Dir(macos)
		app := filepath.Dir(contents)
		if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || filepath.Ext(app) != ".app" {
			return t, errors.New("kaze is not running from an app bundle")
		}
		plist, err := os.ReadFile(filepath.Join(contents, "Info.plist"))
		if err != nil || !bytes.Contains(plist, []byte("<string>"+bundleID+"</string>")) {
			return t, errors.New("this is not a Kaze release app")
		}
		t = Target{Path: app, Old: app + ".old", App: true}
	case "windows":
		if !strings.EqualFold(filepath.Base(exe), "kaze.exe") {
			return t, errors.New("kaze is not running as kaze.exe")
		}
		t = Target{Path: exe, Old: filepath.Join(filepath.Dir(exe), "kaze-old.exe")}
	default:
		return t, fmt.Errorf("updates are not available on %s", goos)
	}
	// A source build sits in its checkout (kaze.exe) or in its dist folder
	// (dist/Kaze.app), so .git is at most two levels up. Higher up it is some
	// other repo, like a dotfiles home folder.
	dir := filepath.Dir(t.Path)
	for _, d := range []string{dir, filepath.Dir(dir)} {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return Target{}, errors.New("kaze was built from source")
		}
	}
	return t, nil
}

// Install swaps the new kaze in unzipped (from the release zip, on the same
// volume as t.Path) into place. The running copy moves to t.Old; if the new
// one cannot move into place, the old one is put back.
func Install(t Target, unzipped string) error {
	src := filepath.Join(unzipped, "kaze.exe")
	check := src
	if t.App {
		src = filepath.Join(unzipped, "Kaze.app")
		check = filepath.Join(src, "Contents", "MacOS", "kaze")
	}
	if fi, err := os.Stat(check); err != nil || !fi.Mode().IsRegular() {
		return errors.New("the download has no kaze in it")
	}
	if err := os.RemoveAll(t.Old); err != nil {
		return err
	}
	if err := rename(t.Path, t.Old); err != nil {
		return fmt.Errorf("moving the old kaze aside: %w", err)
	}
	if err := rename(src, t.Path); err != nil {
		if back := rename(t.Old, t.Path); back != nil {
			return fmt.Errorf("installing failed (%v) and the old kaze is at %s (%v)", err, t.Old, back)
		}
		return fmt.Errorf("installing the new kaze: %w", err)
	}
	return nil
}

// Cleanup removes the copy an earlier update moved aside. It only ever
// removes a path named like one.
func Cleanup(t Target) {
	base := filepath.Base(t.Old)
	if base == "kaze-old.exe" || strings.HasSuffix(base, ".app.old") {
		os.RemoveAll(t.Old)
	}
}

// Relaunch starts the installed kaze as a new process; the caller then quits.
func Relaunch(t Target) error {
	if t.App {
		return start(exec.Command("open", "-n", t.Path))
	}
	return start(exec.Command(t.Path))
}

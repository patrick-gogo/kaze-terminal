package update

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

const kazePlist = `<plist><dict><key>CFBundleIdentifier</key><string>com.patrickgogo.kaze</string></dict></plist>`

// fakeApp makes dir/Kaze.app with an executable that says what it is.
func fakeApp(t *testing.T, dir, id, body string) string {
	t.Helper()
	app := filepath.Join(dir, "Kaze.app")
	os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(`<plist><dict><key>CFBundleIdentifier</key><string>`+id+`</string></dict></plist>`), 0o644)
	os.WriteFile(filepath.Join(app, "Contents", "MacOS", "kaze"), []byte(body), 0o755)
	return app
}

func TestFindTargetAcceptsAReleaseApp(t *testing.T) {
	apps := t.TempDir()
	app := fakeApp(t, apps, "com.patrickgogo.kaze", "old")
	tg, err := FindTarget(filepath.Join(app, "Contents", "MacOS", "kaze"), "darwin")
	if err != nil {
		t.Fatal(err)
	}
	if tg.Path != app || tg.Old != app+".old" || !tg.App {
		t.Fatalf("target %+v", tg)
	}
}

func TestFindTargetRefusesSourceBuilds(t *testing.T) {
	repo := t.TempDir()
	os.Mkdir(filepath.Join(repo, ".git"), 0o755)
	dist := filepath.Join(repo, "dist")
	os.Mkdir(dist, 0o755)
	app := fakeApp(t, dist, "com.patrickgogo.kaze", "dev")
	if _, err := FindTarget(filepath.Join(app, "Contents", "MacOS", "kaze"), "darwin"); err == nil {
		t.Error("an app inside a git checkout was accepted")
	}
	other := fakeApp(t, t.TempDir(), "com.example.other", "x")
	if _, err := FindTarget(filepath.Join(other, "Contents", "MacOS", "kaze"), "darwin"); err == nil {
		t.Error("another bundle id was accepted")
	}
	if _, err := FindTarget(filepath.Join(t.TempDir(), "kaze"), "darwin"); err == nil {
		t.Error("a bare binary was accepted")
	}
	exe := filepath.Join(repo, "kaze.exe")
	os.WriteFile(exe, nil, 0o755)
	if _, err := FindTarget(exe, "windows"); err == nil {
		t.Error("an exe inside a git checkout was accepted")
	}
}

func TestFindTargetAcceptsAWindowsExe(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "kaze.exe")
	os.WriteFile(exe, nil, 0o755)
	tg, err := FindTarget(exe, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if tg.Path != exe || tg.Old != filepath.Join(dir, "kaze-old.exe") || tg.App {
		t.Fatalf("target %+v", tg)
	}
	other := filepath.Join(dir, "kaze-dev.exe")
	if _, err := FindTarget(other, "windows"); err == nil {
		t.Error("an exe with another name was accepted")
	}
}

func exeBody(t *testing.T, app string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(app, "Contents", "MacOS", "kaze"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallSwapsTheAppAndKeepsTheOld(t *testing.T) {
	apps := t.TempDir()
	app := fakeApp(t, apps, "com.patrickgogo.kaze", "old")
	unz := filepath.Join(apps, ".kaze-update")
	fakeApp(t, unz, "com.patrickgogo.kaze", "new")
	tg := Target{Path: app, Old: app + ".old", App: true}
	if err := Install(tg, unz); err != nil {
		t.Fatal(err)
	}
	if exeBody(t, app) != "new" || exeBody(t, tg.Old) != "old" {
		t.Fatalf("after install: %q, old %q", exeBody(t, app), exeBody(t, tg.Old))
	}
	Cleanup(tg)
	if _, err := os.Stat(tg.Old); !os.IsNotExist(err) {
		t.Fatalf("old app left: %v", err)
	}
}

func TestInstallPutsTheOldAppBackWhenTheMoveFails(t *testing.T) {
	apps := t.TempDir()
	app := fakeApp(t, apps, "com.patrickgogo.kaze", "old")
	unz := filepath.Join(apps, ".kaze-update")
	fakeApp(t, unz, "com.patrickgogo.kaze", "new")
	calls := 0
	rename = func(from, to string) error {
		calls++
		if calls == 2 {
			return errors.New("disk said no")
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { rename = os.Rename })
	tg := Target{Path: app, Old: app + ".old", App: true}
	if err := Install(tg, unz); err == nil {
		t.Fatal("a failed move reported success")
	}
	if exeBody(t, app) != "old" {
		t.Fatalf("app is %q after a failed install", exeBody(t, app))
	}
}

func TestInstallNeedsTheNewApp(t *testing.T) {
	apps := t.TempDir()
	app := fakeApp(t, apps, "com.patrickgogo.kaze", "old")
	tg := Target{Path: app, Old: app + ".old", App: true}
	if err := Install(tg, t.TempDir()); err == nil {
		t.Fatal("an empty download installed")
	}
	if exeBody(t, app) != "old" {
		t.Fatal("the app changed")
	}
}

func TestInstallSwapsAWindowsExe(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "kaze.exe")
	os.WriteFile(exe, []byte("old"), 0o755)
	unz := filepath.Join(dir, ".kaze-update")
	os.Mkdir(unz, 0o755)
	os.WriteFile(filepath.Join(unz, "kaze.exe"), []byte("new"), 0o755)
	tg := Target{Path: exe, Old: filepath.Join(dir, "kaze-old.exe")}
	if err := Install(tg, unz); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Fatalf("exe is %q", b)
	}
	if b, _ := os.ReadFile(tg.Old); string(b) != "old" {
		t.Fatalf("old exe is %q", b)
	}
}

func TestCleanupOnlyRemovesTheOldCopy(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep")
	os.WriteFile(keep, []byte("x"), 0o644)
	Cleanup(Target{Path: filepath.Join(dir, "kaze.exe"), Old: keep})
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("Cleanup removed a file that is not an old kaze")
	}
}

func TestRelaunchStartsTheNewKaze(t *testing.T) {
	var got []string
	start = func(c *exec.Cmd) error { got = c.Args; return nil }
	t.Cleanup(func() { start = func(c *exec.Cmd) error { return c.Start() } })
	Relaunch(Target{Path: "/Applications/Kaze.app", App: true})
	if !slices.Equal(got, []string{"open", "-n", "/Applications/Kaze.app"}) {
		t.Fatalf("mac relaunch %v", got)
	}
	Relaunch(Target{Path: `C:\kaze\kaze.exe`})
	if !slices.Equal(got, []string{`C:\kaze\kaze.exe`}) {
		t.Fatalf("windows relaunch %v", got)
	}
}

func TestFindTargetIgnoresAGitFolderHigherUp(t *testing.T) {
	home := t.TempDir()
	os.Mkdir(filepath.Join(home, ".git"), 0o755) // a dotfiles repo in the home folder
	apps := filepath.Join(home, "Applications", "Tools")
	os.MkdirAll(apps, 0o755)
	app := fakeApp(t, apps, "com.patrickgogo.kaze", "x")
	if _, err := FindTarget(filepath.Join(app, "Contents", "MacOS", "kaze"), "darwin"); err != nil {
		t.Fatalf("a release app under a dotfiles home was refused: %v", err)
	}
}

func TestRelaunchWaitsForOldSessionOwner(t *testing.T) {
	var got []string
	start = func(c *exec.Cmd) error { got = c.Args; return nil }
	t.Cleanup(func() { start = func(c *exec.Cmd) error { return c.Start() } })
	RelaunchWaiting(Target{Path: "/Applications/Kaze.app", App: true})
	if !slices.Equal(got, []string{"open", "-n", "/Applications/Kaze.app", "--args", "--restore-wait"}) {
		t.Fatal(got)
	}
	RelaunchWaiting(Target{Path: `C:\kaze\kaze.exe`})
	if !slices.Equal(got, []string{`C:\kaze\kaze.exe`, "--restore-wait"}) {
		t.Fatal(got)
	}
}

package gitstat

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestShortstatParses(t *testing.T) {
	for _, c := range []struct {
		in       string
		add, del int
	}{
		{" 3 files changed, 212 insertions(+), 14 deletions(-)\n", 212, 14},
		{" 1 file changed, 5 insertions(+)\n", 5, 0},
		{" 2 files changed, 7 deletions(-)\n", 0, 7},
		{" 1 file changed, 1 insertion(+), 1 deletion(-)\n", 1, 1},
		{"", 0, 0},
		{"\n", 0, 0},
	} {
		a, d := parse(c.in)
		if a != c.add || d != c.del {
			t.Errorf("parse(%q) = %d, %d; want %d, %d", c.in, a, d, c.add, c.del)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestShortstatInARealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.name", "t")
	git(t, dir, "config", "user.email", "t@example.com")
	git(t, dir, "config", "commit.gpgsign", "false")
	write(t, dir, "a.txt", "one\ntwo\nthree\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "init")
	write(t, dir, "a.txt", "one\nTWO\nthree\nfour\nfive\n")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	add, del, err := Shortstat(ctx, dir)
	if err != nil || add != 3 || del != 1 {
		t.Fatalf("got %d, %d, %v; want 3, 1, nil", add, del, err)
	}
}

// Counting must never rewrite the index: that takes index.lock, which would
// fail the git commands Claude is running in the same repo.
func TestShortstatLeavesTheIndexAlone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.name", "t")
	git(t, dir, "config", "user.email", "t@example.com")
	git(t, dir, "config", "commit.gpgsign", "false")
	write(t, dir, "a.txt", "one\ntwo\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "init")
	// Only stat info changes: the content is the same, the times are not.
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "a.txt"), past, past); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dir, ".git", "index")
	older := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(index, older, older); err != nil {
		t.Fatal(err)
	}
	before := mtime(t, index)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if add, del, err := Shortstat(ctx, dir); err != nil || add != 0 || del != 0 {
		t.Fatalf("got %d, %d, %v; want 0, 0, nil", add, del, err)
	}
	if after := mtime(t, index); !after.Equal(before) {
		t.Fatalf("the index was rewritten: mtime %v, was %v", after, before)
	}
	write(t, dir, "a.txt", "one\nTWO\nthree\n")
	if add, del, err := Shortstat(ctx, dir); err != nil || add != 2 || del != 1 {
		t.Fatalf("after an edit got %d, %d, %v; want 2, 1, nil", add, del, err)
	}
}

func mtime(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.ModTime()
}

func TestShortstatNotARepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if _, _, err := Shortstat(ctx, t.TempDir()); err == nil {
		t.Fatal("a plain folder gave no error")
	}
	if time.Since(start) > 6*time.Second {
		t.Fatal("took longer than the timeout")
	}
}

// TestMain doubles as a helper process: a parent that starts a child holding
// the inherited stdout, like git.exe's launcher, then waits.
func TestMain(m *testing.M) {
	switch os.Getenv("GITSTAT_HELPER") {
	case "child":
		// Outlives the timeout plus WaitDelay below, then exits so go can clean up.
		time.Sleep(3 * time.Second)
		return
	case "parent":
		c := exec.Command(os.Args[0])
		c.Env = append(os.Environ(), "GITSTAT_HELPER=child")
		c.Stdout = os.Stdout
		_ = c.Start()
		time.Sleep(3 * time.Second)
		return
	}
	os.Exit(m.Run())
}

func TestReadIsBoundedWhenAChildKeepsThePipe(t *testing.T) {
	t.Setenv("GITSTAT_HELPER", "parent")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := shortstat(ctx, os.Args[0]); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a killed command gave no error")
		}
	case <-time.After(2 * time.Second): // under the helper's 3s, so a pipe wait shows
		t.Fatal("the read outlived the timeout")
	}
}

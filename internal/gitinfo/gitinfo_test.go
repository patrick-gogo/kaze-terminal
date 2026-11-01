package gitinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Canonical keys may expand Windows 8.3 aliases in the test runner's temp path.
// Compare directory identity here; path-casing behavior has its own regression.
func sameDirectory(a, b string) bool {
	fa, ea := os.Stat(a)
	fb, eb := os.Stat(b)
	return ea == nil && eb == nil && os.SameFile(fa, fb)
}

func TestBranchFromHead(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/feature/x\n")
	if got := Branch(root); got != "feature/x" {
		t.Fatalf("got %q", got)
	}
}

func TestBranchFromSubdir(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Branch(sub); got != "main" {
		t.Fatalf("got %q", got)
	}
}

func TestDetachedHead(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), strings.Repeat("ab12cd3", 6)[:40]+"\n")
	if got := Branch(root); got != "ab12cd3" {
		t.Fatalf("got %q", got)
	}
}

func TestWorktreeGitFile(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "real", ".git", "worktrees", "w", "HEAD"), "ref: refs/heads/wt-branch\n")
	wt := filepath.Join(root, "wt")
	write(t, filepath.Join(wt, ".git"), "gitdir: ../real/.git/worktrees/w\n")
	if got := Branch(wt); got != "wt-branch" {
		t.Fatalf("relative: got %q", got)
	}
	// Absolute gitdir, forward slashes.
	abs := filepath.ToSlash(filepath.Join(root, "real", ".git", "worktrees", "w"))
	write(t, filepath.Join(wt, ".git"), "gitdir: "+abs+"\n")
	if got := Branch(wt); got != "wt-branch" {
		t.Fatalf("absolute: got %q", got)
	}
}

func TestNotARepo(t *testing.T) {
	if got := Branch(t.TempDir()); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestGitdirLoopStops(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git"), "gitdir: .git\n")
	done := make(chan string, 1)
	go func() { done <- Branch(root) }()
	select {
	case got := <-done:
		if got != "" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("did not stop")
	}
}

func TestDepthLimit(t *testing.T) {
	root := t.TempDir()
	dir := root
	for i := 0; i < 65; i++ {
		dir = filepath.Join(dir, "d")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skip("cannot create deep dir:", err)
	}
	// A repo above the 64-level limit must not be found.
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/far\n")
	start := time.Now()
	if got := Branch(dir); got != "" {
		t.Fatalf("got %q", got)
	}
	if time.Since(start) > time.Second {
		t.Fatal("too slow")
	}
}

func TestUnreadableHead(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "HEAD"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Branch(root); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestSymlinkedGitDir(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "store", "HEAD"), "ref: refs/heads/linked\n")
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "store"), filepath.Join(repo, ".git")); err != nil {
		t.Skip("symlinks not permitted:", err)
	}
	if got := Branch(repo); got != "linked" {
		t.Fatalf("got %q", got)
	}
}

func TestBranchStripsControlChars(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/a\x1b[31mb\x07c\x7f\n")
	if got := Branch(root); got != "a[31mbc" {
		t.Fatalf("got %q", got)
	}
}

func TestRepoOfAPlainCheckout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	key, name := Repo(root)
	if !sameDirectory(key, filepath.Join(root, ".git")) || name != "proj" {
		t.Fatalf("got %q, %q", key, name)
	}
}

func TestRepoOfASubfolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	sub := filepath.Join(root, "sub")
	write(t, filepath.Join(sub, "f.txt"), "x")
	wantK, wantN := Repo(root)
	key, name := Repo(sub)
	if key != wantK || name != wantN || name != "proj" {
		t.Fatalf("got %q, %q", key, name)
	}
}

func TestRepoOfAWorktreeIsTheMainRepo(t *testing.T) {
	base := t.TempDir()
	main := filepath.Join(base, "main")
	wtGit := filepath.Join(main, ".git", "worktrees", "wt")
	write(t, filepath.Join(main, ".git", "HEAD"), "ref: refs/heads/main\n")
	write(t, filepath.Join(wtGit, "HEAD"), "ref: refs/heads/wt\n")
	write(t, filepath.Join(wtGit, "commondir"), "../..\n")
	wt := filepath.Join(base, "wt")
	write(t, filepath.Join(wt, ".git"), "gitdir: "+filepath.ToSlash(wtGit)+"\n")
	key, name := Repo(wt)
	if !sameDirectory(key, filepath.Join(main, ".git")) || name != "main" {
		t.Fatalf("got %q, %q", key, name)
	}
}

func TestRepoOfASubmodule(t *testing.T) {
	base := t.TempDir()
	modGit := filepath.Join(base, "super", ".git", "modules", "sub")
	write(t, filepath.Join(modGit, "HEAD"), "ref: refs/heads/main\n")
	sub := filepath.Join(base, "super", "sub")
	write(t, filepath.Join(sub, ".git"), "gitdir: ../.git/modules/sub\n")
	key, name := Repo(sub)
	if !sameDirectory(key, modGit) || name != "sub" {
		t.Fatalf("got %q, %q", key, name)
	}
}

func TestRepoOutsideARepo(t *testing.T) {
	// Not in a repo only if no ancestor of the temp dir has .git; plant the
	// boundary with an unreadable .git file so the walk stops inside it.
	root := t.TempDir()
	write(t, filepath.Join(root, ".git"), "not a gitdir line\n")
	if key, name := Repo(root); key != "" || name != "" {
		t.Fatalf("got %q, %q", key, name)
	}
	if key, name := Repo(""); key != "" || name != "" {
		t.Fatalf("empty dir: got %q, %q", key, name)
	}
}

func TestRepoSelfPointingGitFile(t *testing.T) {
	root := t.TempDir()
	git := filepath.Join(root, ".git")
	write(t, git, "gitdir: "+filepath.ToSlash(git)+"\n")
	key, name := Repo(root)
	if key != "" || name != "" {
		t.Fatalf("got %q, %q", key, name)
	}
}

func TestTopOfAWorktree(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "real", ".git", "HEAD"), "ref: refs/heads/main\n")
	write(t, filepath.Join(root, "real", ".git", "worktrees", "w", "HEAD"), "ref: refs/heads/wt\n")
	wt := filepath.Join(root, "wt")
	write(t, filepath.Join(wt, ".git"), "gitdir: ../real/.git/worktrees/w\n")
	sub := filepath.Join(wt, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Top(sub); got != wt {
		t.Fatalf("worktree top %q, want %q", got, wt)
	}
	if got := Top(filepath.Join(root, "real")); got != filepath.Join(root, "real") {
		t.Fatalf("main top %q", got)
	}
}

func TestTopOutsideARepo(t *testing.T) {
	if got := Top(t.TempDir()); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := Top(""); got != "" {
		t.Fatalf("empty dir: got %q", got)
	}
}

// A cwd typed with other casing on a case-insensitive disk (macOS, Windows) is
// the same repo, so it must give the same key and name (issue #65).
func TestRepoIgnoresPathCasing(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "Docs", "Proj")
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	other := filepath.Join(base, "docs", "proj")
	if _, err := os.Stat(other); err != nil {
		t.Skip("case-sensitive file system")
	}
	wantK, wantN := Repo(root)
	key, name := Repo(other)
	if key != wantK || name != wantN {
		t.Fatalf("got %q, %q; want %q, %q", key, name, wantK, wantN)
	}
	var c Cache
	first, second := c.Lookup(root), c.Lookup(other)
	if first.Repo != second.Repo || first.Name != second.Name {
		t.Fatalf("cached path casing: %+v versus %+v", first, second)
	}
}

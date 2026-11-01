package gitinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCachedLookupRefreshesHeadWithoutDirectoryScan(t *testing.T) {
	root := t.TempDir()
	head := filepath.Join(root, ".git", "HEAD")
	write(t, head, "ref: refs/heads/main\n")
	var c Cache
	calls := 0
	c.names = func(dir, part string) (string, bool) {
		// Shared temporary ancestors may change while other packages run.
		// Assert reuse of this checkout's own stable components.
		if part == ".git" || strings.EqualFold(part, filepath.Base(root)) {
			calls++
		}
		return storedName(dir, part)
	}
	first := c.Lookup(root)
	if first.Branch != "main" || first.Repo == "" {
		t.Fatalf("lookup %+v", first)
	}
	n := calls
	write(t, head, "ref: refs/heads/next\n")
	second := c.Lookup(root)
	if second.Branch != "next" || first.Repo != second.Repo || first.Name != second.Name {
		t.Fatalf("lookup %+v", second)
	}
	if calls != n {
		t.Fatalf("HEAD-only change rescanned names: %d -> %d", n, calls)
	}
	other := t.TempDir()
	if got := c.Lookup(other); got.Repo != "" {
		t.Fatalf("outside repo: %+v", got)
	}
	write(t, filepath.Join(other, ".git", "HEAD"), "ref: refs/heads/new\n")
	if got := c.Lookup(other); got.Branch != "new" || got.Repo == "" {
		t.Fatalf("git init not detected: %+v", got)
	}
}

func TestCachedLookupWorktreeAndDirectoryRename(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "Main")
	write(t, filepath.Join(main, ".git", "HEAD"), "ref: refs/heads/main\n")
	wt := filepath.Join(root, "Worktree")
	write(t, filepath.Join(wt, ".git"), "gitdir: ../Main/.git/worktrees/w\n")
	write(t, filepath.Join(main, ".git", "worktrees", "w", "HEAD"), "ref: refs/heads/work\n")
	write(t, filepath.Join(main, ".git", "worktrees", "w", "commondir"), "../..\n")
	var c Cache
	a, b := c.Lookup(main), c.Lookup(wt)
	if a.Repo != b.Repo || b.Branch != "work" {
		t.Fatalf("main %+v, worktree %+v", a, b)
	}
	renamed := filepath.Join(root, "Renamed")
	if err := os.Rename(main, renamed); err != nil {
		t.Fatal(err)
	}
	if got := c.Lookup(renamed); got.Name != "Renamed" || got.Branch != "main" {
		t.Fatalf("rename stale: %+v", got)
	}
}

func BenchmarkCachedRepoLookup(b *testing.B) {
	root := b.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0700); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0600); err != nil {
		b.Fatal(err)
	}
	for _, cached := range []bool{false, true} {
		b.Run(fmt.Sprint(cached), func(b *testing.B) {
			var c Cache
			c.Lookup(root)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if cached {
					c.Lookup(root)
				} else {
					Branch(root)
					Repo(root)
				}
			}
		})
	}
}

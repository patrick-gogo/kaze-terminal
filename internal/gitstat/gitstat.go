// Package gitstat counts the lines a working tree has added and removed.
package gitstat

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

const (
	maxOutput = 4 << 10
	// waitDelay bounds the read after a kill: on Windows git's launcher leaves a
	// child holding the stdout pipe, which would otherwise block Wait forever.
	waitDelay = 500 * time.Millisecond
)

var (
	insRe = regexp.MustCompile(`(\d+) insertions?\(\+\)`)
	delRe = regexp.MustCompile(`(\d+) deletions?\(-\)`)
)

// head keeps the first max bytes and discards the rest.
type head struct {
	buf bytes.Buffer
	max int
}

func (h *head) Write(p []byte) (int, error) {
	if room := h.max - h.buf.Len(); room > 0 {
		h.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// Shortstat runs `git diff --shortstat HEAD` in dir. Empty output is 0, 0.
// It never refreshes the index: that takes index.lock and would fail the git
// commands Claude runs in the same repo. --no-optional-locks alone still refreshes.
func Shortstat(ctx context.Context, dir string) (add, del int, err error) {
	return shortstat(ctx, "git", "--no-optional-locks",
		"-c", "diff.autoRefreshIndex=false", "-c", "core.fsmonitor=false",
		"-C", dir, "diff", "--no-ext-diff", "--no-textconv", "--shortstat", "HEAD")
}

func shortstat(ctx context.Context, name string, args ...string) (add, del int, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	out := &head{max: maxOutput}
	cmd.Stdout = out
	cmd.WaitDelay = waitDelay
	if err := cmd.Run(); err != nil {
		return 0, 0, err
	}
	add, del = parse(out.buf.String())
	return add, del, nil
}

func parse(s string) (add, del int) {
	if m := insRe.FindStringSubmatch(s); m != nil {
		add, _ = strconv.Atoi(m[1])
	}
	if m := delRe.FindStringSubmatch(s); m != nil {
		del, _ = strconv.Atoi(m[1])
	}
	return add, del
}

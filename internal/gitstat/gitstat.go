// Package gitstat counts the lines a working tree has added and removed.
package gitstat

import (
	"context"
	"io"
	"os/exec"
	"regexp"
	"strconv"
)

const maxOutput = 4 << 10

var (
	insRe = regexp.MustCompile(`(\d+) insertions?\(\+\)`)
	delRe = regexp.MustCompile(`(\d+) deletions?\(-\)`)
)

// Shortstat runs `git diff --shortstat HEAD` in dir. Empty output is 0, 0.
func Shortstat(ctx context.Context, dir string) (add, del int, err error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--shortstat", "HEAD")
	hideWindow(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return 0, 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, 0, err
	}
	buf, _ := io.ReadAll(io.LimitReader(out, maxOutput))
	// Drain so git never blocks on a full pipe before Wait.
	_, _ = io.Copy(io.Discard, out)
	if err := cmd.Wait(); err != nil {
		return 0, 0, err
	}
	add, del = parse(string(buf))
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

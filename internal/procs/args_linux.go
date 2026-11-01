package procs

import (
	"bytes"
	"os"
	"strconv"
)

const maxArgs = 4

// processArgs reads /proc/<pid>/exe and the first arguments of /proc/<pid>/cmdline.
func processArgs(pid int) (string, []string) {
	dir := "/proc/" + strconv.Itoa(pid)
	exe, _ := os.Readlink(dir + "/exe")
	b, err := os.ReadFile(dir + "/cmdline")
	if err != nil {
		return exe, nil
	}
	var args []string
	for _, a := range bytes.Split(bytes.TrimRight(b, "\x00"), []byte{0}) {
		if len(args) == maxArgs {
			break
		}
		args = append(args, string(a))
	}
	return exe, args
}

package procs

import (
	"bytes"
	"encoding/binary"

	"golang.org/x/sys/unix"
)

// maxArgs bounds how many arguments are read; an agent is named by the first two.
const maxArgs = 4

// processArgs reads kern.procargs2: argc, the executable path, padding, then argv.
func processArgs(pid int) (string, []string) {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(buf) < 4 {
		return "", nil
	}
	argc := int(binary.LittleEndian.Uint32(buf))
	rest := buf[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return "", nil
	}
	exe := string(rest[:i])
	rest = bytes.TrimLeft(rest[i:], "\x00")
	var args []string
	for len(args) < min(argc, maxArgs) && len(rest) > 0 {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			j = len(rest)
		}
		args = append(args, string(rest[:j]))
		rest = rest[min(j+1, len(rest)):]
	}
	return exe, args
}

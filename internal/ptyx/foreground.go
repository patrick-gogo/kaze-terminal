package ptyx

// Foregrounder reports the process group in front of a pty: the program the
// user is running in it right now (a shell, or what the shell started).
type Foregrounder interface {
	// Foreground is that group's id, which is its leader's pid; 0 when unknown.
	Foreground() int
}

func (s *session) Foreground() int {
	if s.closed.Load() {
		return 0
	}
	return foreground(s.p.Fd())
}

// Rooter reports the stable shell process, independent of foreground jobs.
type Rooter interface{ RootPID() int }

func (s *session) RootPID() int {
	if s.closed.Load() || s.cmd == nil || s.cmd.Process == nil {
		return 0
	}
	select {
	case <-s.exited:
		return 0
	default:
	}
	return s.cmd.Process.Pid
}

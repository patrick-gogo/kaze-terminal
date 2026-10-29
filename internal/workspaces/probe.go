package workspaces

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"time"
)

const (
	dockerEvery   = 2 * time.Second
	dockerTimeout = 2 * time.Second
	portEvery     = time.Second
	portTimeout   = 500 * time.Millisecond
	overallLimit  = 3 * time.Minute
)

// ErrDockerMissing means neither the docker CLI nor Docker Desktop is installed.
var ErrDockerMissing = errors.New("docker is not installed")

// Probes checks readiness of Docker and local ports. Every field is optional;
// the zero value uses the real system. After paces the retry loop and Timeout
// bounds one probe, so tests can drive both without sleeping.
type Probes struct {
	Exec        func(ctx context.Context, name string, args ...string) error
	Dial        func(ctx context.Context, addr string) error
	StartDocker func() error
	Now         func() time.Time
	After       func(time.Duration) <-chan time.Time
	Timeout     func(time.Duration) <-chan time.Time
}

func (p Probes) withDefaults() Probes {
	if p.Exec == nil {
		p.Exec = func(ctx context.Context, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			hideWindow(cmd)
			return cmd.Run()
		}
	}
	if p.Dial == nil {
		p.Dial = func(ctx context.Context, addr string) error {
			var d net.Dialer
			conn, err := d.DialContext(ctx, "tcp", addr)
			if err != nil {
				return err
			}
			return conn.Close()
		}
	}
	if p.StartDocker == nil {
		p.StartDocker = startDockerDesktop
	}
	if p.Now == nil {
		p.Now = time.Now
	}
	if p.After == nil {
		p.After = time.After
	}
	if p.Timeout == nil {
		p.Timeout = time.After
	}
	return p
}

// probeOnce runs fn with a per-probe deadline; a probe that ignores its
// context is abandoned so one hung command cannot stall the loop.
func (p Probes) probeOnce(ctx context.Context, limit time.Duration, fn func(context.Context) error) error {
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	res := make(chan error, 1)
	go func() { res <- fn(pctx) }()
	select {
	case err := <-res:
		return err
	case <-p.Timeout(limit):
		return context.DeadlineExceeded
	case <-ctx.Done():
		return ctx.Err()
	}
}

// wait runs probe now and then every `every` until it succeeds, the context
// ends, or the overall limit passes. onFail may stop the loop early.
func (p Probes) wait(ctx context.Context, every time.Duration, probe func() error, onFail func(error) error) error {
	start := p.Now()
	for {
		err := probe()
		if err == nil {
			return nil
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if onFail != nil {
			if stop := onFail(err); stop != nil {
				return stop
			}
		}
		if p.Now().Sub(start) >= overallLimit {
			return context.DeadlineExceeded
		}
		select {
		case <-p.After(every):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// WaitDocker blocks until `docker info` succeeds. It starts Docker Desktop at
// most once, and returns ErrDockerMissing if there is no CLI and no Desktop.
func (p Probes) WaitDocker(ctx context.Context) error {
	p = p.withDefaults()
	started := false
	return p.wait(ctx, dockerEvery, func() error {
		return p.probeOnce(ctx, dockerTimeout, func(c context.Context) error {
			return p.Exec(c, "docker", "info")
		})
	}, func(err error) error {
		if started {
			return nil
		}
		started = true
		serr := p.StartDocker()
		if errors.Is(serr, ErrDockerMissing) && errors.Is(err, exec.ErrNotFound) {
			return ErrDockerMissing
		}
		return nil
	})
}

// WaitPort blocks until something accepts connections on 127.0.0.1 or ::1.
func (p Probes) WaitPort(ctx context.Context, port int) error {
	p = p.withDefaults()
	ps := strconv.Itoa(port)
	addrs := []string{net.JoinHostPort("127.0.0.1", ps), net.JoinHostPort("::1", ps)}
	return p.wait(ctx, portEvery, func() error {
		var last error
		for _, a := range addrs {
			last = p.probeOnce(ctx, portTimeout, func(c context.Context) error { return p.Dial(c, a) })
			if last == nil {
				return nil
			}
			if ctx.Err() != nil {
				return last
			}
		}
		return last
	}, nil)
}

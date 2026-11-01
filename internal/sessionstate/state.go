// Package sessionstate saves bounded terminal launch metadata, never terminal output.
package sessionstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode"
)

const MaxBytes = 1 << 20
const MaxEntries = 128

var replace = replaceFile

var ErrOwned = errors.New("session restoration belongs to another Kaze window")

type Entry struct {
	Cwd           string `json:"cwd"`
	Title         string `json:"title,omitempty"`
	FixedTitle    string `json:"fixed_title,omitempty"`
	Color         string `json:"color,omitempty"`
	Agent         string `json:"agent,omitempty"`
	Session       string `json:"session,omitempty"`
	Transcript    string `json:"transcript,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	Run           uint64 `json:"run,omitempty"`
	Member        int    `json:"member,omitempty"`
}
type Snapshot struct {
	Version int     `json:"version"`
	Active  int     `json:"active"`
	Entries []Entry `json:"entries"`
}

func ValidID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
func (s Snapshot) Validate() error {
	if s.Version != 1 {
		return errors.New("unsupported sessions version")
	}
	if len(s.Entries) > MaxEntries {
		return errors.New("too many saved sessions (limit 128)")
	}
	if s.Active < 0 || len(s.Entries) > 0 && s.Active >= len(s.Entries) {
		return errors.New("invalid active session")
	}
	for _, e := range s.Entries {
		for _, v := range []string{e.Cwd, e.Title, e.FixedTitle, e.Color, e.Agent, e.Session, e.Transcript, e.Workspace, e.WorkspaceName} {
			if len(v) > 4096 || strings.IndexFunc(v, unicode.IsControl) >= 0 {
				return errors.New("invalid or oversized session field")
			}
		}
		// Invalid agent identities degrade per entry at restore time, never to a latest-session selector.
		if e.Member < 0 {
			return errors.New("invalid workspace member")
		}
	}
	return nil
}
func Load(path string) (Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return Snapshot{}, err
	}
	if len(b) > MaxBytes {
		return Snapshot{}, errors.New("sessions file exceeds 1 MiB")
	}
	var s Snapshot
	if err = json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	return s, s.Validate()
}
func Save(path string, s Snapshot) error {
	if err := s.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if len(b) > MaxBytes {
		return errors.New("sessions file exceeds 1 MiB")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".sessions-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replace(f.Name(), path)
}

// Live is immutable metadata copied from the tab manager. Seq counts shell prompts.
type Live struct {
	ID                                               int
	Cwd, FixedTitle, Color, Workspace, WorkspaceName string
	Run                                              uint64
	Member                                           int
	Seq                                              uint64
	Exited                                           bool
}
type identity struct {
	agent, id, transcript string
	seq                   uint64
	pending               bool
}
type Store struct {
	mu                                          sync.Mutex
	path                                        string
	lock                                        *os.File
	tabs                                        []Live
	active                                      int
	names                                       map[int]string
	agents                                      map[int]identity
	enabled, frozen, closed, dirty, initialized bool
	wake                                        chan struct{}
	requests                                    chan request
	done                                        chan struct{}
	err                                         error
}
type request struct {
	stop  bool
	reply chan error
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	s := &Store{path: path, lock: f, enabled: true, names: map[int]string{}, agents: map[int]identity{}, wake: make(chan struct{}, 1), requests: make(chan request), done: make(chan struct{})}
	go s.worker()
	return s, nil
}
func (s *Store) changed() {
	s.dirty = true
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Store) SetTabs(t []Live, active int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.frozen {
		return
	}
	if s.initialized && reflect.DeepEqual(s.tabs, t) && s.active == active {
		return
	}
	s.initialized = true
	s.tabs = append([]Live(nil), t...)
	s.active = active
	keep := map[int]bool{}
	for _, t := range t {
		keep[t.ID] = true
		a := s.agents[t.ID]
		if t.Exited || (!a.pending && t.Seq > a.seq) {
			delete(s.agents, t.ID)
		}
	}
	for id := range s.agents {
		if !keep[id] {
			delete(s.agents, id)
		}
	}
	for id := range s.names {
		if !keep[id] {
			delete(s.names, id)
		}
	}
	s.changed()
}
func (s *Store) Rename(id int, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.frozen {
		return
	}
	s.names[id] = name
	s.changed()
}
func (s *Store) Note(id int, agent, uuid, transcript string, seq uint64, start bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.frozen {
		return
	}
	var live *Live
	for i := range s.tabs {
		if s.tabs[i].ID == id {
			live = &s.tabs[i]
			break
		}
	}
	if live == nil || live.Seq > seq {
		return
	}
	a := s.agents[id]
	if a.agent != agent || start {
		a = identity{agent: agent, seq: seq}
	}
	if ValidID(uuid) && (agent == "claude" || agent == "codex") {
		a = identity{agent: agent, id: uuid, transcript: transcript, seq: seq}
	}
	s.agents[id] = a
	s.changed()
}
func (s *Store) Pending(id int, e Entry, seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[id] = identity{agent: e.Agent, id: e.Session, transcript: e.Transcript, seq: seq, pending: true}
	s.changed()
}
func (s *Store) Launched(id int, seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.frozen {
		return
	}
	a := s.agents[id]
	if a.pending {
		a.pending = false
		a.seq = seq
		s.agents[id] = a
		s.changed()
	}
}
func (s *Store) Cancel(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.frozen {
		return
	}
	delete(s.agents, id)
	s.changed()
}
func (s *Store) SetEnabled(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.enabled = on
	s.changed()
}
func (s *Store) Freeze(on bool) { s.mu.Lock(); s.frozen = on; s.mu.Unlock() }
func (s *Store) snapshot() Snapshot {
	out := Snapshot{Version: 1}
	for _, t := range s.tabs {
		if len(out.Entries) >= MaxEntries {
			break
		}
		if t.ID == s.active {
			out.Active = len(out.Entries)
		}
		a := s.agents[t.ID]
		e := Entry{Cwd: t.Cwd, Title: s.names[t.ID], FixedTitle: t.FixedTitle, Color: t.Color, Workspace: t.Workspace, WorkspaceName: t.WorkspaceName, Run: t.Run, Member: t.Member, Agent: a.agent, Session: a.id, Transcript: a.transcript}
		out.Entries = append(out.Entries, e)
	}
	return out
}
func (s *Store) write() error {
	s.mu.Lock()
	if !s.initialized && s.enabled {
		err := s.err
		s.mu.Unlock()
		return err
	}
	if !s.dirty {
		err := s.err
		s.mu.Unlock()
		return err
	}
	s.dirty = false
	enabled, count, snap := s.enabled, len(s.tabs), s.snapshot()
	s.mu.Unlock()
	var err error
	if !enabled {
		err = os.Remove(s.path)
		if os.IsNotExist(err) {
			err = nil
		}
	} else if count > MaxEntries {
		err = fmt.Errorf("session restoration limit is %d; close extra sessions to save", MaxEntries)
	} else {
		err = Save(s.path, snap)
	}
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
	return err
}
func (s *Store) worker() {
	defer close(s.done)
	var timer *time.Timer
	var tick <-chan time.Time
	for {
		select {
		case <-s.wake:
			if timer == nil {
				timer = time.NewTimer(250 * time.Millisecond)
				tick = timer.C
			}
		case <-tick:
			s.write()
			timer = nil
			tick = nil
		case r := <-s.requests:
			if timer != nil {
				timer.Stop()
				timer = nil
				tick = nil
			}
			err := s.write()
			r.reply <- err
			if r.stop {
				return
			}
		}
	}
}
func (s *Store) Flush() error {
	s.mu.Lock()
	if !s.closed && (s.initialized || !s.enabled) {
		s.dirty = true
	}
	s.mu.Unlock()
	r := request{reply: make(chan error, 1)}
	select {
	case s.requests <- r:
		return <-r.reply
	case <-s.done:
		return s.Err()
	}
}
func (s *Store) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.err }
func (s *Store) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	r := request{stop: true, reply: make(chan error, 1)}
	s.requests <- r
	err := <-r.reply
	<-s.done
	unlockFile(s.lock)
	s.lock.Close()
	return err
}

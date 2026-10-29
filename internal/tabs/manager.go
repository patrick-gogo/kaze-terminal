// Package tabs owns the open terminal tabs: their shells, order, the active
// tab and which tab may ask the window for a frame.
package tabs

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

// TabID identifies a tab for its lifetime; IDs are never reused.
type TabID int

// Tab is one shell. Title and Exited are refreshed by Tabs and Active, so read
// them only on the goroutine that calls those (the UI goroutine).
type Tab struct {
	ID      TabID
	Session *term.Session
	Title   string
	Exited  bool

	// FixedTitle, when set, is shown instead of whatever title the shell sets.
	FixedTitle string
	Color      string
	Group      *Group // nil for a tab that no workspace run opened
}

// Group says which workspace run opened a tab and which of its tabs it is.
type Group struct {
	Workspace, Name string
	Run             uint64
	Member          int
}

// StartOpts are a new tab's optional settings; the zero value is a plain New.
type StartOpts struct {
	Dir, Title, Color string // an empty Dir starts the shell in the window's default folder
	Group             *Group
}

// entry holds what the session's notifier goroutine needs without taking the manager lock.
type entry struct {
	tab     *Tab
	sess    atomic.Pointer[term.Session]
	removed atomic.Bool
	watched atomic.Bool // a background tab that asks for frames on output too
	// Touched only by this session's single notifier goroutine.
	seenTitle  string
	seenExited bool
}

// Manager is driven from the UI goroutine; onChange may be called from any goroutine.
type Manager struct {
	start    func(env []string, dir string) (ptyx.Session, error)
	onChange func()
	baseEnv  []string

	closing sync.WaitGroup // shells Close is still releasing in the background

	mu        sync.Mutex
	entries   []*entry
	nextID    TabID
	cols      int
	rows      int
	notifyEnv []string // KAZE_NOTIFY vars for new tabs, empty until SetNotify
	sbLines   int      // history cap for new tabs; 0 keeps the screen default

	active atomic.Int64 // TabID of the active tab, 0 when there is none
}

// NewManager starts no shell yet. start launches one shell with the given env
// in dir, where "" means the default folder; onChange asks the window for a frame.
func NewManager(start func(env []string, dir string) (ptyx.Session, error), cols, rows int, onChange func()) *Manager {
	if onChange == nil {
		onChange = func() {}
	}
	return &Manager{start: start, onChange: onChange, baseEnv: baseEnv(os.Environ()), cols: cols, rows: rows}
}

// Inherited terminal identity is dropped (a kaze started from Windows Terminal or
// VS Code must not claim to be them) and kaze's own is set. KAZE_TAB_ID is
// dropped too so a kaze started inside a kaze tab gets clean IDs.
var droppedEnv = []string{"KAZE_TAB_ID", "KAZE_NOTIFY", "KAZE_NOTIFY_TOKEN", "TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "WT_SESSION", "WT_PROFILE_ID"}

var identityEnv = []string{"TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=kaze"}

func baseEnv(env []string) []string {
	out := make([]string, 0, len(env)+len(identityEnv)+1)
	for _, e := range env {
		if !isDropped(e) {
			out = append(out, e)
		}
	}
	return append(out, identityEnv...)
}

func isDropped(e string) bool {
	name, _, _ := strings.Cut(e, "=")
	for _, d := range droppedEnv {
		if strings.EqualFold(name, d) { // Windows env names ignore case
			return true
		}
	}
	return false
}

// SetNotify gives every tab opened from now on the alert address and token
// that `kaze notify` reads.
func (m *Manager) SetNotify(addr, token string) {
	m.mu.Lock()
	m.notifyEnv = []string{"KAZE_NOTIFY=" + addr, "KAZE_NOTIFY_TOKEN=" + token}
	m.mu.Unlock()
}

// New starts a shell in a new tab at the end and makes it active.
func (m *Manager) New() (*Tab, error) { return m.NewWith(nil) }

// NewWith is New with a hook that writes into the tab before its shell's output is read.
func (m *Manager) NewWith(prefeed func(*term.Session)) (*Tab, error) {
	return m.newTab(StartOpts{}, prefeed)
}

// NewOpts is New with its own folder, fixed title, color and group.
func (m *Manager) NewOpts(o StartOpts) (*Tab, error) { return m.newTab(o, nil) }

func (m *Manager) newTab(o StartOpts, prefeed func(*term.Session)) (*Tab, error) {
	m.mu.Lock()
	m.nextID++
	id := m.nextID
	cols, rows, sb, nenv := m.cols, m.rows, m.sbLines, m.notifyEnv
	m.mu.Unlock()

	env := append(slices.Clip(m.baseEnv), nenv...)
	env = append(env, "KAZE_TAB_ID="+strconv.Itoa(int(id)))
	p, err := m.start(env, o.Dir)
	if err != nil {
		return nil, err
	}
	e := &entry{tab: &Tab{ID: id, Title: o.Title, FixedTitle: o.Title, Color: o.Color, Group: o.Group}}
	s := term.NewSessionOpts(p, cols, rows, term.Opts{Scrollback: sb, Prefeed: prefeed}, func() { m.notify(e) })
	e.sess.Store(s)
	e.tab.Session = s

	m.mu.Lock()
	m.entries = append(m.entries, e)
	m.mu.Unlock()
	m.active.Store(int64(id))
	return e.tab, nil
}

// notify runs on the tab's notifier goroutine. A background tab keeps applying
// output but only asks for a frame when the tab bar would change.
func (m *Manager) notify(e *entry) {
	if e.removed.Load() {
		return
	}
	if TabID(m.active.Load()) == e.tab.ID || e.watched.Load() {
		m.onChange()
		return
	}
	s := e.sess.Load()
	if s == nil {
		return
	}
	exited, _ := s.Exited()
	title := e.tab.FixedTitle // set once at creation, so safe to read here
	if title == "" {
		s.Lock()
		title = s.Applier().Title()
		s.Unlock()
	}
	if title != e.seenTitle || exited != e.seenExited {
		e.seenTitle, e.seenExited = title, exited
		m.onChange()
	}
}

func refresh(t *Tab) {
	t.Exited, _ = t.Session.Exited()
	if t.FixedTitle != "" {
		t.Title = t.FixedTitle
		return
	}
	t.Session.Lock()
	t.Title = t.Session.Applier().Title()
	t.Session.Unlock()
}

// Tabs returns the tabs in display order with Title and Exited refreshed.
func (m *Manager) Tabs() []*Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Tab, len(m.entries))
	for i, e := range m.entries {
		refresh(e.tab)
		out[i] = e.tab
	}
	return out
}

// Active is the active tab, or nil when none are open.
func (m *Manager) Active() *Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.indexLocked(TabID(m.active.Load()))
	if i < 0 {
		return nil
	}
	t := m.entries[i].tab
	refresh(t)
	return t
}

func (m *Manager) indexLocked(id TabID) int {
	for i, e := range m.entries {
		if e.tab.ID == id {
			return i
		}
	}
	return -1
}

// Has reports whether id is an open tab. Safe from any goroutine.
func (m *Manager) Has(id TabID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.indexLocked(id) >= 0
}

// Lookup is the open tab with id, or nil. Unlike Tabs it changes nothing on
// the tab, so it is safe from any goroutine.
func (m *Manager) Lookup(id TabID) *Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i := m.indexLocked(id); i >= 0 {
		return m.entries[i].tab
	}
	return nil
}

// Watch makes a background tab ask for a frame on every output while on, for
// a caller waiting on something the shell prints; an unknown id is ignored.
func (m *Manager) Watch(id TabID, on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i := m.indexLocked(id); i >= 0 {
		m.entries[i].watched.Store(on)
	}
}

// SetActive switches to id; an unknown id is ignored.
func (m *Manager) SetActive(id TabID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.indexLocked(id) >= 0 {
		m.active.Store(int64(id))
	}
}

func (m *Manager) step(d int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.entries)
	if n == 0 {
		return
	}
	i := m.indexLocked(TabID(m.active.Load()))
	i = ((i+d)%n + n) % n
	m.active.Store(int64(m.entries[i].tab.ID))
}

// Next and Prev move the active tab, wrapping at the ends.
func (m *Manager) Next() { m.step(1) }
func (m *Manager) Prev() { m.step(-1) }

// Close removes the tab and kills its shell in the background (ConPTY can take up
// to a second to release). Closing the active tab activates its right neighbour,
// or its left one when it was last.
func (m *Manager) Close(id TabID) {
	m.mu.Lock()
	i := m.indexLocked(id)
	if i < 0 {
		m.mu.Unlock()
		return
	}
	e := m.entries[i]
	e.removed.Store(true)
	m.entries = slices.Delete(m.entries, i, i+1)
	if TabID(m.active.Load()) == id {
		switch {
		case len(m.entries) == 0:
			m.active.Store(0)
		case i < len(m.entries):
			m.active.Store(int64(m.entries[i].tab.ID))
		default:
			m.active.Store(int64(m.entries[i-1].tab.ID))
		}
	}
	m.mu.Unlock()
	m.closing.Add(1)
	go func() { defer m.closing.Done(); e.tab.Session.Close() }()
}

// closeWait bounds how long CloseAll waits for shells to be released.
const closeWait = 2 * time.Second

// CloseAll closes every tab and waits, up to closeWait, for every shell to be
// released, including ones Close is still releasing.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	es := m.entries
	m.entries = nil
	m.active.Store(0)
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, e := range es {
		e.removed.Store(true)
		wg.Add(1)
		go func() { defer wg.Done(); e.tab.Session.Close() }()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); m.closing.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(closeWait): // a hung PTY must not keep the app from quitting
	}
}

// Empty reports that the last tab was closed; the window quits then.
func (m *Manager) Empty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries) == 0
}

// Size is the grid size every tab has, and new tabs start with.
func (m *Manager) Size() (cols, rows int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cols, m.rows
}

// ResizeAll resizes every tab's screen and PTY; an unchanged size does nothing.
func (m *Manager) ResizeAll(cols, rows int) {
	m.mu.Lock()
	if cols == m.cols && rows == m.rows {
		m.mu.Unlock()
		return
	}
	m.cols, m.rows = cols, rows
	es := slices.Clone(m.entries)
	m.mu.Unlock()
	for _, e := range es {
		e.tab.Session.Resize(cols, rows)
	}
}

// SetScrollback sets the history cap, in lines, for tabs opened after it.
func (m *Manager) SetScrollback(lines int) {
	m.mu.Lock()
	m.sbLines = lines
	m.mu.Unlock()
}

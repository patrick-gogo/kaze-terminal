package tabs

// StateTab contains immutable launch metadata and a synchronized shell report.
// Unlike Tabs, State never updates mutable UI titles or exit fields.
type StateTab struct {
	ID                     TabID
	Dir, FixedTitle, Color string
	Group                  *Group
	Seq                    uint64
	Exited                 bool
}

func (m *Manager) SetStateObserver(f func()) {
	if f == nil {
		m.stateObserver.Store(nil)
	} else {
		m.stateObserver.Store(&f)
	}
}
func (m *Manager) stateChanged() {
	if f := m.stateObserver.Load(); f != nil {
		(*f)()
	}
}
func (m *Manager) State() ([]StateTab, TabID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]StateTab, 0, len(m.entries))
	for _, e := range m.entries {
		t := e.tab
		if t.Parent != 0 {
			continue
		}
		s := t.Session
		s.Lock()
		cwd := s.Applier().Cwd()
		seq := s.Applier().CwdSeq() + s.Applier().PromptSeq()
		s.Unlock()
		if cwd == "" {
			cwd = t.Dir
		}
		exited, _ := s.Exited()
		out = append(out, StateTab{ID: t.ID, Dir: cwd, FixedTitle: t.FixedTitle, Color: t.Color, Group: t.Group, Seq: seq, Exited: exited})
	}
	return out, TabID(m.active.Load())
}

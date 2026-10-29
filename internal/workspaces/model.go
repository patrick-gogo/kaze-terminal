// Package workspaces holds the saved recipes kaze can start in one click:
// the model with its validation, and the workspaces.toml store.
package workspaces

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Wait is what a tab waits for before the next one starts.
type Wait string

const (
	WaitNone   Wait = "none"
	WaitDocker Wait = "docker"
	WaitPort   Wait = "port"
)

// Bounds are implementation limits, not UI counters.
const (
	MaxNameRunes   = 80
	MaxTabs        = 20
	MaxCommands    = 20
	MaxCommandSize = 8 << 10
	MaxFileSize    = 1 << 20
)

var colors = map[string]bool{"cyan": true, "magenta": true, "yellow": true, "green": true, "blue": true, "red": true}

type Tab struct {
	ID       string   `toml:"id"`
	Title    string   `toml:"title"`
	Folder   string   `toml:"folder"`
	Commands []string `toml:"commands"`
	Color    string   `toml:"color"`
	Wait     Wait     `toml:"wait"`
	Port     int      `toml:"port,omitempty"`
}

type Workspace struct {
	ID   string `toml:"id"`
	Name string `toml:"name"`
	Tabs []Tab  `toml:"tabs"`
}

type Document struct {
	Workspaces []Workspace
}

// FieldError names one problem; Workspace and Tab are IDs, empty when it does not apply.
type FieldError struct {
	Workspace, Tab, Field, Msg string
}

func (e FieldError) Error() string {
	where := e.Workspace
	if e.Tab != "" {
		where += "/" + e.Tab
	}
	if where == "" {
		return e.Msg
	}
	return where + ": " + e.Msg
}

// NewID returns a random workspace ID.
func NewID() string { return randomID("ws_") }

// NewTabID returns a random tab ID.
func NewTabID() string { return randomID("t_") }

func randomID(prefix string) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	return prefix + hex.EncodeToString(b[:])
}

func (d Document) ByID(id string) (Workspace, bool) {
	for _, w := range d.Workspaces {
		if w.ID == id {
			return w, true
		}
	}
	return Workspace{}, false
}

// ByName matches ignoring case and outer whitespace; names are unique in a valid document.
func (d Document) ByName(name string) (Workspace, bool) {
	want := nameKey(name)
	for _, w := range d.Workspaces {
		if nameKey(w.Name) == want {
			return w, true
		}
	}
	return Workspace{}, false
}

func nameKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func folderExample() string {
	if runtime.GOOS == "windows" {
		return `C:\Users\you\project`
	}
	return "/home/you/project"
}

func hasTilde(folder string) bool {
	return folder == "~" || strings.HasPrefix(folder, "~/") || strings.HasPrefix(folder, `~\`)
}

// ExpandFolder expands a leading ~ to home and requires an absolute result.
// Nothing else is expanded and spaces and Unicode are kept.
func ExpandFolder(folder, home string) (string, error) {
	bad := fmt.Errorf("Folder must be a full path, like %s", folderExample())
	if folder == "" || strings.IndexFunc(folder, unicode.IsControl) >= 0 {
		return "", bad
	}
	out := folder
	if hasTilde(folder) {
		if home == "" {
			return "", fmt.Errorf("Folder starts with ~ but your home folder is not known")
		}
		out = filepath.Join(home, folder[1:])
	}
	if !filepath.IsAbs(out) {
		return "", bad
	}
	return out, nil
}

// Validate checks the whole document. home expands a leading ~ in folders.
func (d Document) Validate(home string) []FieldError { return d.validate(home, false) }

// validate with tildeOK skips the home lookup, for callers that do not know home.
func (d Document) validate(home string, tildeOK bool) []FieldError {
	var errs []FieldError
	names := map[string]bool{}
	wsIDs := map[string]bool{}
	for _, w := range d.Workspaces {
		add := func(tab, field, msg string) { errs = append(errs, FieldError{w.ID, tab, field, msg}) }
		if msg := checkID(w.ID); msg != "" {
			add("", "id", msg)
		} else if wsIDs[w.ID] {
			add("", "id", fmt.Sprintf("Two workspaces share the ID %q", w.ID))
		}
		wsIDs[w.ID] = true

		if msg := checkText(w.Name, "Workspace name"); msg != "" {
			add("", "name", msg)
		} else if k := nameKey(w.Name); names[k] {
			add("", "name", fmt.Sprintf("Another workspace is already named %q", strings.TrimSpace(w.Name)))
		} else {
			names[k] = true
		}
		if len(w.Tabs) > MaxTabs {
			add("", "tabs", fmt.Sprintf("A workspace can have at most %d tabs", MaxTabs))
		}
		tabIDs := map[string]bool{}
		for _, t := range w.Tabs {
			tadd := func(field, msg string) { add(t.ID, field, msg) }
			if msg := checkID(t.ID); msg != "" {
				tadd("id", msg)
			} else if tabIDs[t.ID] {
				tadd("id", fmt.Sprintf("Two tabs in this workspace share the ID %q", t.ID))
			}
			tabIDs[t.ID] = true
			if msg := checkText(t.Title, "Tab title"); msg != "" {
				tadd("title", msg)
			}
			if !(tildeOK && hasTilde(t.Folder)) {
				if _, err := ExpandFolder(t.Folder, home); err != nil {
					tadd("folder", err.Error())
				}
			}
			if len(t.Commands) > MaxCommands {
				tadd("commands", fmt.Sprintf("A tab can have at most %d commands", MaxCommands))
			}
			for _, c := range t.Commands {
				if len(c) > MaxCommandSize {
					tadd("commands", fmt.Sprintf("A command can be at most %d KiB", MaxCommandSize>>10))
				} else if strings.IndexFunc(c, unicode.IsControl) >= 0 {
					tadd("commands", "Each command must be a single line without control characters")
				}
			}
			if t.Color != "" && !colors[t.Color] {
				tadd("color", "Color must be one of cyan, magenta, yellow, green, blue, red")
			}
			switch t.Wait {
			case "", WaitNone, WaitDocker:
				if t.Port != 0 {
					tadd("port", "Port is only used when the tab waits for a port")
				}
			case WaitPort:
				if t.Port < 1 || t.Port > 65535 {
					tadd("port", "Port must be a number from 1 to 65535")
				}
			default:
				tadd("wait", "Wait must be none, docker or port")
			}
		}
	}
	return errs
}

func checkID(id string) string {
	if id == "" || strings.IndexFunc(id, unicode.IsControl) >= 0 || utf8.RuneCountInString(id) > MaxNameRunes {
		return "Every workspace and tab needs an ID of up to 80 characters without control characters"
	}
	return ""
}

func checkText(s, what string) string {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return what + " cannot be empty"
	case utf8.RuneCountInString(s) > MaxNameRunes:
		return fmt.Sprintf("%s can be at most %d characters", what, MaxNameRunes)
	case strings.IndexFunc(s, unicode.IsControl) >= 0:
		return what + " cannot contain control characters"
	}
	return ""
}

// normalize trims names and titles, drops blank commands and fills the default
// wait; it copies what it changes.
func (d Document) normalize() Document {
	out := Document{Workspaces: make([]Workspace, len(d.Workspaces))}
	for i, w := range d.Workspaces {
		w.Name = strings.TrimSpace(w.Name)
		tabs := make([]Tab, len(w.Tabs))
		for j, t := range w.Tabs {
			t.Title = strings.TrimSpace(t.Title)
			cmds := make([]string, 0, len(t.Commands))
			for _, c := range t.Commands {
				if strings.TrimSpace(c) != "" {
					cmds = append(cmds, c)
				}
			}
			t.Commands = cmds
			if t.Wait == "" {
				t.Wait = WaitNone
			}
			tabs[j] = t
		}
		w.Tabs = tabs
		out.Workspaces[i] = w
	}
	return out
}

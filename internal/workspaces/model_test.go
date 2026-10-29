package workspaces

import (
	"path/filepath"
	"strings"
	"testing"
)

func okTab(home string) Tab {
	return Tab{ID: "t_1", Title: "Code", Folder: filepath.Join(home, "src"), Commands: []string{"claude"}, Color: "magenta", Wait: WaitNone}
}

func okDoc(home string) Document {
	return Document{Workspaces: []Workspace{{ID: "ws_1", Name: "My project", Tabs: []Tab{okTab(home)}}}}
}

func fieldsOf(errs []FieldError) string {
	var s []string
	for _, e := range errs {
		s = append(s, e.Field)
	}
	return strings.Join(s, ",")
}

func TestValidateOK(t *testing.T) {
	home := t.TempDir()
	if errs := okDoc(home).Validate(home); len(errs) != 0 {
		t.Fatalf("unexpected: %v", errs)
	}
}

func TestValidateRejects(t *testing.T) {
	home := t.TempDir()
	cases := map[string]struct {
		mut   func(d *Document)
		field string
	}{
		"bad color":        {func(d *Document) { d.Workspaces[0].Tabs[0].Color = "pink" }, "color"},
		"port 0":           {func(d *Document) { d.Workspaces[0].Tabs[0].Wait, d.Workspaces[0].Tabs[0].Port = WaitPort, 0 }, "port"},
		"port 65536":       {func(d *Document) { d.Workspaces[0].Tabs[0].Wait, d.Workspaces[0].Tabs[0].Port = WaitPort, 65536 }, "port"},
		"port with none":   {func(d *Document) { d.Workspaces[0].Tabs[0].Port = 80 }, "port"},
		"port with docker": {func(d *Document) { d.Workspaces[0].Tabs[0].Wait, d.Workspaces[0].Tabs[0].Port = WaitDocker, 80 }, "port"},
		"bad wait":         {func(d *Document) { d.Workspaces[0].Tabs[0].Wait = "soon" }, "wait"},
		"CR in command":    {func(d *Document) { d.Workspaces[0].Tabs[0].Commands = []string{"a\rb"} }, "commands"},
		"LF in command":    {func(d *Document) { d.Workspaces[0].Tabs[0].Commands = []string{"a\nb"} }, "commands"},
		"NUL in command":   {func(d *Document) { d.Workspaces[0].Tabs[0].Commands = []string{"a\x00b"} }, "commands"},
		"big command":      {func(d *Document) { d.Workspaces[0].Tabs[0].Commands = []string{strings.Repeat("a", MaxCommandSize+1)} }, "commands"},
		"21 commands":      {func(d *Document) { d.Workspaces[0].Tabs[0].Commands = make([]string, 21) }, "commands"},
		"empty title":      {func(d *Document) { d.Workspaces[0].Tabs[0].Title = "  " }, "title"},
		"long title":       {func(d *Document) { d.Workspaces[0].Tabs[0].Title = strings.Repeat("あ", 81) }, "title"},
		"control in name":  {func(d *Document) { d.Workspaces[0].Name = "a\tb" }, "name"},
		"empty name":       {func(d *Document) { d.Workspaces[0].Name = "" }, "name"},
		"relative folder":  {func(d *Document) { d.Workspaces[0].Tabs[0].Folder = "src/project" }, "folder"},
		"empty folder":     {func(d *Document) { d.Workspaces[0].Tabs[0].Folder = "" }, "folder"},
		"env var folder":   {func(d *Document) { d.Workspaces[0].Tabs[0].Folder = "$HOME/src" }, "folder"},
		"tilde user":       {func(d *Document) { d.Workspaces[0].Tabs[0].Folder = "~bob/src" }, "folder"},
		"empty id":         {func(d *Document) { d.Workspaces[0].ID = "" }, "id"},
		"duplicate tab id": {func(d *Document) { d.Workspaces[0].Tabs = append(d.Workspaces[0].Tabs, okTab(home)) }, "id"},
		"duplicate ws id": {func(d *Document) {
			w := d.Workspaces[0]
			w.Name = "Other"
			d.Workspaces = append(d.Workspaces, w)
		}, "id"},
		"duplicate name": {func(d *Document) {
			w := d.Workspaces[0]
			w.ID, w.Name = "ws_2", "  MY PROJECT "
			d.Workspaces = append(d.Workspaces, w)
		}, "name"},
		"21 tabs": {func(d *Document) {
			var tabs []Tab
			for i := 0; i < 21; i++ {
				tb := okTab(home)
				tb.ID = NewTabID()
				tabs = append(tabs, tb)
			}
			d.Workspaces[0].Tabs = tabs
		}, "tabs"},
	}
	for name, c := range cases {
		d := okDoc(home)
		c.mut(&d)
		errs := d.Validate(home)
		if len(errs) == 0 || !strings.Contains(fieldsOf(errs), c.field) {
			t.Errorf("%s: want a %q error, got %v", name, c.field, errs)
		}
	}
}

func TestValidateAcceptsBoundaries(t *testing.T) {
	home := t.TempDir()
	d := okDoc(home)
	tab := &d.Workspaces[0].Tabs[0]
	tab.Title = strings.Repeat("あ", 80)
	tab.Wait, tab.Port = WaitPort, 65535
	tab.Commands = []string{strings.Repeat("a", MaxCommandSize)}
	if errs := d.Validate(home); len(errs) != 0 {
		t.Fatalf("unexpected: %v", errs)
	}
}

func TestExpandFolder(t *testing.T) {
	home := t.TempDir()
	got, err := ExpandFolder("~/my src/プロジェクト", home)
	if err != nil || got != filepath.Join(home, "my src", "プロジェクト") {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := ExpandFolder("~", home); err != nil || got != home {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := ExpandFolder("~/x", ""); err == nil {
		t.Fatal("tilde without a home must fail")
	}
	abs := filepath.Join(home, "a")
	if got, err := ExpandFolder(abs, ""); err != nil || got != abs {
		t.Fatalf("absolute: %q, %v", got, err)
	}
	_, err = ExpandFolder("relative", home)
	if err == nil || !strings.Contains(err.Error(), "full path") {
		t.Fatalf("want a plain-language error, got %v", err)
	}
}

func TestLookupsAndIDs(t *testing.T) {
	home := t.TempDir()
	d := okDoc(home)
	if w, ok := d.ByID("ws_1"); !ok || w.Name != "My project" {
		t.Fatal("ByID")
	}
	if _, ok := d.ByID("nope"); ok {
		t.Fatal("ByID miss")
	}
	if _, ok := d.ByName("  my PROJECT"); !ok {
		t.Fatal("ByName ignores case")
	}
	if id := NewID(); len(id) != len("ws_")+8 || !strings.HasPrefix(id, "ws_") || id == NewID() {
		t.Fatalf("NewID %q", id)
	}
	if id := NewTabID(); len(id) != len("t_")+8 || !strings.HasPrefix(id, "t_") {
		t.Fatalf("NewTabID %q", id)
	}
}

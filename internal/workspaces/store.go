package workspaces

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	schemaVersion = 1
	header        = "# Written by kaze's settings page. Edits here are fine, but comments are not kept.\n"
)

// ErrStale means the file changed since it was loaded, so nothing was written.
var ErrStale = errors.New("workspaces.toml changed since it was loaded; reload it and try again")

// ErrInvalidFile means the file on disk does not load, so it is never replaced.
var ErrInvalidFile = errors.New("The workspaces file has a problem; fix it or delete it before saving.")

// Revision identifies the file bytes a Document was loaded from; zero for a missing file.
type Revision [32]byte

// Path is the workspaces file beside config.toml.
func Path(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "workspaces.toml")
}

type fileDoc struct {
	SchemaVersion int         `toml:"schema_version"`
	Workspaces    []Workspace `toml:"workspaces"`
}

// Problems is the error for a file that parses but is not valid.
type Problems []FieldError

func (p Problems) Error() string {
	var msgs []string
	for i, e := range p {
		if i == 5 {
			msgs = append(msgs, fmt.Sprintf("and %d more", len(p)-i))
			break
		}
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// readCurrent returns the bytes of path (following symlinks) and their revision;
// nil bytes and a zero revision for a missing file.
func readCurrent(path string) ([]byte, Revision, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, Revision{}, nil
	}
	if err != nil {
		return nil, Revision{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return nil, Revision{}, err
	}
	if len(data) > MaxFileSize {
		return nil, Revision{}, fmt.Errorf("%s is larger than %d MiB", path, MaxFileSize>>20)
	}
	return data, sha256.Sum256(data), nil
}

// Load reads and strictly validates the file. A missing file is an empty document.
// An invalid file returns an error and a zero Revision, so Save cannot replace it.
func Load(path string) (Document, Revision, error) {
	data, rev, err := readCurrent(path)
	if err != nil {
		return Document{}, Revision{}, err
	}
	if data == nil {
		return Document{}, Revision{}, nil
	}
	fail := func(err error) (Document, Revision, error) {
		return Document{}, Revision{}, fmt.Errorf("%s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Document{}, rev, nil
	}
	var f fileDoc
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return fail(err)
	}
	if un := md.Undecoded(); len(un) > 0 {
		return fail(fmt.Errorf("unknown setting %q", un[0].String()))
	}
	if f.SchemaVersion != schemaVersion {
		return fail(fmt.Errorf("schema_version must be %d (found %d); this file was written by a different version of kaze", schemaVersion, f.SchemaVersion))
	}
	doc := Document{Workspaces: f.Workspaces}.normalize()
	home, _ := os.UserHomeDir()
	if errs := doc.validate(home, false); len(errs) > 0 {
		return fail(Problems(errs))
	}
	return doc, rev, nil
}

// Save validates d and writes it, unless the file no longer matches prev.
// It returns the new revision.
func Save(path string, d Document, prev Revision) (Revision, error) {
	d = d.normalize()
	if errs := d.validate("", true); len(errs) > 0 {
		return Revision{}, Problems(errs)
	}
	var buf bytes.Buffer
	buf.WriteString(header)
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(fileDoc{SchemaVersion: schemaVersion, Workspaces: d.Workspaces}); err != nil {
		return Revision{}, err
	}
	if buf.Len() > MaxFileSize {
		return Revision{}, fmt.Errorf("workspaces are larger than %d MiB", MaxFileSize>>20)
	}

	// Folders keep a leading ~ as written; it is expanded at load and run.
	// A symlinked file (a dotfiles repo) is updated in place, not replaced by a plain file.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if _, cur, err := readCurrent(path); err != nil {
		return Revision{}, err
	} else if cur != prev {
		if _, _, err := Load(path); err != nil {
			return Revision{}, ErrInvalidFile
		}
		return Revision{}, ErrStale
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Revision{}, err
	}
	if err := replace(path, buf.Bytes()); err != nil {
		return Revision{}, err
	}
	return sha256.Sum256(buf.Bytes()), nil
}

// replace writes data to a temp file beside path and renames it over path,
// keeping the existing permissions.
func replace(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	err = tmp.Chmod(mode)
	if err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}

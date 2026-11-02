package mdoc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"
)

const MaxBytes = 2 << 20

type Stamp struct {
	Mod  time.Time
	Size int64
}
type File struct {
	Data  []byte
	Cut   bool
	Total int64
	Stamp Stamp
}

func StatFile(path string) (Stamp, error) {
	s, e := os.Stat(path)
	if e != nil {
		return Stamp{}, e
	}
	if !s.Mode().IsRegular() {
		return Stamp{}, fmt.Errorf("not a regular file")
	}
	return Stamp{s.ModTime(), s.Size()}, nil
}
func Load(path string) (File, error) {
	f, e := os.Open(path)
	if e != nil {
		return File{}, e
	}
	defer f.Close()
	s, e := f.Stat()
	if e != nil {
		return File{}, e
	}
	if !s.Mode().IsRegular() {
		return File{}, fmt.Errorf("not a regular file")
	}
	data, e := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if e != nil {
		return File{}, e
	}
	cut := len(data) > MaxBytes
	if cut {
		data = data[:MaxBytes]
		if n := bytes.LastIndexByte(data, '\n'); n >= 0 {
			data = data[:n+1]
		}
	}
	return File{Data: data, Cut: cut, Total: s.Size(), Stamp: Stamp{s.ModTime(), s.Size()}}, nil
}

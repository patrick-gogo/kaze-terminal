package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func serve(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadWritesTheFileAndReportsProgress(t *testing.T) {
	body := bytes.Repeat([]byte("k"), 300<<10)
	srv := serve(t, body)
	dst := filepath.Join(t.TempDir(), "a.zip")
	var last, total int64
	err := Download(context.Background(), srv.Client(), srv.URL, dst, 1<<20, func(d, tot int64) { last, total = d, tot })
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, body) || last != int64(len(body)) || total != int64(len(body)) {
		t.Fatalf("got %d bytes, progress %d/%d", len(got), last, total)
	}
}

func TestDownloadOverTheLimitLeavesNoFile(t *testing.T) {
	srv := serve(t, bytes.Repeat([]byte("k"), 4096))
	dst := filepath.Join(t.TempDir(), "a.zip")
	if err := Download(context.Background(), srv.Client(), srv.URL, dst, 1024, nil); err == nil {
		t.Fatal("a body over the limit downloaded")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("file left behind: %v", err)
	}
}

func TestCheckSum(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "Kaze-macOS.zip")
	os.WriteFile(f, []byte("zipdata"), 0o644)
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte("zipdata")))
	sums := []byte("0000  Kaze-Windows-x64.zip\n" + sum + "  Kaze-macOS.zip\n")
	if err := CheckSum(sums, "Kaze-macOS.zip", f); err != nil {
		t.Fatalf("match: %v", err)
	}
	if err := CheckSum([]byte(strings.Replace(string(sums), sum[:4], "ffff", 1)), "Kaze-macOS.zip", f); err == nil {
		t.Fatal("a mismatch passed")
	}
	if err := CheckSum([]byte("0000  Kaze-Windows-x64.zip\n"), "Kaze-macOS.zip", f); err == nil {
		t.Fatal("a missing entry passed")
	}
}

type zipEntry struct {
	name string
	mode os.FileMode
	body string
}

func makeZip(t *testing.T, entries ...zipEntry) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(e.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e.body))
	}
	zw.Close()
	p := filepath.Join(t.TempDir(), "in.zip")
	os.WriteFile(p, buf.Bytes(), 0o644)
	return p
}

func TestUnzipKeepsTheExecBit(t *testing.T) {
	z := makeZip(t,
		zipEntry{"Kaze.app/", os.ModeDir | 0o755, ""},
		zipEntry{"Kaze.app/Contents/MacOS/kaze", 0o755, "binary"},
		zipEntry{"Kaze.app/Contents/Info.plist", 0o644, "plist"})
	dir := t.TempDir()
	if err := Unzip(z, dir, 1<<20); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "Kaze.app", "Contents", "MacOS", "kaze")
	fi, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode()&0o100 == 0 {
		t.Fatalf("mode %v lost the exec bit", fi.Mode())
	}
}

func TestUnzipRefusesPathsOutside(t *testing.T) {
	for _, name := range []string{"../evil", "a/../../evil", "/abs/evil"} {
		z := makeZip(t, zipEntry{name, 0o644, "x"})
		dir := filepath.Join(t.TempDir(), "out")
		os.Mkdir(dir, 0o755)
		if err := Unzip(z, dir, 1<<20); err == nil {
			t.Errorf("%q was unzipped", name)
		}
	}
}

func TestUnzipRefusesSymlinks(t *testing.T) {
	z := makeZip(t, zipEntry{"link", os.ModeSymlink | 0o777, "/etc/passwd"})
	if err := Unzip(z, t.TempDir(), 1<<20); err == nil {
		t.Fatal("a symlink was unzipped")
	}
}

func TestUnzipStopsAtTheLimit(t *testing.T) {
	z := makeZip(t, zipEntry{"big", 0o644, strings.Repeat("a", 8192)})
	if err := Unzip(z, t.TempDir(), 1024); err == nil {
		t.Fatal("an entry over the limit was unzipped")
	}
}

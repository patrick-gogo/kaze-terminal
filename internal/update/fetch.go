package update

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/patrick-gogo/kaze-terminal/internal/version"
)

// Download fetches url into dst, at most limit bytes. progress, if set, is
// called as bytes arrive with the total (-1 when unknown). On any failure dst
// is removed, so a half download never looks finished.
func Download(ctx context.Context, c *http.Client, url, dst string, limit int64, progress func(done, total int64)) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "kaze/"+version.Number)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download answered %s", resp.Status)
	}
	if resp.ContentLength > limit {
		return fmt.Errorf("the download is %d bytes, over the %d byte limit", resp.ContentLength, limit)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(dst)
		}
	}()
	var done int64
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if done+int64(n) > limit {
				return fmt.Errorf("the download is over the %d byte limit", limit)
			}
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if progress != nil {
				progress(done, resp.ContentLength)
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// CheckSum checks file against name's line in sums, a sha256sum style list
// ("<hex>  <name>" per line). A missing line fails, as a mismatch does.
func CheckSum(sums []byte, name, file string) error {
	want := ""
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			want = strings.ToLower(f[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("%s has no checksum for %s", SumsName, name)
	}
	in, err := os.Open(file)
	if err != nil {
		return err
	}
	defer in.Close()
	h := sha256.New()
	if _, err := io.Copy(h, in); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("%s does not match its checksum", name)
	}
	return nil
}

// Unzip extracts zipFile into dir, at most limit bytes in all. It refuses
// entries that would land outside dir and symlinks, and keeps the exec bit.
func Unzip(zipFile, dir string, limit int64) error {
	zr, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer zr.Close()
	var total int64
	for _, e := range zr.File {
		name := e.Name
		if strings.Contains(name, `\`) || path.IsAbs(name) || filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
			return fmt.Errorf("unsafe path in the zip: %q", name)
		}
		clean := path.Clean(name)
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("unsafe path in the zip: %q", name)
		}
		mode := e.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("the zip holds a link: %q", name)
		}
		dst := filepath.Join(dir, filepath.FromSlash(clean))
		if mode.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			continue
		}
		if !mode.IsRegular() {
			return fmt.Errorf("the zip holds an unusual file: %q", name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		n, err := unzipFile(e, dst, mode.Perm()|0o600, limit-total)
		total += n
		if err != nil {
			return err
		}
	}
	return nil
}

func unzipFile(e *zip.File, dst string, perm os.FileMode, room int64) (int64, error) {
	r, err := e.Open()
	if err != nil {
		return 0, err
	}
	defer r.Close()
	w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(w, io.LimitReader(r, room+1))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > room {
		err = errors.New("the zip is larger than allowed")
	}
	return n, err
}

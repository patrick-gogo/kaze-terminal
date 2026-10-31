package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.10", "0.11", -1},
		{"0.11", "0.11.1", -1},
		{"0.11.1", "1.0", -1},
		{"0.9", "0.10", -1},
		{"v0.10", "0.10", 0},
		{"0.10.0", "0.10", 0},
		{"1.0", "0.99", 1},
		{"0.x", "0.1", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := Compare(c.b, c.a); got != -c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

const releaseJSON = `{"tag_name":"v0.11","html_url":"https://github.com/o/r/releases/tag/v0.11",
"assets":[{"name":"Kaze-macOS.zip","browser_download_url":"https://github.com/o/r/releases/download/v0.11/Kaze-macOS.zip"},
{"name":"SHA256SUMS","browser_download_url":"https://github.com/o/r/releases/download/v0.11/SHA256SUMS"}]}`

func TestLatestParsesTheRelease(t *testing.T) {
	var agent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent = r.Header.Get("User-Agent")
		w.Write([]byte(releaseJSON))
	}))
	defer srv.Close()
	rel, err := Latest(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "0.11" || rel.Page != "https://github.com/o/r/releases/tag/v0.11" {
		t.Fatalf("release %+v", rel)
	}
	if rel.Assets["Kaze-macOS.zip"] == "" || rel.Assets["SHA256SUMS"] == "" || len(rel.Assets) != 2 {
		t.Fatalf("assets %v", rel.Assets)
	}
	if !strings.HasPrefix(agent, "kaze/") {
		t.Fatalf("user agent %q", agent)
	}
}

func TestLatestFailsOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := Latest(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("a 403 gave no error")
	}
}

func TestLatestRefusesAHugeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v0.11","html_url":"` + strings.Repeat("a", 2<<20) + `"}`))
	}))
	defer srv.Close()
	if _, err := Latest(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("a 2 MB body was read")
	}
}

func TestAssetNames(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}:  "Kaze-macOS.zip",
		{"darwin", "amd64"}:  "Kaze-macOS.zip",
		{"windows", "amd64"}: "Kaze-Windows-x64.zip",
		{"windows", "arm64"}: "Kaze-Windows-arm64.zip",
		{"linux", "amd64"}:   "",
		{"windows", "386"}:   "",
	}
	for p, want := range cases {
		if got := AssetName(p[0], p[1]); got != want {
			t.Errorf("AssetName(%s, %s) = %q, want %q", p[0], p[1], got, want)
		}
	}
}

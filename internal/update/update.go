// Package update finds newer kaze releases on GitHub and installs them in place.
// Everything that reaches the network or replaces files lives here, with its
// URLs, client and paths passed in, so tests never touch the real ones.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/patrick-gogo/kaze-terminal/internal/version"
)

// LatestURL is GitHub's latest-release endpoint for kaze. It skips drafts and prereleases.
const LatestURL = "https://api.github.com/repos/patrick-gogo/kaze-terminal/releases/latest"

// SumsName is the checksum list each release carries.
const SumsName = "SHA256SUMS"

// maxReleaseJSON bounds the release lookup's body.
const maxReleaseJSON = 1 << 20

// Release is one published kaze release.
type Release struct {
	Version string            // "0.11", without the tag's v
	Page    string            // the release page, for What's new and manual downloads
	Assets  map[string]string // file name to download URL
}

// Compare orders two versions numerically, part by part: -1, 0 or 1. A leading
// v is ignored and a missing part counts as 0, so 0.10 equals 0.10.0. A part
// that is not a number sorts below every number.
func Compare(a, b string) int {
	pa, pb := parts(a), parts(b)
	for i := 0; i < max(len(pa), len(pb)); i++ {
		x, y := 0, 0
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

func parts(v string) []int {
	var out []int
	for _, p := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			n = -1
		}
		out = append(out, n)
	}
	return out
}

// Latest asks url (LatestURL outside tests) for the newest release.
func Latest(ctx context.Context, c *http.Client, url string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("User-Agent", "kaze/"+version.Number)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseJSON+1))
	if err != nil {
		return Release{}, err
	}
	if len(body) > maxReleaseJSON {
		return Release{}, errors.New("the release answer is too large")
	}
	var raw struct {
		Tag    string `json:"tag_name"`
		Page   string `json:"html_url"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Release{}, fmt.Errorf("reading the release: %w", err)
	}
	if !strings.HasPrefix(raw.Tag, "v") {
		return Release{}, fmt.Errorf("unexpected release tag %q", raw.Tag)
	}
	rel := Release{Version: strings.TrimPrefix(raw.Tag, "v"), Page: raw.Page, Assets: map[string]string{}}
	for _, a := range raw.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}

// AssetName is the release zip for a platform, "" when kaze ships none for it.
func AssetName(goos, goarch string) string {
	switch {
	case goos == "darwin" && (goarch == "arm64" || goarch == "amd64"):
		return "Kaze-macOS.zip"
	case goos == "windows" && goarch == "amd64":
		return "Kaze-Windows-x64.zip"
	case goos == "windows" && goarch == "arm64":
		return "Kaze-Windows-arm64.zip"
	}
	return ""
}

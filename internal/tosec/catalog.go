package tosec

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// BaseURL is tosecdev.org; tests point the client at an httptest server.
const BaseURL = "https://www.tosecdev.org"

// UserAgent identifies us to the site.
const UserAgent = "romcure (+local ROM library manager)"

// Release is one published DAT pack.
type Release struct {
	Date        string // "2025-03-13", the pack's version
	CategoryURL string // page listing the pack
	PackURL     string // direct download of the Complete pack
}

var (
	categoryLink = regexp.MustCompile(`href="(/downloads/category/(\d+)-(\d{4}-\d{2}-\d{2}))"`)
	packLink     = regexp.MustCompile(`href="([^"]*\?download=\d+:tosec-dat-pack-complete[^"]*)"`)
)

// maxPage bounds an HTML page we read.
const maxPage = 4 << 20

func get(ctx context.Context, c *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: HTTP %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPage))
	return string(b), err
}

// ParseLatest picks the newest release category from the datfiles index
// page (highest category id).
func ParseLatest(indexHTML string) (id int, date, path string, ok bool) {
	for _, m := range categoryLink.FindAllStringSubmatch(indexHTML, -1) {
		n, _ := strconv.Atoi(m[2])
		if n > id {
			id, date, path, ok = n, m[3], m[1], true
		}
	}
	return
}

// ParsePackLink finds the "Complete" pack's download link on a release page.
func ParsePackLink(categoryHTML string) (string, bool) {
	m := packLink.FindStringSubmatch(categoryHTML)
	if m == nil {
		return "", false
	}
	return strings.ReplaceAll(m[1], "&amp;", "&"), true
}

// FetchLatest finds the newest release and its pack URL. It errors, rather
// than guessing, if either page doesn't look as expected.
func FetchLatest(ctx context.Context, c *http.Client, baseURL string) (Release, error) {
	index, err := get(ctx, c, baseURL+"/downloads/category/22-datfiles")
	if err != nil {
		return Release{}, err
	}
	_, date, catPath, ok := ParseLatest(index)
	if !ok {
		return Release{}, fmt.Errorf("no TOSEC releases found on the datfiles page; its layout may have changed")
	}
	page, err := get(ctx, c, baseURL+catPath)
	if err != nil {
		return Release{}, err
	}
	link, ok := ParsePackLink(page)
	if !ok {
		return Release{}, fmt.Errorf("no Complete DAT pack link on %s; its layout may have changed", catPath)
	}
	if strings.HasPrefix(link, "/") {
		link = baseURL + link
	}
	return Release{Date: date, CategoryURL: baseURL + catPath, PackURL: link}, nil
}

// Package redump discovers and downloads datfiles and SBI subchannel
// archives from redump.info. See docs/redump-integration-plan.md.
package redump

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// BaseURL is redump.info; tests point the client at an httptest server.
const BaseURL = "https://redump.info"

// UserAgent identifies us to the volunteer-run site.
const UserAgent = "romcure (+local ROM library manager)"

// System is one row of the downloads table.
type System struct {
	Name    string // "Sony - PlayStation" style display name from the page
	Slug    string // URL slug: /datfile/<Slug>
	Version string // timestamp shown on the page; changes when the DAT does
	HasCues bool
	HasSBI  bool
}

// Catalog is the parsed downloads page.
type Catalog struct {
	Systems []System // main table: game datfiles
	BIOS    []System // BIOS datfiles (/bios/<Slug>)
}

// Find returns the game-datfile system with slug (case-insensitive).
func (c *Catalog) Find(slug string) (System, bool) {
	for _, s := range c.Systems {
		if strings.EqualFold(s.Slug, slug) {
			return s, true
		}
	}
	return System{}, false
}

var (
	rowPattern     = regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	cellPattern    = regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)
	linkPattern    = regexp.MustCompile(`href="/(datfile|bios|cues|sbi)/([^"/]+)"`)
	tagPattern     = regexp.MustCompile(`<[^>]*>`)
	biosHeaderMark = `downloads-section-title">BIOS`
)

// ParseCatalog parses the /downloads HTML. The page has no API, so this is
// deliberately strict about what it expects: if it finds no systems at
// all it returns an error rather than an empty catalog, so a layout change
// surfaces instead of silently looking like "nothing to download".
func ParseCatalog(page string) (*Catalog, error) {
	gamesPart, biosPart := page, ""
	if i := strings.Index(page, biosHeaderMark); i >= 0 {
		gamesPart, biosPart = page[:i], page[i:]
	}

	c := &Catalog{}
	c.Systems = parseRows(gamesPart, "datfile")
	c.BIOS = parseRows(biosPart, "bios")
	if len(c.Systems) == 0 {
		return nil, fmt.Errorf("no systems found on the downloads page; its layout may have changed")
	}
	return c, nil
}

func parseRows(section, mainKind string) []System {
	var out []System
	for _, row := range rowPattern.FindAllStringSubmatch(section, -1) {
		cells := cellPattern.FindAllStringSubmatch(row[1], -1)
		if len(cells) < 3 {
			continue // header row
		}
		s := System{
			Name:    clean(cells[0][1]),
			Version: clean(cells[1][1]),
		}
		for _, l := range linkPattern.FindAllStringSubmatch(cells[2][1], -1) {
			switch l[1] {
			case mainKind:
				s.Slug = l[2]
			case "cues":
				s.HasCues = true
			case "sbi":
				s.HasSBI = true
			}
		}
		if s.Slug != "" && s.Name != "" {
			out = append(out, s)
		}
	}
	return out
}

func clean(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagPattern.ReplaceAllString(s, " ")))
}

// maxCatalogBytes bounds the page we're willing to read.
const maxCatalogBytes = 4 << 20

// FetchCatalog downloads and parses the downloads page from baseURL.
func FetchCatalog(ctx context.Context, client *http.Client, baseURL string) (*Catalog, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/downloads", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch redump catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch redump catalog: HTTP %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogBytes))
	if err != nil {
		return nil, fmt.Errorf("read redump catalog: %w", err)
	}
	return ParseCatalog(string(body))
}

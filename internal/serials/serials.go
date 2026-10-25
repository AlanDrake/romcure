// Package serials reads the serial → title lists of the libretro-database
// project (CC BY-SA 4.0), downloaded on demand. PlayStation and PlayStation 2
// discs carry only a serial (discid.ReadPlayStation); these lists turn it
// into the game's name when no DAT matched the disc's checksums.
package serials

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// DefaultBaseURL is where libretro-database keeps its Redump-derived lists.
const DefaultBaseURL = "https://raw.githubusercontent.com/libretro/libretro-database/master/metadat/redump/"

// files names the list of each system that has one.
var files = map[string]string{
	"psx": "Sony - PlayStation.dat",
	"ps2": "Sony - PlayStation 2.dat",
}

// Has reports whether a serial list exists for systemID.
func Has(systemID string) bool { _, ok := files[systemID]; return ok }

// Entry is one serial and the game it belongs to.
type Entry struct {
	Serial string
	Name   string
}

// List is a parsed serial list.
type List struct {
	Version string
	Entries []Entry
}

var (
	field  = regexp.MustCompile(`^\s*(name|serial|version)\s+"([^"]*)"`)
	gameAt = regexp.MustCompile(`^\s*game\s*\(`)
	romAt  = regexp.MustCompile(`^\s*rom\s*\(`)
)

// Parse reads a clrmamepro-format list: the game's name and its serial(s)
// ("SLUS-00594", or several separated by commas).
func Parse(r io.Reader) (List, error) {
	var out List
	var name, serial string
	inGame := false
	flush := func() {
		if name != "" && serial != "" {
			for _, s := range strings.Split(serial, ",") {
				if s = strings.TrimSpace(s); s != "" {
					out.Entries = append(out.Entries, Entry{Serial: strings.ToUpper(s), Name: name})
				}
			}
		}
		name, serial = "", ""
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case gameAt.MatchString(line):
			flush()
			inGame = true
		case romAt.MatchString(line):
			// a rom line repeats the serial; the game's own field is enough
		default:
			m := field.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			switch {
			case m[1] == "version" && !inGame:
				out.Version = m[2]
			case m[1] == "name" && inGame && name == "":
				name = m[2]
			case m[1] == "serial" && inGame && serial == "":
				serial = m[2]
			}
		}
	}
	flush()
	return out, sc.Err()
}

// Fetch downloads and parses the list of systemID from baseURL ("" = the
// default).
func Fetch(ctx context.Context, client *http.Client, baseURL, systemID string) (List, error) {
	name, ok := files[systemID]
	if !ok {
		return List{}, fmt.Errorf("no serial list for %s", systemID)
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+url.PathEscape(name), nil)
	if err != nil {
		return List{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return List{}, fmt.Errorf("download the %s serial list: %w", systemID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return List{}, fmt.Errorf("download the %s serial list: %s", systemID, resp.Status)
	}
	return Parse(resp.Body)
}

package redump

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "redump", "downloads.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseCatalog(t *testing.T) {
	c, err := ParseCatalog(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Systems) != 4 || len(c.BIOS) != 1 {
		t.Fatalf("systems=%d bios=%d, want 4 and 1", len(c.Systems), len(c.BIOS))
	}
	psx, ok := c.Find("psx")
	if !ok || psx.Name != "Sony PlayStation" || !psx.HasSBI || !psx.HasCues || psx.Version == "" {
		t.Fatalf("psx = %+v", psx)
	}
	if ps2, _ := c.Find("PS2"); ps2.HasSBI {
		t.Error("PS2 has no SBI archive")
	}
	if c.BIOS[0].Slug != "PSX" {
		t.Errorf("bios = %+v", c.BIOS[0])
	}
}

func TestParseCatalogLayoutChangeIsAnError(t *testing.T) {
	if _, err := ParseCatalog("<html><body>maintenance</body></html>"); err == nil {
		t.Fatal("expected error for a page with no systems")
	}
}

func TestFetchCatalogSendsUserAgent(t *testing.T) {
	page := fixture(t)
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		if r.URL.Path != "/downloads" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(page))
	}))
	defer srv.Close()
	c, err := FetchCatalog(context.Background(), srv.Client(), srv.URL)
	if err != nil || len(c.Systems) == 0 {
		t.Fatalf("err=%v catalog=%+v", err, c)
	}
	if ua != UserAgent {
		t.Errorf("User-Agent = %q", ua)
	}
}

func TestFetchCatalogHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", http.StatusServiceUnavailable) }))
	defer srv.Close()
	if _, err := FetchCatalog(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("expected error on 503")
	}
}

func TestSystemIDRoundTrip(t *testing.T) {
	id, ok := SystemID("ps2")
	if !ok || id != "ps2" {
		t.Fatal("ps2")
	}
	if s, _ := Slug("saturn"); s != "SS" {
		t.Errorf("Slug(saturn) = %q", s)
	}
}

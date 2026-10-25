package notices

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// goModRequires lists the module paths in go.mod's require blocks.
func goModRequires(t *testing.T) []string {
	t.Helper()
	f, err := os.Open("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "require ("):
			in = true
		case line == ")":
			in = false
		case in && line != "":
			out = append(out, strings.Fields(line)[0])
		case strings.HasPrefix(line, "require "):
			out = append(out, strings.Fields(line)[1])
		}
	}
	return out
}

// A new or changed dependency without regenerated notices must fail the build
// (spec §7: fail the release when a component changes without its notice).
func TestEveryGoModuleHasANotice(t *testing.T) {
	have := map[string]Entry{}
	for _, e := range All() {
		have[e.Name] = e
	}
	for _, m := range goModRequires(t) {
		e, ok := have[m]
		if !ok {
			t.Errorf("go.mod requires %s but the notices do not list it: run `go run ./tools/gen-notices`", m)
			continue
		}
		if strings.TrimSpace(e.Text) == "" || e.License == "" {
			t.Errorf("%s has no license text or name", m)
		}
	}
	// The version shown must be the one in go.mod.
	data, _ := os.ReadFile("../../go.mod")
	for _, e := range All() {
		if e.Kind == "go" && !strings.Contains(string(data), e.Name+" "+e.Version) {
			t.Errorf("notice for %s says %s, which is not what go.mod pins: regenerate", e.Name, e.Version)
		}
	}
}

func TestBundledToolsAndTheAppAreListed(t *testing.T) {
	for _, name := range []string{"RomCure", "chdman (MAME)", "dolphin-tool (Dolphin)", "7-Zip", "libretro-database (PlayStation serial lists, No-Intro DATs)", "Inter (font)", "@wailsio/runtime"} {
		e, ok := Find(name)
		if !ok {
			t.Errorf("%s missing from the notices", name)
			continue
		}
		if strings.TrimSpace(e.Text) == "" {
			t.Errorf("%s has no license text", name)
		}
	}
	if e, _ := Find("chdman (MAME)"); !strings.Contains(e.License, "GPL") || e.Kind != "tool" {
		t.Errorf("chdman entry = %+v", e)
	}
}

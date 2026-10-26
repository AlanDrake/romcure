// Command gen-notices writes the third-party license data the app shows
// (internal/notices/data.json) and the THIRD_PARTY_NOTICES.txt shipped with
// installers (spec §7). Run it from the repository root after changing
// dependencies or bundled tools:
//
//	go run ./tools/gen-notices
//
// internal/notices has a test that fails when go.mod names a module this
// output does not cover, so a release cannot ship stale notices.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type entry struct {
	Name    string
	Version string
	License string
	URL     string
	Kind    string // app | go | js | font | tool | data
	Note    string
	Text    string
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-notices:", err)
		os.Exit(1)
	}
}

func read(path string) string {
	b, err := os.ReadFile(path)
	must(err)
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// requiredModules returns the module paths named in go.mod's require blocks.
func requiredModules() map[string]bool {
	out := map[string]bool{}
	in := false
	sc := bufio.NewScanner(strings.NewReader(read("go.mod")))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "require ("):
			in = true
		case line == ")":
			in = false
		case in && line != "":
			out[strings.Fields(line)[0]] = true
		case strings.HasPrefix(line, "require "):
			out[strings.Fields(line)[1]] = true
		}
	}
	return out
}

type goModule struct {
	Path, Version, Dir string
}

func listModules() []goModule {
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	must(cmd.Run())
	var out []goModule
	dec := json.NewDecoder(&buf)
	for dec.More() {
		var m goModule
		must(dec.Decode(&m))
		out = append(out, m)
	}
	return out
}

var licenseFile = regexp.MustCompile(`(?i)^(licen[cs]e|copying|unlicense|notice)([-._].*)?$`)

// licenseFiles reads the license files of a module directory.
func licenseFiles(dir string) []string {
	ents, _ := os.ReadDir(dir)
	var parts []string
	for _, e := range ents {
		if e.IsDir() || !licenseFile.MatchString(e.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil {
			parts = append(parts, strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n")))
		}
	}
	return parts
}

// licenseText joins a module's license files.
func licenseText(dir string) string { return strings.Join(licenseFiles(dir), "\n\n---\n\n") }

// identifyAll names the license of each file and joins the distinct names
// ("BSD-3-Clause + Apache-2.0" for a module that ships both).
func identifyAll(dir string) string {
	var names []string
	seen := map[string]bool{}
	for _, t := range licenseFiles(dir) {
		n := identify(t)
		if n != "see text" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "see text"
	}
	return strings.Join(names, " + ")
}

// identify names a license from its text; "see text" when unsure.
func identify(text string) string {
	t := strings.Join(strings.Fields(strings.ToLower(text)), " ") // wrapped lines compare equal
	switch {
	case strings.Contains(t, "apache license") && strings.Contains(t, "version 2.0"):
		return "Apache-2.0"
	case strings.Contains(t, "mozilla public license"):
		return "MPL-2.0"
	case strings.Contains(t, "gnu lesser general public license"):
		return "LGPL"
	case strings.Contains(t, "gnu general public license"):
		return "GPL"
	case strings.Contains(t, "this is free and unencumbered software"):
		return "Unlicense"
	case strings.Contains(t, "permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(t, "redistribution and use in source and binary forms"):
		if strings.Contains(t, "neither the name") || strings.Contains(t, "endorse or promote") {
			return "BSD-3-Clause"
		}
		return "BSD-2-Clause"
	case strings.Contains(t, "permission to use, copy, modify, and/or distribute") || strings.Contains(t, "permission to use, copy, modify, and distribute this software for any purpose with or without fee") || strings.Contains(t, "isc license"):
		return "ISC"
	}
	return "see text"
}

func toolPin(name string) string {
	for _, l := range strings.Split(read("tools/versions.txt"), "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && f[0] == name {
			return f[1]
		}
	}
	return ""
}

func main() {
	var entries []entry
	gpl3 := strings.TrimSpace(read("LICENSE"))

	entries = append(entries, entry{
		Name: "RomCure", License: "GPL-3.0", Kind: "app",
		URL: "https://github.com/AlanDrake/romcure", Text: gpl3,
		Note: "This application. Its complete source is at the address above.",
	})

	need := requiredModules()
	var mods []goModule
	for _, m := range listModules() {
		if need[m.Path] {
			mods = append(mods, m)
		}
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Path < mods[j].Path })
	var wailsText string
	for _, m := range mods {
		text := licenseText(m.Dir)
		if text == "" {
			fmt.Fprintf(os.Stderr, "gen-notices: warning: no license file found for %s %s\n", m.Path, m.Version)
		}
		if m.Path == "github.com/wailsapp/wails/v3" {
			wailsText = text
		}
		url := "https://" + m.Path
		if strings.HasPrefix(m.Path, "golang.org/x/") {
			url = "https://pkg.go.dev/" + m.Path
		}
		entries = append(entries, entry{Name: m.Path, Version: m.Version, License: identifyAll(m.Dir), URL: url, Kind: "go", Text: text})
	}

	// Compiled into the web interface.
	entries = append(entries, entry{
		Name: "@wailsio/runtime", Version: "3.0.0-beta.30", License: "MIT", URL: "https://github.com/wailsapp/wails", Kind: "js",
		Note: "The Wails project's license, which covers its JavaScript runtime.", Text: wailsText,
	})
	if t := licenseText("frontend/node_modules/svelte"); t != "" {
		entries = append(entries, entry{Name: "svelte", Version: "5", License: identify(t), URL: "https://svelte.dev", Kind: "js", Text: t,
			Note: "Its runtime is compiled into the interface."})
	}
	entries = append(entries, entry{
		Name: "Inter (font)", License: "OFL-1.1", URL: "https://github.com/rsms/inter", Kind: "font",
		Text: strings.TrimSpace(read("frontend/Inter Font License.txt")),
	})

	// Bundled converters: separate programs, started as subprocesses only.
	gplNote := "GPL-2.0-or-later. Run only as a separate program; the app is not linked with it. " +
		"The full text of the GNU GPL version 3 (a later version, which this license allows) follows; " +
		"version 2 is at https://www.gnu.org/licenses/old-licenses/gpl-2.0.html. " +
		"RomCure ships the project's own unmodified release. The complete source of that exact version is attached to every RomCure release (https://github.com/AlanDrake/romcure/releases), is published by the project (the address and version are given here), and RomCure will provide it on request (see \"Source code\")."
	entries = append(entries, entry{
		Name: "chdman (MAME)", Version: toolPin("chdman"), License: "GPL-2.0-or-later", URL: "https://github.com/mamedev/mame", Kind: "tool",
		Note: gplNote + " Most MAME files are individually BSD-3-Clause. Official release " + toolPin("chdman") + " (source: https://github.com/mamedev/mame/releases/tag/" + toolPin("chdman") + ").", Text: gpl3,
	})
	entries = append(entries, entry{
		Name: "dolphin-tool (Dolphin)", Version: toolPin("dolphin"), License: "GPL-2.0-or-later", URL: "https://github.com/dolphin-emu/dolphin", Kind: "tool",
		Note: gplNote + " Official release " + toolPin("dolphin") + " (source: https://github.com/dolphin-emu/dolphin/releases/tag/" + toolPin("dolphin") + ", or dolphin-emu.org/download).", Text: gpl3,
	})

	entries = append(entries, entry{
		Name: "7-Zip", Version: toolPin("7zip"), License: "LGPL-2.1-or-later (with the unRAR restriction)", URL: "https://www.7-zip.org", Kind: "tool",
		Note: "Reads RAR archives. Run only as a separate program (7z.exe with 7z.dll, or 7zz); the app is not linked with it and does not change it. " +
			"7-Zip's own license notice comes first, then the GNU Lesser General Public License version 2.1. " +
			"Its source is attached to every RomCure release (https://github.com/AlanDrake/romcure/releases), and is at https://github.com/ip7z/7zip (the release named in the version above) and https://www.7-zip.org, " +
			"and RomCure will provide it on request (see \"Source code\"). The unRAR restriction means the RAR code may not be used to build a RAR-compatible archiver; RomCure only reads archives.",
		Text: strings.TrimSpace(read("tools/licenses/7zip-License.txt")) + "\n\n\n" + strings.TrimSpace(read("tools/licenses/LGPL-2.1.txt")),
	})

	entries = append(entries, entry{
		Name: "libretro-database (PlayStation serial lists, No-Intro DATs)", License: "CC-BY-SA-4.0", URL: "https://github.com/libretro/libretro-database", Kind: "data",
		Note: "The serial to game name lists for PlayStation and PlayStation 2 (metadat/redump), used to name discs no database matched, " +
			"and its copies of the No-Intro DATs (metadat/no-intro, compiled by No-Intro, https://no-intro.org), used to identify cartridge games. " +
			"Downloaded with the game databases when you ask for it. Not shipped with the app; not modified.",
		Text: strings.TrimSpace(read("tools/licenses/CC-BY-SA-4.0.txt")),
	})

	data, err := json.MarshalIndent(entries, "", " ")
	must(err)
	must(os.WriteFile(filepath.Join("internal", "notices", "data.json"), append(data, '\n'), 0o644))

	var txt strings.Builder
	txt.WriteString("THIRD-PARTY NOTICES\n===================\n\nRomCure includes or runs the components below. Their licenses follow.\nGenerated by tools/gen-notices; do not edit by hand.\n\n")
	for _, e := range entries {
		fmt.Fprintf(&txt, "%s\n%s\n", e.Name, strings.Repeat("-", len(e.Name)))
		if e.Version != "" {
			fmt.Fprintf(&txt, "Version: %s\n", e.Version)
		}
		fmt.Fprintf(&txt, "License: %s\nWebsite: %s\n", e.License, e.URL)
		if e.Note != "" {
			fmt.Fprintf(&txt, "%s\n", e.Note)
		}
		fmt.Fprintf(&txt, "\n%s\n\n\n", e.Text)
	}
	must(os.WriteFile("THIRD_PARTY_NOTICES.txt", []byte(txt.String()), 0o644))
	fmt.Printf("wrote %d components\n", len(entries))
}

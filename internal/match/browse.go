package match

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"romcure/internal/discid"
	"romcure/internal/discsheet"

	"romcure/internal/store"
)

// Item is one browsable entry: a raw file, or one member of an archive.
type Item struct {
	// Path is the file's path; for an archive member, the archive's path
	// (the member lives inside it, not at its own filesystem path).
	Path string
	// MemberName is "" for a raw file, or the member's name within the
	// archive otherwise.
	MemberName string
	Size       int64
	// ContentHash is the CRC32 hex used to match this item (the raw
	// file's own hash, or the archive member's). Always present: the
	// scanner computes it for every raw file and every archive member
	// header. Used downstream as a library.GameKey input.
	ContentHash string
	Result      Result
	// Bundled marks an archive item standing for several program files (bundleProgramFiles).
	Bundled bool

	disc discid.Info // what the disc header says, if anything (nameFromDiscHeaders)
}

// Counts tallies a Browse result by confidence tier.
type Counts struct {
	DATVerified  int
	Heuristic    int
	Unidentified int
}

// add tallies one Result into c.
func (c *Counts) add(confidence Confidence) {
	switch confidence {
	case ConfidenceDATVerified:
		c.DATVerified++
	case ConfidenceHeuristic:
		c.Heuristic++
	case ConfidenceUnidentified:
		c.Unidentified++
	}
}

// Total returns the sum of all three tiers.
func (c Counts) Total() int {
	return c.DATVerified + c.Heuristic + c.Unidentified
}

// Browse lists every scanned file (and archive member) under root,
// matched against systemID's loaded DAT entries, or heuristically when
// none matched (or none are loaded for systemID at all). This is the
// pre-canonical, pre-1G1R "browsable library" spec §2.6 describes —
// the Rule Engine (M2) is what turns this into canonical game records.
func Browse(db *store.DB, root, systemID string) ([]Item, Counts, error) {
	files, err := db.ListFilesUnderRoot(root)
	if err != nil {
		return nil, Counts{}, fmt.Errorf("browse %s: %w", root, err)
	}

	var items []Item
	var counts Counts

	for _, f := range files {
		switch f.Kind {
		case "zip", "sevenzip", "rar":
			members, err := db.ListArchiveMembers(f.ID)
			if err != nil {
				return nil, Counts{}, fmt.Errorf("browse %s: %w", root, err)
			}
			names := make([]string, len(members))
			for i, m := range members {
				names[i] = m.MemberName
			}
			stray := notGameMembers(systemID, names)
			first := len(items)
			for _, m := range members {
				if stray[m.MemberName] || isSidecar(systemID, m.MemberName) {
					continue
				}
				res, err := matchFile(db, systemID, m.MemberName, hashOf{m.CRC32, m.Size}, hashOf{m.AltCRC32, m.AltSize})
				if err != nil {
					return nil, Counts{}, fmt.Errorf("browse %s: match %s in %s: %w", root, m.MemberName, f.Path, err)
				}
				items = append(items, Item{Path: f.Path, MemberName: m.MemberName, Size: m.Size, ContentHash: m.CRC32, Result: res,
					disc: discid.Info{ID: m.DiscID.String, Title: m.DiscTitle, Disc: m.DiscNumber}})
				counts.add(res.Confidence)
			}
			if f.Kind != "rar" {
				items = bundleProgramFiles(systemID, f, items, first)
			}

		default: // "raw"
			if isSidecar(systemID, f.Path) {
				continue
			}
			if strings.EqualFold(filepath.Ext(f.Path), ".chd") {
				tracks, err := db.CHDTracks(f)
				if err != nil {
					return nil, Counts{}, fmt.Errorf("browse %s: chd tracks for %s: %w", root, f.Path, err)
				}
				if res, ok, err := MatchTracks(db, systemID, tracks); err != nil {
					return nil, Counts{}, err
				} else if ok {
					items = append(items, Item{Path: f.Path, Size: f.Size, ContentHash: TracksContentHash(tracks), Result: res})
					counts.add(res.Confidence)
					continue
				}
			}
			crc32 := ""
			if f.CRC32.Valid {
				crc32 = f.CRC32.String
			}
			res, err := matchFile(db, systemID, filepath.Base(f.Path), hashOf{crc32, f.Size}, hashOf{f.AltCRC32, f.AltSize})
			if err != nil {
				return nil, Counts{}, fmt.Errorf("browse %s: match %s: %w", root, f.Path, err)
			}
			items = append(items, Item{Path: f.Path, Size: f.Size, ContentHash: crc32, Result: res,
				disc: discid.Info{ID: f.DiscID.String, Title: f.DiscTitle, Disc: f.DiscNumber}})
			counts.add(res.Confidence)
		}
	}

	if err := resolveSheets(db, systemID, items); err != nil {
		return nil, Counts{}, err
	}
	if err := nameFromDiscHeaders(db, systemID, items); err != nil {
		return nil, Counts{}, err
	}
	// Sheet resolution and disc headers can promote items, so recount from the final list.
	counts = Counts{}
	for _, it := range items {
		counts.add(it.Result.Confidence)
	}
	return items, counts, nil
}

// nameFromDiscHeaders names the discs that neither a DAT nor their file name
// identified (scene names such as "scn-gzlt") from their disc header: a
// GameCube/Wii header has the title; a PlayStation disc has only its serial,
// looked up in the downloaded serial list. A cue/gdi sheet takes the header
// of its first track. A file name with tags is kept: it usually says more
// (languages, revision) than the header does.
func nameFromDiscHeaders(db *store.DB, systemID string, items []Item) error {
	for i := range items {
		it := &items[i]
		if discid.IsCartridgeSystem(systemID) {
			if err := nameFromCartridgeCode(db, systemID, it); err != nil {
				return err
			}
			continue
		}
		if it.Result.Confidence != ConfidenceUnidentified {
			continue
		}
		disc := it.disc
		if disc.ID == "" && isSheet(it) {
			disc = trackDisc(items, it)
		}
		if disc.ID == "" {
			continue
		}
		name := disc.Name()
		if discid.IsPlayStationSerial(disc.ID) {
			n, ok, err := db.SerialName(systemID, disc.ID)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			name = n
		}
		if name == "" {
			continue
		}
		h := MatchHeuristic(name + ".iso") // the extension keeps a dot in the title from being cut
		h.Confidence = ConfidenceHeuristic
		// The disc number goes on the title, which keeps it (tags are cut from it),
		// so a game's second disc is not taken for another version of the first.
		if m := discTagIn.FindString(name); m != "" {
			h.Title += " " + m // "Final Fantasy VII (USA) (Disc 1)": the list's own disc tag
		} else {
			h.Title += disc.DiscTag()
		}
		it.Result = h
	}
	return nil
}

var discTagIn = regexp.MustCompile(`\(Disc \d+\)`)

func isSheet(it *Item) bool {
	name := it.MemberName
	if name == "" {
		name = it.Path
	}
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".cue" || ext == ".gdi"
}

// trackDisc finds the disc header of a sheet's track: an item next to it (in
// the same folder, or the same archive) whose name starts with the sheet's
// ("Game.cue" -> "Game.bin", "Game (Track 1).bin").
func trackDisc(items []Item, sheet *Item) discid.Info {
	dir, base := filepath.Dir(sheet.Path), sheet.Path
	if sheet.MemberName != "" {
		dir, base = sheet.Path+"|"+filepath.Dir(sheet.MemberName), sheet.MemberName
	}
	stem := strings.ToLower(strings.TrimSuffix(filepath.Base(base), filepath.Ext(base)))
	for i := range items {
		t := &items[i]
		if t.disc.ID == "" || t == sheet {
			continue
		}
		tdir, tbase := filepath.Dir(t.Path), t.Path
		if t.MemberName != "" {
			tdir, tbase = t.Path+"|"+filepath.Dir(t.MemberName), t.MemberName
		}
		if tdir == dir && strings.HasPrefix(strings.ToLower(filepath.Base(tbase)), stem) {
			return t.disc
		}
	}
	return discid.Info{}
}

// sidecarExtensions are files that live beside games but aren't games:
// subchannel data, artwork, notes, saves, playlists, shortcuts, checksums.
// They must never become library items (they would otherwise surface as
// "heuristic" games).
var sidecarExtensions = map[string]bool{
	".sbi": true, ".sub": true, ".txt": true, ".nfo": true, ".jpg": true, ".jpeg": true,
	".png": true, ".m3u": true, ".lpl": true, ".srm": true, ".sav": true, ".state": true,
	".xml": true, ".dat": true, ".pdf": true, ".db": true, ".ini": true, ".cfg": true,
	".url": true, ".lnk": true, ".htm": true, ".html": true, ".md": true, ".log": true,
	".diz": true, ".rtf": true, ".doc": true, ".docx": true, ".bak": true, ".tmp": true,
	".gif": true, ".bmp": true, ".ico": true, ".webp": true, ".json": true, ".csv": true,
	".sfv": true, ".md5": true, ".sha1": true, ".crc": true, ".torrent": true, ".nzb": true,
	".par2": true, ".mp3": true, ".ogg": true, ".mp4": true, ".avi": true, ".mkv": true,
	// patches: applied to a ROM, not games themselves
	".ips": true, ".ups": true, ".bps": true, ".ppf": true, ".xdelta": true, ".aps": true,
}

// programExtensions are programs and scripts: tools users keep next to their
// games (chdman.exe, a .bat that converts discs) are not games, except on
// systems whose games are programs.
var programExtensions = map[string]bool{
	".exe": true, ".com": true, ".bat": true, ".cmd": true, ".sh": true, ".dll": true,
	".msi": true, ".py": true, ".jar": true, ".ps1": true, ".vbs": true,
}

// programSystems are systems whose games are themselves programs.
var programSystems = map[string]bool{"dos": true, "pc": true, "windows": true, "msx": true, "amstradcpc": true}

// IsSidecar reports whether a file name is something that lives beside games
// but is not one for systemID (see isSidecar).
func IsSidecar(systemID, name string) bool { return isSidecar(systemID, name) }

// isSidecar reports whether a file (or archive member) name is something
// that lives beside games but is not one for systemID.
func isSidecar(systemID, name string) bool {
	base := strings.ToLower(filepath.Base(name))
	switch base {
	case ".ds_store", "thumbs.db", "desktop.ini":
		return true
	}
	if strings.HasPrefix(base, "._") { // macOS resource-fork leftovers
		return true
	}
	ext := filepath.Ext(base)
	return sidecarExtensions[ext] || (programExtensions[ext] && !programSystems[systemID])
}

// resolveSheets gives a raw .cue/.gdi the DAT identity of its tracks. A
// user's cue text rarely equals Redump's byte for byte, so the sheet itself
// is usually not hash-matched even though every track is; the disc is
// identified by its tracks (same rule as a CHD's extracted tracks).
func resolveSheets(db *store.DB, systemID string, items []Item) error {
	byPath := make(map[string]int, len(items))
	for i, it := range items {
		if it.MemberName == "" {
			byPath[strings.ToLower(filepath.Clean(it.Path))] = i
		}
	}
	for i, it := range items {
		ext := strings.ToLower(filepath.Ext(it.Path))
		if it.MemberName != "" || (ext != ".cue" && ext != ".gdi") || it.Result.Confidence == ConfidenceDATVerified {
			continue
		}
		files, err := discsheet.ReferencedFiles(it.Path)
		if err != nil || len(files) == 0 {
			continue
		}
		var tracks []store.CHDTrack
		complete := true
		for n, f := range files {
			j, ok := byPath[strings.ToLower(filepath.Clean(filepath.Join(filepath.Dir(it.Path), f)))]
			if !ok {
				complete = false
				break
			}
			tracks = append(tracks, store.CHDTrack{Ordinal: n + 1, Name: f, Size: items[j].Size, CRC32: items[j].ContentHash})
		}
		if !complete {
			continue
		}
		res, ok, err := MatchTracks(db, systemID, tracks)
		if err != nil {
			return err
		}
		if ok {
			items[i].Result = res
			items[i].ContentHash = TracksContentHash(tracks)
		}
	}
	return nil
}

// nameFromCartridgeCode names a GBA, NDS or N64 dump from the game code in its
// header, through the serial the system's DAT lists for each game. Dumps no DAT
// matched by CRC (scene releases with a trimmed or patched ROM) are the target,
// and a DAT name beats the scene file-name tags, so heuristic items are renamed
// too; a DAT-verified item never is. A code the DAT does not list leaves the
// item alone: the raw header title is too cryptic ("EXQUEST", "MARIOKART") to use.
func nameFromCartridgeCode(db *store.DB, systemID string, it *Item) error {
	if it.Result.Confidence == ConfidenceDATVerified || it.disc.ID == "" {
		return nil
	}
	name := it.MemberName
	if name == "" {
		name = filepath.Base(it.Path)
	}
	if hackMarker.MatchString(name) {
		return nil // a hack or translation keeps its base game's code, not its identity
	}
	entries, err := db.DatEntriesBySerial(systemID, it.disc.ID)
	if err != nil || len(entries) == 0 {
		return err
	}
	game := pickSerialName(entries, it.Size)
	h := MatchHeuristic(game + ".rom") // the extension keeps a dot in the title from being cut
	h.Confidence = ConfidenceHeuristic
	it.Result = h
	return nil
}

// hackMarker finds the GoodTools and TOSEC marks of a modified ROM: [h], [h1],
// [h Name], [T+Ita], [T-Eng], [tr ...], (Hack), (Translation ...).
var hackMarker = regexp.MustCompile(`(?i)\[h|\[t[+-]|\[tr|\(hack\)|\(translation`)

// notPreferred are tags of versions that lose to a regular release sharing the
// same serial (re-releases on Virtual Console, prototypes, demos).
var notPreferred = []string{"(Virtual Console)", "(Beta", "(Proto", "(Demo", "(Kiosk", "(Sample", "(Debug"}

// pickSerialName chooses among the DAT games sharing a serial: the one whose
// ROM size equals the dump's first, then a regular release, then an original
// revision, then alphabetical order so the choice is stable.
func pickSerialName(entries []store.DatSerialEntry, size int64) string {
	rank := func(e store.DatSerialEntry) [3]int {
		var r [3]int
		if e.Size != size {
			r[0] = 1
		}
		for _, tag := range notPreferred {
			if strings.Contains(e.GameName, tag) {
				r[1] = 1
			}
		}
		if strings.Contains(e.GameName, "(Rev ") {
			r[2] = 1
		}
		return r
	}
	less := func(a, b [3]int) bool {
		for i := range a {
			if a[i] != b[i] {
				return a[i] < b[i]
			}
		}
		return false
	}
	best := entries[0]
	for _, e := range entries[1:] {
		if re, rb := rank(e), rank(best); re != rb {
			if less(re, rb) {
				best = e
			}
		} else if e.GameName < best.GameName {
			best = e
		}
	}
	return best.GameName
}

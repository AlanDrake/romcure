package match

import (
	"fmt"
	"hash/crc32"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"romcure/internal/store"
)

// bundleSystems are systems where one game can be many program files in one
// archive (a loader and its data files). vic20 and plus4 can join later.
var bundleSystems = map[string]bool{"c64": true}

// programFileExt matches the member types that are pieces of a program: .prg,
// .seq, .usr, .rel and the PC64 family (.p00, .s00, .u00, .r00).
var programFileExt = regexp.MustCompile(`^\.(prg|seq|usr|rel|[psur]\d\d)$`)

// bundleProgramFiles folds the items of one archive (items[first:]) into a
// single item for the archive when it holds two or more unverified program
// files: a C64 game packed as LOADER.P00 plus 00.P00..3E.P00 is one game, not
// 64. The archive itself becomes the game (MemberName ""), so a profile links
// it whole. DAT-verified members and disk or tape images stay as they are.
// RAR archives are not passed here: only zip and 7z are linked as they are.
func bundleProgramFiles(systemID string, f store.FileRecord, items []Item, first int) []Item {
	if !bundleSystems[systemID] {
		return items
	}
	var kept []Item
	var crcs []string
	var size int64
	for _, it := range items[first:] {
		if it.Result.Confidence != ConfidenceDATVerified && programFileExt.MatchString(strings.ToLower(filepath.Ext(it.MemberName))) {
			crcs = append(crcs, it.ContentHash)
			size += it.Size
			continue
		}
		kept = append(kept, it)
	}
	if len(crcs) < 2 {
		return items
	}
	sort.Strings(crcs)
	res := MatchHeuristic(archiveTitle(f.Path) + ".zip")
	items = append(items[:first], kept...)
	return append(items, Item{
		Path: f.Path, Size: f.Size, Result: res, Bundled: true,
		ContentHash: fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(strings.Join(crcs, "+")))),
	})
}

// archiveTitle is a game title from an archive's file name: the extension
// dropped; for a name without spaces ("Castle_Quest.Publisher.zip")
// only the part before the first dot, underscores read as spaces.
func archiveTitle(path string) string {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if !strings.Contains(stem, " ") {
		if i := strings.Index(stem, "."); i > 0 {
			stem = stem[:i]
		}
		stem = strings.ReplaceAll(stem, "_", " ")
	}
	return stem
}

package library

import (
	"regexp"
	"strconv"
	"strings"
)

// discTagPattern finds the tags that number one part of a multi-part game:
//
//	(Disc 1)  (Disk 2 of 3)            Redump, No-Intro
//	(Disk 1 of 4 Side A)               TOSEC, side in the tag
//	(Disk 1 of 2)(Side B)              TOSEC, side as a second tag
//	(Tape 1 of 2)  (Tape 1 of 2 Side A)
//	(Side A)                           a two-sided disk or tape on its own
//
// Groups: 1 = number, 2 = side in the tag, 3 = side as a second tag, 4 = lone side.
var discTagPattern = regexp.MustCompile(`(?i)\((?:(?:dis[ck]|tape)\s+(\d+)(?:\s+of\s+\d+)?(?:\s+side\s+([ab]))?\)(?:\s*\(side\s+([ab])\))?|side\s+([ab])\))`)

// ParseDiscTag extracts a disc, disk, tape or side tag (see discTagPattern)
// from title, returning the part's number and the title with that tag removed
// and whitespace cleaned up (other tags, e.g. a region, are left alone).
// A disk with a side counts two parts per disk, (N-1)*2 + 1 for side A and
// +2 for side B, so the sides of a game fall in order; a lone side is 1 or 2.
// ok is false if title has no such tag.
//
// Used by Sync to keep discs of the same multi-disc game from competing
// in 1G1R selection (each disc gets its own grouping key and display
// name, so neither loses to the other — spec §2.5's multi-disc games are
// not "variants," every disc is required) and by internal/profile to
// recognize which selected games are discs of the same set when linking
// (spec §2.5's .m3u generation).
func ParseDiscTag(title string) (discNum int, baseTitle string, ok bool) {
	m := discTagPattern.FindStringSubmatchIndex(title)
	if m == nil {
		return 0, "", false
	}
	group := func(i int) string {
		if m[2*i] < 0 {
			return ""
		}
		return title[m[2*i]:m[2*i+1]]
	}
	side := strings.ToLower(group(2) + group(3) + group(4))
	sideNum := 0
	switch side {
	case "a":
		sideNum = 1
	case "b":
		sideNum = 2
	}
	switch n := group(1); {
	case n == "":
		discNum = sideNum
	default:
		v, err := strconv.Atoi(n)
		if err != nil {
			return 0, "", false
		}
		discNum = v
		if sideNum > 0 {
			discNum = (v-1)*2 + sideNum
		}
	}
	base := title[:m[0]] + title[m[1]:]
	base = strings.TrimSpace(strings.Join(strings.Fields(base), " "))
	return discNum, base, true
}

// tosecFlags matches TOSEC's bracketed dump flags: [a4], [cr Group], [!].
var tosecFlags = regexp.MustCompile(`\s*\[[^\]]*\]`)

// SetTitle is the title that ties the parts of one game together: ParseDiscTag's
// base without TOSEC dump flags. The sides of one game come from different dumps
// ("…(Disk 1 of 4 Side A)[a4]" and "…(Disk 3 of 4 Side B)"), and the flags
// describe the dump, not the game. Redump names have no flags, so are unchanged.
func SetTitle(base string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(tosecFlags.ReplaceAllString(base, "")), " "))
}

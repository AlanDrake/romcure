package redump

import "strings"

// systemIDs maps a Redump slug to this app's system id (the ids used by
// profile.LibretroDBName etc.). Deliberately partial: unmapped slugs are
// still listed in the Catalog but aren't offered for auto-sync.
var systemIDs = map[string]string{
	"PSX": "psx",
	"PS2": "ps2",
	"PSP": "psp",
	"SS":  "saturn",
	"DC":  "dreamcast",
	"MCD": "segacd",
	"PCE": "pcecd",
	"3DO": "3do",
	"GC":  "gc",
	"WII": "wii",
}

// SystemID returns the app system id for a Redump slug.
func SystemID(slug string) (string, bool) {
	id, ok := systemIDs[strings.ToUpper(slug)]
	return id, ok
}

// Slug is the inverse of SystemID.
func Slug(systemID string) (string, bool) {
	for slug, id := range systemIDs {
		if id == systemID {
			return slug, true
		}
	}
	return "", false
}

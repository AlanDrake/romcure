package match

import (
	"path/filepath"
	"regexp"
	"strings"
)

// romExtensions lists, for systems whose dumps are single files with a known
// extension, what a game inside an archive looks like. Scene releases pack
// notes and tools beside the ROM (info.f01, __tmp__, GRPTAG, a patch); those
// members are not games. Systems without an entry are not filtered.
var romExtensions = map[string]string{
	"gba":       ".gba .agb .bin .mb",
	"gb":        ".gb .gbc .sgb .cgb .bin",
	"gbc":       ".gbc .gb .cgb .sgb .bin",
	"nds":       ".nds .dsi .ids .srl .bin",
	"n64":       ".z64 .n64 .v64 .u64 .ndd .bin .rom",
	"nes":       ".nes .unf .unif .fds .bin",
	"snes":      ".sfc .smc .swc .fig .bs .st .bin",
	"genesis":   ".md .gen .smd .sgd .mdx .68k .bin",
	"sms":       ".sms .bin",
	"gamegear":  ".gg .bin",
	"pce":       ".pce .sgx .bin",
	"ngp":       ".ngp .bin",
	"ngpc":      ".ngc .ngp .npc .bin",
	"ws":        ".ws .bin",
	"wsc":       ".wsc .ws .pc2 .bin",
	"atari2600": ".a26 .bin",
	"atari7800": ".a78 .bin",
	"lynx":      ".lnx .lyx .o",
	"jaguar":    ".j64 .jag .rom .abs .cof .bin .prg",
	"c64":       ".d64 .d71 .d80 .d81 .d82 .g64 .g41 .x64 .t64 .tap .prg .crt .lnx .nib .nbz .reu .vsf .bin .seq .usr .rel",
}

var romExtSets = func() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for sys, list := range romExtensions {
		set := map[string]bool{}
		for _, e := range strings.Fields(list) {
			set[e] = true
		}
		out[sys] = set
	}
	return out
}()

// pc64Ext matches the PC64 file family: .p00 (program), .s00 (sequential),
// .u00 (user), .r00 (relative) and .d00 (disk) with a two-digit counter.
var pc64Ext = regexp.MustCompile(`^\.[psurd]\d\d$`)

// hasROMExt reports whether a name has an extension that systemID's dumps use.
func hasROMExt(systemID, name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if systemID == "c64" && pc64Ext.MatchString(ext) {
		return true
	}
	return romExtSets[systemID][ext]
}

// notGameMembers returns, per member name, whether the member is a stray file
// of a ROM archive: its system has a known set of ROM extensions, the archive
// holds at least one such member, and this member is not one. An archive with
// no recognisable ROM at all is left alone, so an unusual extension still
// shows up as a game rather than vanishing.
func notGameMembers(systemID string, names []string) map[string]bool {
	if romExtSets[systemID] == nil {
		return nil
	}
	any := false
	for _, n := range names {
		if hasROMExt(systemID, n) {
			any = true
			break
		}
	}
	if !any {
		return nil
	}
	out := map[string]bool{}
	for _, n := range names {
		if !hasROMExt(systemID, n) {
			out[n] = true
		}
	}
	return out
}

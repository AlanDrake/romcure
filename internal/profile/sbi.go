package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"romcure/internal/store"
)

// linkSBI links the Redump .sbi subchannel file for g (if one was
// downloaded) next to the game's linked file, with the same basename:
// emulators such as DuckStation and PCSX-ReARMed look for <game>.sbi beside
// the image to get past LibCrypt protection. A game with no SBI is the
// normal case and not an error.
func linkSBI(db *store.DB, systemID string, g store.SelectedGame, gameDst string, opts Options) error {
	if opts.SBIDir == "" {
		return nil
	}
	datName, ok, err := db.DatGameName(systemID, g.GameKey)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	src := filepath.Join(opts.SBIDir, invalidFileNameChars.ReplaceAllString(datName, "_")+".sbi")
	if _, err := os.Stat(src); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("check SBI %s: %w", src, err)
	}
	dst := strings.TrimSuffix(gameDst, filepath.Ext(gameDst)) + ".sbi"
	if err := linkFile(db, systemID, g.GameKey, src, dst, "sbi", opts); err != nil {
		return fmt.Errorf("link SBI for %s: %w", g.DisplayName, err)
	}
	return nil
}

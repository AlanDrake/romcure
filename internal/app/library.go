package app

import (
	"fmt"

	"romcure/internal/store"
)

// Groups lists a system's games with their variants and why each pick was
// made. It reflects the last Sync.
func (a *App) Groups(systemID string) ([]store.Group, error) {
	return a.DB.ListGroups(systemID)
}

// PickVariant makes gameKey the chosen version of its game, overriding the
// region/language rules, for every output (spec §2.1: overrides live on the
// library, not per output). It takes effect on the next Sync. The override
// is keyed by content, so it survives DAT updates that rename games.
func (a *App) PickVariant(systemID, gameKey string) error {
	gid, err := a.DB.GroupOf(systemID, gameKey)
	if err != nil {
		return err
	}
	return a.DB.SetOverride(gid, gameKey, "chosen by you")
}

// ClearPick removes the override on the game that gameKey belongs to, so the
// rules decide again on the next Sync.
func (a *App) ClearPick(systemID, gameKey string) error {
	gid, err := a.DB.GroupOf(systemID, gameKey)
	if err != nil {
		return err
	}
	if err := a.DB.ClearOverride(gid); err != nil {
		return fmt.Errorf("clear override: %w", err)
	}
	return nil
}

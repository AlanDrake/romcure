package app

import (
	"romcure/internal/elevate"
	"romcure/internal/settings"
)

// LinkStatus tells the UI whether outputs can use symbolic links and whether
// to offer the Windows administrator relaunch (spec §2.3).
type LinkStatus struct {
	SymlinksOK bool
	// Offer is true when links are not possible here, the user has not said
	// "never ask", and some output would use links: the UI shows the
	// "restart as administrator" choice.
	Offer bool
}

// offerElevation decides whether to show the elevation choice.
func offerElevation(applies, symlinksOK bool, el settings.Elevation, anyLinkOutput bool) bool {
	return applies && !symlinksOK && el == settings.ElevationAsk && anyLinkOutput
}

// LinkStatus reports link capability and whether to offer elevation.
func (a *App) LinkStatus() LinkStatus {
	anyLink := false
	if profs, err := a.DB.Profiles(); err == nil {
		for _, p := range profs {
			if p.Kind != "device" { // device outputs copy by design
				anyLink = true
			}
		}
	}
	return LinkStatus{SymlinksOK: a.symlinksOK, Offer: offerElevation(a.elevationApplies, a.symlinksOK, a.Settings.Elevation, anyLink)}
}

// RelaunchElevated restarts the app with administrator rights. It returns
// true when the new process is on its way (the caller should quit). When the
// user declines the prompt it remembers "never ask" and returns false, so the
// run continues with the next link tier (spec §2.3).
func (a *App) RelaunchElevated() (bool, error) {
	err := elevate.RelaunchElevated()
	if err == nil {
		return true, nil
	}
	if err == elevate.ErrUACDeclined {
		return false, a.SaveElevation(settings.ElevationNever)
	}
	return false, err
}

// SaveElevation stores the elevation choice.
func (a *App) SaveElevation(e settings.Elevation) error {
	next := a.Settings
	next.Elevation = e
	if err := settings.Save(a.Dir, next); err != nil {
		return err
	}
	a.Settings = next
	return nil
}

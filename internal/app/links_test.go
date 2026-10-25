package app

import (
	"testing"

	"romcure/internal/settings"
)

func TestOfferElevationOnlyWhenItWouldHelp(t *testing.T) {
	ask, never := settings.ElevationAsk, settings.ElevationNever
	for _, c := range []struct {
		name                        string
		applies, symlinksOK, linked bool
		el                          settings.Elevation
		want                        bool
	}{
		{"windows, no links, ask", true, false, true, ask, true},
		{"links already work", true, true, true, ask, false},
		{"user said never", true, false, true, never, false},
		{"only SD card outputs copy anyway", true, false, false, ask, false},
		{"not windows", false, false, true, ask, false},
	} {
		if got := offerElevation(c.applies, c.symlinksOK, c.el, c.linked); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

package link

import (
	"strings"
	"testing"
)

func caps(tiers ...TierAvailability) TierCapabilities {
	return TierCapabilities{Tiers: tiers}
}

func avail(t Tier) TierAvailability { return TierAvailability{Tier: t, Available: true} }
func unavail(t Tier, reason string) TierAvailability {
	return TierAvailability{Tier: t, Available: false, Reason: reason}
}

func TestSelectTier(t *testing.T) {
	tests := []struct {
		name         string
		caps         TierCapabilities
		prefs        Preferences
		wantTier     Tier
		wantMentions []Tier // tiers whose unavailability/skip reason must appear in Reason
	}{
		{
			name:     "everything available, hardlink not opted in -> symlink",
			caps:     caps(avail(TierSymlink), avail(TierReflink), avail(TierHardlink), avail(TierCopy)),
			prefs:    Preferences{AllowHardlink: false},
			wantTier: TierSymlink,
		},
		{
			name: "symlink unavailable -> reflink",
			caps: caps(
				unavail(TierSymlink, "no privilege"),
				avail(TierReflink),
				avail(TierHardlink),
				avail(TierCopy),
			),
			prefs:        Preferences{AllowHardlink: false},
			wantTier:     TierReflink,
			wantMentions: []Tier{TierSymlink},
		},
		{
			name: "symlink and reflink unavailable, hardlink opted in -> hardlink",
			caps: caps(
				unavail(TierSymlink, "no privilege"),
				unavail(TierReflink, "not same volume"),
				avail(TierHardlink),
				avail(TierCopy),
			),
			prefs:        Preferences{AllowHardlink: true},
			wantTier:     TierHardlink,
			wantMentions: []Tier{TierSymlink, TierReflink},
		},
		{
			name: "symlink and reflink unavailable, hardlink NOT opted in -> copy",
			caps: caps(
				unavail(TierSymlink, "no privilege"),
				unavail(TierReflink, "not same volume"),
				avail(TierHardlink),
				avail(TierCopy),
			),
			prefs:        Preferences{AllowHardlink: false},
			wantTier:     TierCopy,
			wantMentions: []Tier{TierSymlink, TierReflink, TierHardlink},
		},
		{
			name: "only copy available -> copy",
			caps: caps(
				unavail(TierSymlink, "no privilege"),
				unavail(TierReflink, "not same volume"),
				unavail(TierHardlink, "not same volume"),
				avail(TierCopy),
			),
			prefs:    Preferences{AllowHardlink: true},
			wantTier: TierCopy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectTier(tt.caps, tt.prefs)
			if got.Tier != tt.wantTier {
				t.Errorf("Tier = %v, want %v (reason: %q)", got.Tier, tt.wantTier, got.Reason)
			}
			if got.Reason == "" {
				t.Error("Reason is empty, want an explanation (spec §5 transparency rule)")
			}
			for _, skippedTier := range tt.wantMentions {
				if !strings.Contains(got.Reason, skippedTier.String()) {
					t.Errorf("Reason %q does not mention skipped tier %v", got.Reason, skippedTier)
				}
			}
		})
	}
}

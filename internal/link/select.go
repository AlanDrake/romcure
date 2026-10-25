package link

import (
	"fmt"
	"strings"
)

// Preferences controls tier selection beyond raw capability (spec §2.2).
type Preferences struct {
	// AllowHardlink opts into the hardlink tier. Off by default: a
	// hardlink *is* the source file, so an in-place write by a target
	// (a ROM patcher, a header fixer, a rename-in-place tool) would
	// modify the source, violating spec §5.
	AllowHardlink bool
}

// Selection is the result of SelectTier: which tier to use, and a
// human-readable reason. Shown in sync reports later (spec §5's
// transparency rule), so it names every higher-preference tier that was
// passed over and why, not just the one picked.
type Selection struct {
	Tier   Tier
	Reason string
}

// SelectTier picks the best available tier from caps according to prefs,
// in spec §2.2 preference order: symlink, reflink, hardlink (opt-in only),
// copy. Copy is always available, so SelectTier always returns a result.
func SelectTier(caps TierCapabilities, prefs Preferences) Selection {
	var skipped []string

	for _, avail := range caps.Tiers {
		if !avail.Available {
			skipped = append(skipped, fmt.Sprintf("%s unavailable (%s)", avail.Tier, avail.Reason))
			continue
		}
		if avail.Tier == TierHardlink && !prefs.AllowHardlink {
			skipped = append(skipped, "hardlink available but not opted in")
			continue
		}

		if len(skipped) == 0 {
			return Selection{Tier: avail.Tier, Reason: fmt.Sprintf("using %s", avail.Tier)}
		}
		return Selection{Tier: avail.Tier, Reason: fmt.Sprintf("using %s (%s)", avail.Tier, strings.Join(skipped, "; "))}
	}

	// Unreachable in practice: TierCopy is always Available per
	// Capabilities. Returned explicitly rather than falling through to a
	// zero Selection.
	return Selection{Tier: TierCopy, Reason: "using copy (no other tier was available)"}
}

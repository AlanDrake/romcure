package link

import (
	"fmt"
	"os"
	"path/filepath"

	"romcure/internal/fsinfo"
)

// Tier is a link mechanism, in spec §2.2 preference order.
type Tier int

const (
	TierSymlink Tier = iota
	TierReflink
	TierHardlink
	TierCopy
)

func (t Tier) String() string {
	switch t {
	case TierSymlink:
		return "symlink"
	case TierReflink:
		return "reflink"
	case TierHardlink:
		return "hardlink"
	case TierCopy:
		return "copy"
	default:
		return "unknown"
	}
}

// TierAvailability records whether a tier can be used to link src at dst,
// and — per the cross-cutting transparency rule (spec §5) — why not when
// it can't.
type TierAvailability struct {
	Tier      Tier
	Available bool
	Reason    string
}

// TierCapabilities is the result of probing every tier for one src/dst
// pair. Tiers is always exactly the four tiers, in spec §2.2 preference
// order, so callers can pick the first Available one.
type TierCapabilities struct {
	SameVolume bool
	Tiers      []TierAvailability
}

// Capabilities probes which link tiers are usable for linking src at dst,
// by actually attempting each one against a scratch file in dst's
// directory (removed afterward) rather than inferring from filesystem
// metadata — metadata can be misleading (e.g. a CoW-capable filesystem
// with reflink disabled by mount option). dst's directory must already
// exist; src must exist.
func Capabilities(src, dst string) (TierCapabilities, error) {
	destDir := filepath.Dir(dst)

	sameVolume, err := fsinfo.SameVolume(src, destDir)
	if err != nil {
		return TierCapabilities{}, fmt.Errorf("check same volume: %w", err)
	}

	caps := TierCapabilities{
		SameVolume: sameVolume,
		Tiers: []TierAvailability{
			probeSymlink(src, destDir),
			probeReflink(src, destDir, sameVolume),
			probeHardlink(src, destDir, sameVolume),
			{Tier: TierCopy, Available: true, Reason: "always available"},
		},
	}
	return caps, nil
}

// probePath returns a scratch path in dir unlikely to collide with a real
// target, for a one-shot create-then-remove probe.
func probePath(dir, tier string) string {
	return filepath.Join(dir, fmt.Sprintf(".romcure-probe-%s-%d", tier, os.Getpid()))
}

func probeSymlink(src, destDir string) TierAvailability {
	probe := probePath(destDir, "symlink")
	defer os.Remove(probe)

	if err := SymlinkAbsolute(src, probe); err != nil {
		return TierAvailability{Tier: TierSymlink, Available: false, Reason: err.Error()}
	}
	return TierAvailability{Tier: TierSymlink, Available: true}
}

func probeReflink(src, destDir string, sameVolume bool) TierAvailability {
	if !sameVolume {
		return TierAvailability{Tier: TierReflink, Available: false, Reason: "source and destination are not on the same volume"}
	}

	probe := probePath(destDir, "reflink")
	defer os.Remove(probe)

	if err := Reflink(src, probe); err != nil {
		return TierAvailability{Tier: TierReflink, Available: false, Reason: err.Error()}
	}
	return TierAvailability{Tier: TierReflink, Available: true}
}

func probeHardlink(src, destDir string, sameVolume bool) TierAvailability {
	if !sameVolume {
		return TierAvailability{Tier: TierHardlink, Available: false, Reason: "source and destination are not on the same volume"}
	}

	probe := probePath(destDir, "hardlink")
	defer os.Remove(probe)

	if err := Hardlink(src, probe); err != nil {
		return TierAvailability{Tier: TierHardlink, Available: false, Reason: err.Error()}
	}
	return TierAvailability{Tier: TierHardlink, Available: true}
}

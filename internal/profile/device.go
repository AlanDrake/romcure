package profile

import (
	"fmt"
	"os"
	"path/filepath"

	"romcure/internal/discsheet"
	"romcure/internal/fsinfo"
	"romcure/internal/store"
)

// fat32MaxFile is the largest file FAT32 can hold (4 GiB - 1).
const fat32MaxFile = 4<<30 - 1

// DeviceExportOptions configures an export to a removable device (SD card,
// USB stick: spec §2.4). The device keeps real copies, and the planner
// checks destination type, free space, and the FAT32 file-size limit
// before anything is written.
type DeviceExportOptions struct {
	Options // OutputRoot is the device's ROM root
	Frontend         Frontend
	Systems          []string
}

// SystemEstimate is one system's share of an export plan.
type SystemEstimate struct {
	SystemID string
	Games    int
	Bytes    int64
}

// ExportPlan is the up-front size estimate and verdict for an export.
type ExportPlan struct {
	Systems    []SystemEstimate
	TotalBytes int64
	FreeBytes  uint64
	FSType     fsinfo.FilesystemType
	// MaxFileBytes is the destination's per-file limit (0 = none).
	MaxFileBytes int64
	// Oversize lists games whose files exceed MaxFileBytes; they are skipped
	// at export time with this same reason.
	Oversize []string
	// Estimated is true when some sizes are upper bounds (a disc that will
	// be transcoded is counted at its uncompressed size).
	Estimated bool
	// Blockers explain why the export must not start.
	Blockers []string
}

// OK reports whether the export may proceed.
func (p ExportPlan) OK() bool { return len(p.Blockers) == 0 }

// PlanDeviceExport estimates and validates an export without writing
// anything: remote destinations are blocked, the filesystem decides the
// file-size limit, and the total is compared with the free space.
func PlanDeviceExport(db *store.DB, opts DeviceExportOptions) (ExportPlan, error) {
	var plan ExportPlan
	if err := os.MkdirAll(opts.OutputRoot, 0o755); err != nil {
		return plan, fmt.Errorf("create %s: %w", opts.OutputRoot, err)
	}
	info, err := fsinfo.Detect(opts.OutputRoot)
	if err != nil {
		return plan, fmt.Errorf("inspect destination %s: %w", opts.OutputRoot, err)
	}
	plan.FSType, plan.FreeBytes = info.Type, info.FreeBytes
	if info.IsRemote {
		plan.Blockers = append(plan.Blockers, "destination is a network share; network destinations are not supported (spec §2.4)")
	}
	if info.Type == fsinfo.FAT32 {
		plan.MaxFileBytes = fat32MaxFile
	}

	for _, systemID := range opts.Systems {
		games, _, err := selectedGames(db, systemID, opts.Options)
		if err != nil {
			return plan, err
		}
		est := SystemEstimate{SystemID: systemID, Games: len(games)}
		for _, g := range games {
			size, largest, estimated, err := estimateGame(db, systemID, g, opts.Options)
			if err != nil {
				return plan, fmt.Errorf("estimate %s: %w", g.DisplayName, err)
			}
			if plan.MaxFileBytes > 0 && largest > plan.MaxFileBytes {
				plan.Oversize = append(plan.Oversize, fmt.Sprintf("%s (%s): %d bytes", g.DisplayName, systemID, largest))
				continue // will be skipped, so it takes no space
			}
			est.Bytes += size
			plan.Estimated = plan.Estimated || estimated
		}
		plan.Systems = append(plan.Systems, est)
		plan.TotalBytes += est.Bytes
	}

	plan.checkSpace()
	return plan, nil
}

func (p *ExportPlan) checkSpace() {
	if uint64(p.TotalBytes) > p.FreeBytes {
		p.Blockers = append(p.Blockers, fmt.Sprintf("needs %d bytes but only %d are free on the destination", p.TotalBytes, p.FreeBytes))
	}
}

// estimateGame returns the bytes a game's output will occupy, the size of
// its largest single file, and whether the figure is an upper bound.
func estimateGame(db *store.DB, systemID string, g store.SelectedGame, opts Options) (total, largest int64, estimated bool, err error) {
	add := func(n int64) {
		total += n
		if n > largest {
			largest = n
		}
	}
	switch g.UltimateSourceKind {
	case "raw_file":
		st, err := os.Stat(g.UltimateSourceRef)
		if err != nil {
			return 0, 0, false, err
		}
		add(st.Size())
		members, _ := discsheet.ReferencedFiles(g.UltimateSourceRef)
		for _, m := range members {
			if ms, err := os.Stat(filepath.Join(filepath.Dir(g.UltimateSourceRef), m)); err == nil {
				add(ms.Size())
			}
		}
		if t := opts.Transcode; t != nil && IsDiscSystem(systemID) && isRawDiscImage(systemID, g.UltimateSourceRef) {
			estimated = true // output is a CHD, no larger than this
		}
	case "archive":
		rec, err := db.GetFileByPath(g.UltimateSourceRef)
		if err != nil || rec == nil {
			return 0, 0, false, fmt.Errorf("archive %s not in hash cache", g.UltimateSourceRef)
		}
		members, err := db.ListArchiveMembers(rec.ID)
		if err != nil {
			return 0, 0, false, err
		}
		switch archivePolicyFor(*rec, len(members) == 1, systemID) {
		case PolicyPassthrough:
			add(rec.Size)
		case PolicyExtract:
			for _, m := range members {
				if m.MemberName == g.MemberName {
					add(m.Size)
				}
			}
		default: // transcode: CHD, bounded by the uncompressed image
			estimated = true
			var sum int64
			for _, m := range members {
				sum += m.Size
			}
			add(sum)
		}
	}
	return total, largest, estimated, nil
}

// ExportToDevice plans, then (if the plan has no blockers) copies every
// system into the device's ES-family layout. Files over the destination's
// size limit are reported as errors and skipped. Re-running only copies
// what is missing or whose size changed.
func ExportToDevice(db *store.DB, opts DeviceExportOptions) (ExportPlan, map[string]Report, error) {
	plan, err := PlanDeviceExport(db, opts)
	if err != nil || !plan.OK() {
		return plan, nil, err
	}
	reports := map[string]Report{}
	ro := opts.Options
	ro.ForceCopy = true
	ro.MaxFileBytes = plan.MaxFileBytes
	if ro.ProfileID == "" {
		// One profile id per destination so two cards never share link_state rows.
		ro.ProfileID = "device:" + opts.OutputRoot
	}
	for _, systemID := range opts.Systems {
		rep, err := GenerateESFamily(db, systemID, opts.Frontend, ro)
		reports[systemID] = rep
		if err != nil {
			return plan, reports, err
		}
	}
	return plan, reports, nil
}

package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"romcure/internal/chdverify"
	"romcure/internal/nointro"
	"romcure/internal/redump"
	"romcure/internal/serials"
	"romcure/internal/store"
	"romcure/internal/tosec"
	"romcure/internal/transcode"
)

// DATResult is the outcome of updating one system's DAT.
type DATResult struct {
	SystemID string
	Source   string // "redump", "tosec", "no-intro" or "" when no online DAT exists for it
	Updated  bool
	Message  string // what happened, in words (also for "nothing to do")
	Error    string
}

// Endpoints lets tests point the downloaders at local servers.
type Endpoints struct {
	Client    *http.Client
	RedumpURL string
	TOSECURL  string
	// NoIntroURL lists libretro-database's No-Intro DATs (nointro.IndexURL).
	NoIntroURL string
	// SerialsURL is where the libretro serial lists are (serials.DefaultBaseURL).
	SerialsURL string
}

func (e Endpoints) withDefaults() Endpoints {
	if e.Client == nil {
		e.Client = &http.Client{Timeout: 15 * time.Minute}
	}
	if e.RedumpURL == "" {
		e.RedumpURL = redump.BaseURL
	}
	if e.TOSECURL == "" {
		e.TOSECURL = tosec.BaseURL
	}
	return e
}

func hasTOSEC(systemID string) bool {
	for _, id := range tosec.Systems() {
		if id == systemID {
			return true
		}
	}
	return false
}

// UpdateDATs downloads the latest Redump, TOSEC or No-Intro (libretro-database's
// copy) DAT for every system that has a source, only touching the network on
// this explicit call. Each site's index is fetched once; systems whose DAT is
// current cost no download. A No-Intro DAT imported by hand takes precedence
// over the downloaded one. A failure for one system is reported and the others
// still run. Run Sync afterwards to apply the new DATs to the library.
func (a *App) UpdateDATs(ctx context.Context, ep Endpoints, progress func(Progress)) ([]DATResult, error) {
	ep = ep.withDefaults()
	all, err := a.DB.Sources()
	if err != nil {
		return nil, err
	}
	var srcs []store.Source // one per system: a DAT belongs to the system, not to a folder
	seen := map[string]bool{}
	for _, s := range all {
		if !seen[s.SystemID] {
			seen[s.SystemID] = true
			srcs = append(srcs, s)
		}
	}

	var (
		rcat    *redump.Catalog
		rcatErr error
		rel     *tosec.Release
		relErr  error
		nidx    nointro.Index
		nidxErr error
		results []DATResult
		dl      = &redump.Downloader{Client: ep.Client, BaseURL: ep.RedumpURL, Dir: a.Dir}
		tsyncer = &tosec.Syncer{DB: a.DB, Client: ep.Client, Dir: a.Dir}
		nsyncer = &nointro.Syncer{DB: a.DB, Client: ep.Client, Dir: a.Dir}
		r       = a.reporter(ctx, progress, len(srcs))
	)

	for _, s := range srcs {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		res := DATResult{SystemID: s.SystemID}
		switch slug, ok := redump.Slug(s.SystemID); {
		case ok:
			res.Source = "redump"
			r.step(Progress{Stage: "dat", SystemID: s.SystemID, Detail: "Redump"})
			if rcat == nil && rcatErr == nil {
				rcat, rcatErr = redump.FetchCatalog(ctx, ep.Client, ep.RedumpURL)
			}
			if rcatErr != nil {
				res.Error = rcatErr.Error()
				break
			}
			sys, found := rcat.Find(slug)
			if !found {
				res.Error = "Redump no longer lists " + slug
				break
			}
			r, err := redump.SyncSystem(ctx, a.DB, dl, sys, true)
			res.Updated, res.Message = r.Updated, r.Reason
			if r.SBIFiles > 0 {
				res.Message += fmt.Sprintf(" (+%d SBI files)", r.SBIFiles)
			}
			if err != nil {
				res.Error = err.Error()
			}
			if serials.Has(s.SystemID) {
				res.Message += a.updateSerials(ctx, ep, s.SystemID)
			}
		case hasTOSEC(s.SystemID):
			res.Source = "tosec"
			r.step(Progress{Stage: "dat", SystemID: s.SystemID, Detail: "TOSEC"})
			if rel == nil && relErr == nil {
				var r tosec.Release
				r, relErr = tosec.FetchLatest(ctx, ep.Client, ep.TOSECURL)
				rel = &r
			}
			if relErr != nil {
				res.Error = relErr.Error()
				break
			}
			r, err := tsyncer.SyncSystem(ctx, *rel, s.SystemID, nil)
			res.Updated, res.Message = r.Updated, r.Reason
			if err != nil {
				res.Error = err.Error()
			}
		default:
			if manual, err := a.hasManualDAT(s.SystemID); err != nil || manual {
				res.Source = "manual"
				res.Message, res.Error = "using the DAT you imported", errText(err)
				break
			}
			r.step(Progress{Stage: "dat", SystemID: s.SystemID, Detail: "No-Intro"})
			if nidx == nil && nidxErr == nil {
				nidx, nidxErr = nointro.FetchIndex(ctx, ep.Client, ep.NoIntroURL)
			}
			if nidxErr != nil {
				res.Source, res.Error = "no-intro", nidxErr.Error()
				break
			}
			f, ok := nidx.Find(s.SystemID)
			if !ok {
				res.Message = "no online DAT source for this system; import a DAT by hand or rely on file names"
				break
			}
			res.Source = "no-intro"
			r, err := nsyncer.SyncSystem(ctx, s.SystemID, f)
			res.Updated, res.Message, res.Error = r.Updated, r.Reason, errText(err)
		}
		results = append(results, res)
		r.next()
	}
	return results, nil
}

// hasManualDAT reports whether the user imported a DAT for systemID by hand.
func (a *App) hasManualDAT(systemID string) (bool, error) {
	var n int
	err := a.DB.QueryRow(`SELECT COUNT(*) FROM dats WHERE remote_source = 'manual' AND remote_slug = ?`, systemID).Scan(&n)
	return n > 0, err
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// updateSerials downloads the serial list of a PlayStation system (used to name
// discs no DAT matched) and says how it went, for the DAT result's message. A
// failure does not fail the DAT update: the list only improves names.
func (a *App) updateSerials(ctx context.Context, ep Endpoints, systemID string) string {
	list, err := serials.Fetch(ctx, ep.Client, ep.SerialsURL, systemID)
	if err != nil {
		a.Log.Warn("serial list", "system", systemID, "error", err)
		return " (serial list not updated: " + err.Error() + ")"
	}
	entries := make([]store.SerialEntry, len(list.Entries))
	for i, e := range list.Entries {
		entries[i] = store.SerialEntry{Serial: e.Serial, Name: e.Name}
	}
	if err := a.DB.ReplaceSerials(systemID, list.Version, entries); err != nil {
		return " (serial list not stored: " + err.Error() + ")"
	}
	return fmt.Sprintf(" (+%d serials)", len(entries))
}

// CHDResult summarises one system's CHD verification.
type CHDResult struct {
	SystemID string
	Checked  int
	Errors   []string
}

// VerifyCHDs hashes the tracks of every CHD in the disc-system sources so
// they can be matched against Redump (a CHD header alone can't be). It is
// slow, a full read and temporary extraction of each disc, so it only runs
// on request; already-checked unchanged files are skipped. Run Sync
// afterwards to identify them.
func (a *App) VerifyCHDs(ctx context.Context, progress func(Progress)) ([]CHDResult, error) {
	if a.Tools.Chdman == "" {
		return nil, fmt.Errorf("chdman was not found, so CHD files can't be checked")
	}
	srcs, err := a.DB.Sources()
	if err != nil {
		return nil, err
	}
	v := &chdverify.Verifier{
		DB:         a.DB,
		Tool:       &transcode.Chdman{Path: a.Tools.Chdman, Version: a.Tools.ChdmanVersion},
		ScratchDir: a.Settings.Scratch(a.Dir),
	}
	var out []CHDResult
	for _, s := range srcs {
		if _, ok := redump.Slug(s.SystemID); !ok {
			continue
		}
		res := CHDResult{SystemID: s.SystemID}
		err := v.VerifyUnder(ctx, s.Path,
			func(done, total int, path string) {
				res.Checked = done + 1
				if progress != nil {
					progress(Progress{Stage: "chd", SystemID: s.SystemID, Item: baseName(path), Done: done, Total: total, Detail: fmt.Sprintf("%d/%d %s", done+1, total, baseName(path))})
				}
			},
			func(path string, err error) { res.Errors = append(res.Errors, baseName(path)+": "+err.Error()) })
		if err != nil {
			return out, err
		}
		out = append(out, res)
	}
	return out, nil
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}

package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"romcure/internal/catalog"
	"romcure/internal/library"
	"romcure/internal/profile"
	"romcure/internal/store"
)

// This file backs the Games screen (docs/game-selection-plan.md): browsing,
// ticking games per output, choosing versions, previewing and applying.

// maxUndo is how many changes can be undone.
const maxUndo = 20

type undoEntry struct {
	token string
	label string
	run   func() error
}

// InvalidateCatalog makes the next Games call rebuild the index.
func (a *App) InvalidateCatalog() {
	a.gamesMu.Lock()
	a.cat = nil
	a.incl = nil
	a.gamesMu.Unlock()
}

// dropInclusions forgets the cached ticks after they changed.
func (a *App) dropInclusions() {
	a.gamesMu.Lock()
	a.incl = nil
	a.gamesMu.Unlock()
}

func (a *App) catalog() (*catalog.Catalog, error) {
	a.gamesMu.Lock()
	defer a.gamesMu.Unlock()
	if a.cat == nil {
		c, err := catalog.Load(a.DB)
		if err != nil {
			return nil, err
		}
		a.cat = c
	}
	return a.cat, nil
}

func (a *App) pushUndo(label string, run func() error) string {
	a.gamesMu.Lock()
	defer a.gamesMu.Unlock()
	a.undoSeq++
	tok := strconv.Itoa(a.undoSeq)
	a.undo = append(a.undo, undoEntry{token: tok, label: label, run: run})
	if len(a.undo) > maxUndo {
		a.undo = a.undo[len(a.undo)-maxUndo:]
	}
	return tok
}

// Undo reverts the latest change. token must be the one SetIncluded,
// FollowSystem, SetVersion or AutoVersion returned, or "" for whatever came
// last. It returns what was undone.
func (a *App) Undo(token string) (string, error) {
	a.gamesMu.Lock()
	if len(a.undo) == 0 {
		a.gamesMu.Unlock()
		return "", errors.New("nothing to undo")
	}
	top := a.undo[len(a.undo)-1]
	if token != "" && token != top.token {
		a.gamesMu.Unlock()
		return "", errors.New("a newer change has to be undone first")
	}
	a.undo = a.undo[:len(a.undo)-1]
	a.gamesMu.Unlock()
	if err := top.run(); err != nil {
		return "", err
	}
	return top.label, nil
}

// UndoLabel describes the change Undo would revert, "" when there is none.
func (a *App) UndoLabel() string {
	a.gamesMu.Lock()
	defer a.gamesMu.Unlock()
	if len(a.undo) == 0 {
		return ""
	}
	return a.undo[len(a.undo)-1].label
}

func (a *App) inclusion(outputID int64) (*store.Inclusion, error) {
	if outputID == 0 {
		return nil, nil
	}
	a.gamesMu.Lock()
	in := a.incl[outputID]
	a.gamesMu.Unlock()
	if in != nil {
		return in, nil
	}
	in, err := a.DB.Inclusion(outputID)
	if err != nil {
		return nil, err
	}
	a.gamesMu.Lock()
	if a.incl == nil {
		a.incl = map[int64]*store.Inclusion{}
	}
	a.incl[outputID] = in
	a.gamesMu.Unlock()
	return in, nil
}

func (a *App) profileByID(id int64) (store.Profile, error) {
	profs, err := a.DB.Profiles()
	if err != nil {
		return store.Profile{}, err
	}
	for _, p := range profs {
		if p.ID == id {
			return p, nil
		}
	}
	return store.Profile{}, fmt.Errorf("output %d not found", id)
}

// Games returns one page of the filtered game list with its state on the
// filter's output.
func (a *App) Games(f catalog.Filter, offset, limit int) (catalog.Page, error) {
	c, err := a.catalog()
	if err != nil {
		return catalog.Page{}, err
	}
	in, err := a.inclusion(f.OutputID)
	if err != nil {
		return catalog.Page{}, err
	}
	return c.Page(f, in, offset, limit), nil
}

// SystemCounts is the systems rail: games and included games per system, and
// what new games do on this output.
type SystemCounts struct {
	Systems []catalog.SystemCount
	// Default is what a system without its own tick does (new games follow it).
	Default bool
	// Ticked holds the systems that have a tick of their own.
	Ticked map[string]bool
}

// SystemCounts lists the systems of the library with their counts on an output.
func (a *App) SystemCounts(outputID int64) (SystemCounts, error) {
	c, err := a.catalog()
	if err != nil {
		return SystemCounts{}, err
	}
	in, err := a.inclusion(outputID)
	if err != nil {
		return SystemCounts{}, err
	}
	out := SystemCounts{Systems: c.Counts(in), Ticked: map[string]bool{}}
	if in != nil {
		out.Default = in.Default
		out.Ticked = in.Systems
	}
	return out, nil
}

// OutputState is a game's state on one output.
type OutputState struct {
	ID       int64
	Name     string
	Included bool
	// Source says what decided it: "game" (the user's tick on the game),
	// "system" (the user's tick on its system) or "default".
	Source string
}

// GameDetail is everything the details drawer shows about one game.
type GameDetail struct {
	Key      string
	Name     string
	SystemID string
	Discs    int
	// Groups has one entry per disc (one for a single game); each lists its
	// versions and why one was picked.
	Groups  []store.Group
	Outputs []OutputState
}

// Game returns the details of one list row.
func (a *App) Game(key string) (GameDetail, error) {
	c, err := a.catalog()
	if err != nil {
		return GameDetail{}, err
	}
	r, ok := c.Row(key)
	if !ok {
		return GameDetail{}, fmt.Errorf("game %s not found; the library changed, reload the list", key)
	}
	groups, err := a.DB.GroupsByID(r.GroupIDs)
	if err != nil {
		return GameDetail{}, err
	}
	d := GameDetail{Key: r.Key, Name: r.Name, SystemID: r.SystemID, Discs: r.Discs, Groups: groups}
	profs, err := a.DB.Profiles()
	if err != nil {
		return d, err
	}
	for _, p := range profs {
		in, err := a.inclusion(p.ID)
		if err != nil {
			return d, err
		}
		st := OutputState{ID: p.ID, Name: p.Name, Included: true, Source: "default"}
		viaGame := false
		for _, g := range r.GroupIDs {
			if !in.Includes(r.SystemID, g) {
				st.Included = false
			}
			if _, ok := in.Groups[g]; ok {
				viaGame = true
			}
		}
		switch _, viaSystem := in.Systems[r.SystemID]; {
		case viaGame:
			st.Source = "game"
		case viaSystem:
			st.Source = "system"
		}
		d.Outputs = append(d.Outputs, st)
	}
	return d, nil
}

// Target says what a tick applies to; exactly one field is set.
type Target struct {
	All      bool            // every game
	SystemID string          // every game of a system
	Keys     []string        // these list rows
	Filter   *catalog.Filter // every game the filter matches
}

// Change is the outcome of a tick, with the token that undoes it.
type Change struct {
	Token    string
	Affected int // games whose state changed
	Message  string
}

func (a *App) pushTickUndo(outputID int64, label string, snap store.TickState) string {
	return a.pushUndo(label, func() error {
		defer a.dropInclusions()
		return a.DB.RestoreTickState(outputID, snap)
	})
}

// SetIncluded ticks or unticks games on an output. The choice is saved at
// once; the output's folder changes on Apply.
func (a *App) SetIncluded(outputID int64, t Target, included bool) (Change, error) {
	prof, err := a.profileByID(outputID)
	if err != nil {
		return Change{}, err
	}
	c, err := a.catalog()
	if err != nil {
		return Change{}, err
	}
	in, err := a.inclusion(outputID)
	if err != nil {
		return Change{}, err
	}
	snap, err := a.DB.TickState(outputID)
	if err != nil {
		return Change{}, err
	}
	flip := func(total, on int) int {
		if included {
			return total - on
		}
		return on
	}
	var affected int
	switch {
	case t.All:
		p := c.Page(catalog.Filter{}, in, 0, 0)
		affected = flip(p.Total, p.Included)
		err = a.DB.SetOutputDefault(outputID, included)
	case t.SystemID != "":
		p := c.Page(catalog.Filter{Systems: []string{t.SystemID}}, in, 0, 0)
		affected = flip(p.Total, p.Included)
		err = a.DB.SetSystemIncluded(outputID, t.SystemID, included)
	default:
		var rows []*catalog.Row
		if t.Filter != nil {
			rows = c.Matching(*t.Filter, in)
		} else {
			rows = c.ByKeys(t.Keys)
		}
		affected = flip(len(rows), catalog.CountIncluded(rows, in))
		err = a.DB.SetGamesIncluded(outputID, c.Groups(rows), included)
	}
	a.dropInclusions()
	if err != nil {
		return Change{}, err
	}
	verb, prep := "Included", "in"
	if !included {
		verb, prep = "Left out", "from"
	}
	msg := fmt.Sprintf("%s %d %s %s %s.", verb, affected, plural(affected, "game"), prep, prof.Name)
	return Change{Token: a.pushTickUndo(outputID, msg, snap), Affected: affected, Message: msg}, nil
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// FollowSystem drops the user's own tick on games, so they follow their
// system's setting (or the output's default) again.
func (a *App) FollowSystem(outputID int64, keys []string) (Change, error) {
	c, err := a.catalog()
	if err != nil {
		return Change{}, err
	}
	snap, err := a.DB.TickState(outputID)
	if err != nil {
		return Change{}, err
	}
	rows := c.ByKeys(keys)
	err = a.DB.ClearGamesTicks(outputID, c.Groups(rows))
	a.dropInclusions()
	if err != nil {
		return Change{}, err
	}
	msg := "Games follow their system's setting again."
	return Change{Token: a.pushTickUndo(outputID, msg, snap), Affected: len(rows), Message: msg}, nil
}

// reselect re-runs the rules for one system from the database (no folder
// scan), so a changed override takes effect: it updates which version each
// game has selected, and the display names that outputs are named after.
func (a *App) reselect(systemID string) error {
	srcs, err := a.DB.Sources()
	if err != nil {
		return err
	}
	_, roots := systemsOf(srcs)
	if len(roots[systemID]) > 0 {
		if _, err := library.SyncRoots(a.DB, roots[systemID], systemID, a.prefs()); err != nil {
			return err
		}
	}
	a.InvalidateCatalog()
	return nil
}

type overrideState struct {
	key, reason string
	set         bool
}

// setOverrides applies an override per group (nil key = back to automatic),
// then reselects the system. It returns an undo that restores the previous
// overrides.
func (a *App) setOverrides(systemID, label string, groups map[int64]string) (Change, error) {
	prev := map[int64]overrideState{}
	for gid := range groups {
		k, r, ok, err := a.DB.GetOverride(gid)
		if err != nil {
			return Change{}, err
		}
		prev[gid] = overrideState{key: k, reason: r, set: ok}
	}
	apply := func(want map[int64]overrideState) error {
		for gid, o := range want {
			var err error
			if o.set {
				err = a.DB.SetOverride(gid, o.key, o.reason)
			} else {
				err = a.DB.ClearOverride(gid)
			}
			if err != nil {
				return err
			}
		}
		return a.reselect(systemID)
	}
	next := map[int64]overrideState{}
	for gid, key := range groups {
		next[gid] = overrideState{key: key, reason: "chosen by you", set: key != ""}
	}
	if err := apply(next); err != nil {
		return Change{}, err
	}
	tok := a.pushUndo(label, func() error { return apply(prev) })
	return Change{Token: tok, Affected: len(groups), Message: label}, nil
}

// SetVersion makes gameKey (one of the versions listed in Game) the version
// used on every output. For a multi-disc game, each other disc takes the
// version with the same region, language and revision, or keeps its own pick
// when it has none.
func (a *App) SetVersion(rowKey, gameKey string) (Change, error) {
	c, err := a.catalog()
	if err != nil {
		return Change{}, err
	}
	r, ok := c.Row(rowKey)
	if !ok {
		return Change{}, fmt.Errorf("game %s not found; reload the list", rowKey)
	}
	groups, err := a.DB.GroupsByID(r.GroupIDs)
	if err != nil {
		return Change{}, err
	}
	var want *store.Variant
	for _, g := range groups {
		for i := range g.Variants {
			if g.Variants[i].GameKey == gameKey {
				want = &g.Variants[i]
			}
		}
	}
	if want == nil {
		return Change{}, errors.New("that version does not belong to this game")
	}
	picks := map[int64]string{}
	for _, g := range groups {
		for _, v := range g.Variants {
			if v.GameKey == gameKey || (v.Region == want.Region && v.Language == want.Language && v.Revision == want.Revision) {
				picks[g.ID] = v.GameKey
				break
			}
		}
	}
	return a.setOverrides(r.SystemID, "Changed the version of "+r.Name+".", picks)
}

// AutoVersion puts a game back to the automatic version choice.
func (a *App) AutoVersion(rowKey string) (Change, error) {
	c, err := a.catalog()
	if err != nil {
		return Change{}, err
	}
	r, ok := c.Row(rowKey)
	if !ok {
		return Change{}, fmt.Errorf("game %s not found; reload the list", rowKey)
	}
	picks := map[int64]string{}
	for _, g := range r.GroupIDs {
		picks[g] = ""
	}
	return a.setOverrides(r.SystemID, r.Name+" is back to the automatic version.", picks)
}

// Pending summarises what Apply would do to one output.
type Pending struct {
	Add, Remove, Replace int
	// Count is how many games the output takes; Bytes their size (an upper
	// bound when Estimated: a disc that will be converted counts uncompressed).
	Count     int
	Bytes     int64
	Estimated bool
	// Any is true when Apply has something to do.
	Any bool
	// Device is set for SD card / USB outputs.
	Device *DevicePending
	// UndoLabel describes what the Undo button would revert.
	UndoLabel string
}

// DevicePending is the size check of an export to a removable device.
type DevicePending struct {
	FSType       string
	FreeBytes    uint64
	MaxFileBytes int64
	Oversize     []string
	// Blockers are reasons Apply must not run (does not fit, network share...).
	Blockers []string
}

// outputOptions builds the generator options for one output and system.
func (a *App) outputOptions(p store.Profile, systemID string) (profile.Options, error) {
	opts := profile.Options{
		OutputRoot:      p.OutputRoot,
		ExtractCacheDir: a.bigCache("extract-cache"),
		SevenZip:        a.Tools.SevenZip,
		AllowHardlink:   a.Settings.AllowHardlink,
		Transcode:       a.transcodeOptions(),
		ProfileID:       store.OutputKey(p.ID),
		Parallelism:     a.Settings.Parallelism,
	}
	if systemID == "psx" {
		opts.SBIDir = a.cachePath("sbi", "PSX")
	}
	in, err := a.DB.Inclusion(p.ID)
	if err != nil {
		return opts, err
	}
	opts.Include = func(g store.SelectedGame) bool { return in.Includes(g.SystemID, g.GroupID) }
	return opts, nil
}

// Pending compares what the output should hold with what it holds now.
func (a *App) Pending(outputID int64) (Pending, error) {
	var out Pending
	prof, err := a.profileByID(outputID)
	if err != nil {
		return out, err
	}
	c, err := a.catalog()
	if err != nil {
		return out, err
	}
	in, err := a.inclusion(outputID)
	if err != nil {
		return out, err
	}
	srcs, err := a.DB.Sources()
	if err != nil {
		return out, err
	}
	hasSource := map[string]bool{}
	for _, s := range srcs {
		hasSource[s.SystemID] = true
	}
	linkedGames, err := a.DB.LinkedGames(store.OutputKey(outputID))
	if err != nil {
		return out, err
	}
	type sk struct{ sys, key string }
	linked := map[sk]bool{}
	linkedOfGroup := map[int64][]sk{}
	for _, l := range linkedGames {
		if !hasSource[l.SystemID] {
			continue
		}
		k := sk{l.SystemID, l.GameKey}
		linked[k] = true
		if l.GroupID != 0 {
			linkedOfGroup[l.GroupID] = append(linkedOfGroup[l.GroupID], k)
		}
	}
	selected, err := a.DB.SelectedKeys()
	if err != nil {
		return out, err
	}
	wanted := map[sk]bool{}
	consumed := map[sk]bool{}
	for _, g := range selected {
		if !hasSource[g.SystemID] || !in.Includes(g.SystemID, g.GroupID) {
			continue
		}
		k := sk{g.SystemID, g.GameKey}
		wanted[k] = true
		if linked[k] {
			continue
		}
		if others := linkedOfGroup[g.GroupID]; len(others) > 0 {
			for _, o := range others {
				consumed[o] = true
			}
			out.Replace++
		} else {
			out.Add++
		}
	}
	for k := range linked {
		if !wanted[k] && !consumed[k] {
			out.Remove++
		}
	}
	out.Bytes, out.Estimated, out.Count = c.IncludedBytes(in)
	out.Any = out.Add+out.Remove+out.Replace > 0
	out.UndoLabel = a.UndoLabel()

	if prof.Kind == "device" {
		d := &DevicePending{}
		out.Device = d
		systems, _ := systemsOf(srcs)
		opts, err := a.outputOptions(prof, "")
		if err != nil {
			return out, err
		}
		plan, err := profile.PlanDeviceExport(a.DB, profile.DeviceExportOptions{
			Options: opts, Frontend: profile.FrontendESDE, Systems: systems,
		})
		if err != nil {
			d.Blockers = append(d.Blockers, err.Error())
			return out, nil
		}
		d.FSType, d.FreeBytes, d.MaxFileBytes = plan.FSType.String(), plan.FreeBytes, plan.MaxFileBytes
		d.Oversize, d.Blockers = plan.Oversize, plan.Blockers
		out.Bytes, out.Estimated = plan.TotalBytes, plan.Estimated
	}
	return out, nil
}

// Apply regenerates the given outputs (all when outputIDs is empty) from the
// database. It does not scan the source folders or re-run the rules, so it is
// quick when little changed.
func (a *App) Apply(ctx context.Context, outputIDs []int64, progress func(Progress)) (SyncReport, error) {
	var report SyncReport
	srcs, err := a.DB.Sources()
	if err != nil {
		return report, err
	}
	all, err := a.DB.Profiles()
	if err != nil {
		return report, err
	}
	var profs []store.Profile
	for _, p := range all {
		if len(outputIDs) == 0 {
			profs = append(profs, p)
			continue
		}
		for _, id := range outputIDs {
			if id == p.ID {
				profs = append(profs, p)
			}
		}
	}
	systems, _ := systemsOf(srcs)
	r := a.reporter(ctx, progress, len(profs)*(len(systems)+1))
	return report, a.generateOutputs(ctx, systems, profs, r, &report)
}

// OutputSummary is one output's card on the Outputs tab.
type OutputSummary struct {
	ID       int64
	Included int // games the output takes
	Total    int // games in the library
	Pending  Pending
}

// OutputSummaries returns every output's size and waiting changes.
func (a *App) OutputSummaries() ([]OutputSummary, error) {
	profs, err := a.DB.Profiles()
	if err != nil {
		return nil, err
	}
	c, err := a.catalog()
	if err != nil {
		return nil, err
	}
	var out []OutputSummary
	for _, p := range profs {
		in, err := a.inclusion(p.ID)
		if err != nil {
			return nil, err
		}
		page := c.Page(catalog.Filter{}, in, 0, 0)
		pend, err := a.Pending(p.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, OutputSummary{ID: p.ID, Included: page.Included, Total: page.Total, Pending: pend})
	}
	return out, nil
}

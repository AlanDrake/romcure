package rules

// Preset is a ready-made set of 1G1R preferences for a typical audience
// (spec M5 "region preset templates"). Presets are starting points: callers
// copy Prefs and adjust (e.g. promote another language).
type Preset struct {
	ID          string
	Name        string
	Description string
	Prefs       Preferences
}

func cleanPrefs(regions, languages []string) Preferences {
	return Preferences{
		RegionPriority:       regions,
		LanguagePriority:     languages,
		PreferLatestRevision: true,
		ExcludeBeta:          true,
		ExcludeProto:         true,
		ExcludeDemo:          true,
	}
}

// Presets lists the built-in region templates. A multi-region release such
// as "(USA, Europe)" ranks as its best-ranked region in the list.
var Presets = []Preset{
	{
		ID: "usa", Name: "USA first",
		Description: "English releases: USA, then World, other English-speaking regions, then Europe and Japan.",
		Prefs: cleanPrefs(
			[]string{"USA", "World", "Canada", "Australia", "UK", "Europe", "Japan", "Asia"},
			[]string{"En"}),
	},
	{
		ID: "europe", Name: "Europe first",
		Description: "PAL releases: Europe, then World, then the major European countries, then USA and Japan.",
		Prefs: cleanPrefs(
			[]string{"Europe", "World", "UK", "Germany", "France", "Spain", "Italy", "Australia", "USA", "Japan", "Asia"},
			[]string{"En", "Fr", "De", "Es", "It"}),
	},
	{
		ID: "japan", Name: "Japan first",
		Description: "Japanese releases first (English-patched or import play), then World, USA, Europe.",
		Prefs: cleanPrefs(
			[]string{"Japan", "Asia", "World", "USA", "Europe"},
			[]string{"Ja", "En"}),
	},
	{
		ID: "world", Name: "World / English",
		Description: "Region-free releases first, then USA, Europe, Japan; English preferred.",
		Prefs: cleanPrefs(
			[]string{"World", "USA", "Europe", "UK", "Australia", "Canada", "Japan", "Asia"},
			[]string{"En"}),
	},
}

// PresetByID returns the preset with the given id.
func PresetByID(id string) (Preset, bool) {
	for _, p := range Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

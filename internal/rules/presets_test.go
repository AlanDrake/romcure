package rules

import (
	"testing"

	"romcure/internal/match"
)

func cand(key, region, lang string) Candidate {
	return Candidate{GameKey: key, Confidence: match.ConfidenceDATVerified, Region: region, Language: lang, IsVerifiedDump: true}
}

func TestPresetsPickExpectedVariant(t *testing.T) {
	group := []Candidate{
		cand("jp", "Japan", "Ja"),
		cand("us", "USA", "En"),
		cand("eu", "Europe", "En,Fr,De,Es,It"),
	}
	for id, want := range map[string]string{"usa": "us", "europe": "eu", "japan": "jp"} {
		p, ok := PresetByID(id)
		if !ok {
			t.Fatalf("no preset %s", id)
		}
		if got, reason := Select(group, p.Prefs); got != want {
			t.Errorf("%s picked %s (%s), want %s", id, got, reason, want)
		}
	}
}

func TestMultiRegionAndMultiLanguageRankByBestElement(t *testing.T) {
	// "USA, Europe" must count as USA under the USA preset and as Europe
	// under the Europe preset, beating a plain single-region sibling.
	group := []Candidate{
		cand("eu", "Europe", "En"),
		cand("usa-eu", "USA, Europe", "En"),
		cand("jp", "Japan", "Ja"),
	}
	usa, _ := PresetByID("usa")
	if got, _ := Select(group, usa.Prefs); got != "usa-eu" {
		t.Errorf("usa preset picked %s, want usa-eu", got)
	}
	eu, _ := PresetByID("europe")
	if got, _ := Select(group, eu.Prefs); got != "eu" && got != "usa-eu" {
		t.Errorf("europe preset picked %s", got)
	}

	langs := []Candidate{cand("a", "Europe", "En,Fr,De"), cand("b", "Europe", "Ja")}
	if got, _ := Select(langs, Preferences{RegionPriority: []string{"Europe"}, LanguagePriority: []string{"Fr", "En"}}); got != "a" {
		t.Errorf("language list ranking picked %s, want a", got)
	}
}

func TestPresetsExcludeBetaProtoDemoAndPreferLatest(t *testing.T) {
	for _, p := range Presets {
		if !p.Prefs.ExcludeBeta || !p.Prefs.ExcludeProto || !p.Prefs.ExcludeDemo || !p.Prefs.PreferLatestRevision {
			t.Errorf("%s: expected clean defaults, got %+v", p.ID, p.Prefs)
		}
		if p.Name == "" || p.Description == "" {
			t.Errorf("%s missing name/description", p.ID)
		}
	}
	if _, ok := PresetByID("nope"); ok {
		t.Error("unknown id must not resolve")
	}
}

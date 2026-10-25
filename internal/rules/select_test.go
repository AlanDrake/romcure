package rules

import (
	"strings"
	"testing"

	"romcure/internal/match"
)

func TestSelectPrefersHigherConfidence(t *testing.T) {
	group := []Candidate{
		{GameKey: "heuristic", Confidence: match.ConfidenceHeuristic},
		{GameKey: "verified", Confidence: match.ConfidenceDATVerified},
		{GameKey: "unidentified", Confidence: match.ConfidenceUnidentified},
	}
	got, reason := Select(group, Preferences{})
	if got != "verified" {
		t.Errorf("Select() = %q, want %q (reason: %s)", got, "verified", reason)
	}
	if reason == "" {
		t.Error("reason is empty, want an explanation (spec §5)")
	}
}

func TestSelectRegionPriority(t *testing.T) {
	group := []Candidate{
		{GameKey: "jpn", Confidence: match.ConfidenceDATVerified, Region: "Japan"},
		{GameKey: "usa", Confidence: match.ConfidenceDATVerified, Region: "USA"},
		{GameKey: "eur", Confidence: match.ConfidenceDATVerified, Region: "Europe"},
	}
	prefs := Preferences{RegionPriority: []string{"USA", "Europe", "Japan"}}

	got, _ := Select(group, prefs)
	if got != "usa" {
		t.Errorf("Select() = %q, want %q", got, "usa")
	}
}

func TestSelectRegionPriorityCaseInsensitive(t *testing.T) {
	group := []Candidate{
		{GameKey: "jpn", Confidence: match.ConfidenceDATVerified, Region: "Japan"},
		{GameKey: "usa", Confidence: match.ConfidenceDATVerified, Region: "USA"},
	}
	prefs := Preferences{RegionPriority: []string{"usa"}}

	got, _ := Select(group, prefs)
	if got != "usa" {
		t.Errorf("Select() = %q, want %q", got, "usa")
	}
}

func TestSelectUnlistedRegionSortsLast(t *testing.T) {
	group := []Candidate{
		{GameKey: "unlisted", Confidence: match.ConfidenceDATVerified, Region: "Brazil"},
		{GameKey: "listed", Confidence: match.ConfidenceDATVerified, Region: "Europe"},
	}
	prefs := Preferences{RegionPriority: []string{"USA", "Europe"}}

	got, _ := Select(group, prefs)
	if got != "listed" {
		t.Errorf("Select() = %q, want %q (an explicitly-ranked region beats an unlisted one)", got, "listed")
	}
}

func TestSelectLanguagePriorityIsTiebreakAfterRegion(t *testing.T) {
	group := []Candidate{
		{GameKey: "a", Confidence: match.ConfidenceDATVerified, Region: "Europe", Language: "Fr"},
		{GameKey: "b", Confidence: match.ConfidenceDATVerified, Region: "Europe", Language: "En"},
	}
	prefs := Preferences{LanguagePriority: []string{"En", "Fr"}}

	got, _ := Select(group, prefs)
	if got != "b" {
		t.Errorf("Select() = %q, want %q (same region, En ranked ahead of Fr)", got, "b")
	}
}

func TestSelectExcludesBetaProtoDemo(t *testing.T) {
	group := []Candidate{
		{GameKey: "beta", Confidence: match.ConfidenceDATVerified, IsBeta: true},
		{GameKey: "proto", Confidence: match.ConfidenceDATVerified, IsProto: true},
		{GameKey: "demo", Confidence: match.ConfidenceDATVerified, IsDemo: true},
		{GameKey: "final", Confidence: match.ConfidenceDATVerified},
	}
	prefs := Preferences{ExcludeBeta: true, ExcludeProto: true, ExcludeDemo: true}

	got, reason := Select(group, prefs)
	if got != "final" {
		t.Errorf("Select() = %q, want %q (reason: %s)", got, "final", reason)
	}
	if !strings.Contains(reason, "left out") {
		t.Errorf("reason %q does not mention exclusion", reason)
	}
}

func TestSelectRequireVerified(t *testing.T) {
	group := []Candidate{
		{GameKey: "unverified", Confidence: match.ConfidenceDATVerified, IsVerifiedDump: false},
		{GameKey: "verified", Confidence: match.ConfidenceDATVerified, IsVerifiedDump: true},
	}
	got, _ := Select(group, Preferences{RequireVerified: true})
	if got != "verified" {
		t.Errorf("Select() = %q, want %q", got, "verified")
	}
}

func TestSelectNeverReturnsNothingWhenExclusionsWouldEmptyGroup(t *testing.T) {
	// Every candidate is a beta; excluding betas would leave nothing.
	// The group must still get a selection.
	group := []Candidate{
		{GameKey: "beta1", Confidence: match.ConfidenceDATVerified, IsBeta: true},
		{GameKey: "beta2", Confidence: match.ConfidenceDATVerified, IsBeta: true, Region: "USA"},
	}
	prefs := Preferences{ExcludeBeta: true, RegionPriority: []string{"USA"}}

	got, reason := Select(group, prefs)
	if got == "" {
		t.Fatal("Select() returned no selection for a non-empty group")
	}
	if got != "beta2" {
		t.Errorf("Select() = %q, want %q (region preference should still apply among the un-excludable set)", got, "beta2")
	}
	if !strings.Contains(reason, "set aside") {
		t.Errorf("reason %q does not explain the exclusion-rules fallback", reason)
	}
}

func TestSelectIsDeterministicOnTies(t *testing.T) {
	group := []Candidate{
		{GameKey: "first", Confidence: match.ConfidenceDATVerified},
		{GameKey: "second", Confidence: match.ConfidenceDATVerified},
	}
	got, _ := Select(group, Preferences{})
	if got != "first" {
		t.Errorf("Select() = %q, want %q (stable tie-break keeps the first candidate)", got, "first")
	}
}

func TestSelectEmptyGroup(t *testing.T) {
	got, reason := Select(nil, Preferences{})
	if got != "" {
		t.Errorf("Select(nil) = %q, want \"\"", got)
	}
	if reason == "" {
		t.Error("reason is empty even for the empty-group case")
	}
}

func TestSelectPrefersLatestRevisionWhenEnabled(t *testing.T) {
	group := []Candidate{
		{GameKey: "rev0", Confidence: match.ConfidenceDATVerified, Revision: "Rev 0"},
		{GameKey: "rev2", Confidence: match.ConfidenceDATVerified, Revision: "Rev 2"},
		{GameKey: "rev1", Confidence: match.ConfidenceDATVerified, Revision: "Rev 1"},
	}
	got, _ := Select(group, Preferences{PreferLatestRevision: true})
	if got != "rev2" {
		t.Errorf("Select() = %q, want %q (highest revision number)", got, "rev2")
	}
}

func TestSelectIgnoresRevisionWhenDisabled(t *testing.T) {
	group := []Candidate{
		{GameKey: "first", Confidence: match.ConfidenceDATVerified, Revision: "Rev 0"},
		{GameKey: "second", Confidence: match.ConfidenceDATVerified, Revision: "Rev 2"},
	}
	got, _ := Select(group, Preferences{}) // PreferLatestRevision not set
	if got != "first" {
		t.Errorf("Select() = %q, want %q (revision ignored, stable tie-break keeps the first)", got, "first")
	}
}

func TestSelectRevisionIsTiebreakAfterRegionAndLanguage(t *testing.T) {
	group := []Candidate{
		{GameKey: "wrong-region", Confidence: match.ConfidenceDATVerified, Region: "Europe", Revision: "Rev 9"},
		{GameKey: "right-region", Confidence: match.ConfidenceDATVerified, Region: "USA", Revision: "Rev 0"},
	}
	prefs := Preferences{RegionPriority: []string{"USA"}, PreferLatestRevision: true}
	got, _ := Select(group, prefs)
	if got != "right-region" {
		t.Errorf("Select() = %q, want %q (region priority outranks revision)", got, "right-region")
	}
}

func TestRevisionRank(t *testing.T) {
	tests := []struct {
		revision string
		want     int
	}{
		{"Rev 0", 0},
		{"Rev 1", 1000000},
		{"Rev 12", 12000000},
		{"v1.1", 1001000},
		{"v1.10", 1010000},
		{"Rev A", -1},
		{"", -1},
	}
	for _, tt := range tests {
		t.Run(tt.revision, func(t *testing.T) {
			if got := revisionRank(tt.revision); got != tt.want {
				t.Errorf("revisionRank(%q) = %d, want %d", tt.revision, got, tt.want)
			}
		})
	}
}

func TestRevisionRankOrdersDottedVersions(t *testing.T) {
	if !(revisionRank("v1.9") < revisionRank("v1.10") && revisionRank("Rev 1") < revisionRank("v1.1") && revisionRank("v1.1") < revisionRank("v2.0")) {
		t.Error("dotted versions must order numerically, not lexically")
	}
}

func TestSelectPrefersCleanOverModifiedVariants(t *testing.T) {
	group := []Candidate{
		{GameKey: "cracked", Confidence: "dat_verified", Region: "Europe", VariantPenalty: 2},
		{GameKey: "alt", Confidence: "dat_verified", Region: "Europe", VariantPenalty: 1},
		{GameKey: "clean", Confidence: "dat_verified", Region: "Europe"},
	}
	if got, _ := Select(group, Preferences{}); got != "clean" {
		t.Errorf("picked %s, want clean", got)
	}
	// Region still outranks the penalty: a cracked USA copy beats a clean Japanese one for a USA preference.
	group = []Candidate{
		{GameKey: "jp-clean", Confidence: "dat_verified", Region: "Japan"},
		{GameKey: "us-cracked", Confidence: "dat_verified", Region: "USA", VariantPenalty: 2},
	}
	if got, _ := Select(group, Preferences{RegionPriority: []string{"USA"}}); got != "us-cracked" {
		t.Errorf("picked %s", got)
	}
}

package match

import "testing"

func TestIsTOSECName(t *testing.T) {
	for name, want := range map[string]bool{
		"Cannon Fodder v1.2 (1993)(Virgin)(EU)(en)[!]": true,
		"Game (19xx)(-)":          true,
		"Game (1993-05-12)(Pub)":  true,
		"Super Mario Bros. (USA)": false,
		"Game (Rev 1)":            false,
	} {
		if got := isTOSECName(name); got != want {
			t.Errorf("isTOSECName(%q) = %v", name, got)
		}
	}
}

func TestClassifyTOSEC(t *testing.T) {
	info := classify("Cannon Fodder v1.2 (1993)(Virgin)(EU-US)(en-de)(Disk 1 of 2)[cr CSL][a]")
	if info.region != "Europe, USA" || info.language != "En,De" || info.revision != "v1.2" || info.penalty != 2 {
		t.Errorf("info = %+v", info)
	}
	if clean := classify("Game (1990)(Pub)(US)(en)[!]"); clean.penalty != 0 || clean.region != "USA" {
		t.Errorf("clean = %+v", clean)
	}
	if alt := classify("Game (1990)(Pub)(JP)(ja)[a]"); alt.penalty != 1 || alt.region != "Japan" || alt.language != "Ja" {
		t.Errorf("alt = %+v", alt)
	}
	if dev := classify("Game (1990)(Pub)(beta)"); !dev.isBeta {
		t.Errorf("beta not detected: %+v", dev)
	}
	if dev := classify("Game (1990)(Pub)(proto)"); !dev.isProto {
		t.Error("proto not detected")
	}
	if dev := classify("Game (1990)(Pub)(demo-playable)"); !dev.isDemo {
		t.Error("demo not detected")
	}
	// The publisher slot is never mistaken for a country even when it
	// looks like a country code.
	if p := classify("Game (1990)(US)(DE)(de)"); p.region != "Germany" {
		t.Errorf("region = %q, want Germany (index 1 is the publisher)", p.region)
	}
	// A No-Intro name still goes through the old classifier.
	if ni := classify("Game (USA, Europe) (En,Fr)"); ni.region != "USA, Europe" || ni.language != "En,Fr" {
		t.Errorf("no-intro = %+v", ni)
	}
}

func TestHeuristicTOSECResult(t *testing.T) {
	r := MatchHeuristic("Cannon Fodder v1.2 (1993)(Virgin)(EU)(en)[cr].adf")
	if r.Confidence != ConfidenceHeuristic || r.VariantPenalty != 2 || r.Region != "Europe" {
		t.Errorf("result = %+v", r)
	}
}

func TestTOSECFlagsWithNumbersAndVirus(t *testing.T) {
	for name, want := range map[string]int{
		"Game (1990)(Pub)[b2 dump]":             2,
		"Game (1990)(Pub)[a4 cracktro]":         1,
		"Game (1990)(Pub)[cr][v Some virus]":    2,
		"Game (1990)(Pub)[m3 highscore]":        2,
		"Game (1990)(Pub)[budget][!]":           0,
		"Game (1990)(Pub)[bootable][data disk]": 0,
	} {
		if got := classify(name).penalty; got != want {
			t.Errorf("penalty(%q) = %d, want %d", name, got, want)
		}
	}
}

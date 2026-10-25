package match

import "testing"

func TestMatchHeuristic(t *testing.T) {
	tests := []struct {
		name           string
		filename       string
		wantConfidence Confidence
		wantTitle      string
		wantRegion     string
		wantLanguage   string
		wantRevision   string
		wantBeta       bool
		wantProto      bool
		wantDemo       bool
	}{
		{
			name:           "region tag",
			filename:       "Super Mario Bros. (USA).nes",
			wantConfidence: ConfidenceHeuristic,
			wantTitle:      "Super Mario Bros.",
			wantRegion:     "USA",
		},
		{
			name:           "language tag",
			filename:       "Game (En,Fr,De).nes",
			wantConfidence: ConfidenceHeuristic,
			wantTitle:      "Game",
			wantLanguage:   "En,Fr,De",
		},
		{
			name:           "region and revision tags",
			filename:       "Game (USA) (Rev 1).nes",
			wantConfidence: ConfidenceHeuristic,
			wantTitle:      "Game",
			wantRegion:     "USA",
			wantRevision:   "Rev 1",
		},
		{
			name:           "version-style revision tag",
			filename:       "Game (USA) (v1.1).nes",
			wantConfidence: ConfidenceHeuristic,
			wantTitle:      "Game",
			wantRegion:     "USA",
			wantRevision:   "v1.1",
		},
		{
			name:           "beta tag",
			filename:       "Game (USA) (Beta).nes",
			wantConfidence: ConfidenceHeuristic,
			wantTitle:      "Game",
			wantRegion:     "USA",
			wantBeta:       true,
		},
		{
			name:           "proto tag",
			filename:       "Game (USA) (Proto).nes",
			wantConfidence: ConfidenceHeuristic,
			wantTitle:      "Game",
			wantRegion:     "USA",
			wantProto:      true,
		},
		{
			name:           "demo tag",
			filename:       "Game (USA) (Demo).nes",
			wantConfidence: ConfidenceHeuristic,
			wantTitle:      "Game",
			wantRegion:     "USA",
			wantDemo:       true,
		},
		{
			name:           "unrecognized tag still counts as heuristic",
			filename:       "Game (Unlicensed).nes",
			wantConfidence: ConfidenceHeuristic,
			wantTitle:      "Game",
		},
		{
			name:           "no tags at all -> unidentified",
			filename:       "homebrew.gb",
			wantConfidence: ConfidenceUnidentified,
			wantTitle:      "homebrew",
		},
		{
			name:           "no extension, no tags",
			filename:       "justaname",
			wantConfidence: ConfidenceUnidentified,
			wantTitle:      "justaname",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchHeuristic(tt.filename)
			if got.Confidence != tt.wantConfidence {
				t.Errorf("Confidence = %v, want %v", got.Confidence, tt.wantConfidence)
			}
			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Region != tt.wantRegion {
				t.Errorf("Region = %q, want %q", got.Region, tt.wantRegion)
			}
			if got.Language != tt.wantLanguage {
				t.Errorf("Language = %q, want %q", got.Language, tt.wantLanguage)
			}
			if got.Revision != tt.wantRevision {
				t.Errorf("Revision = %q, want %q", got.Revision, tt.wantRevision)
			}
			if got.IsBeta != tt.wantBeta {
				t.Errorf("IsBeta = %v, want %v", got.IsBeta, tt.wantBeta)
			}
			if got.IsProto != tt.wantProto {
				t.Errorf("IsProto = %v, want %v", got.IsProto, tt.wantProto)
			}
			if got.IsDemo != tt.wantDemo {
				t.Errorf("IsDemo = %v, want %v", got.IsDemo, tt.wantDemo)
			}
		})
	}
}

func TestClassifyTagsMultiRegionAndLanguage(t *testing.T) {
	_, region, language, _, _, _, _ := classifyTags("Game (USA, Europe) (En,Fr,De) (Rev 1)")
	if region != "USA, Europe" || language != "En,Fr,De" {
		t.Errorf("region=%q language=%q", region, language)
	}
	_, region, _, _, _, _, _ = classifyTags("Game (Europe, Australia)")
	if region != "Europe, Australia" {
		t.Errorf("region=%q", region)
	}
	_, region, _, _, _, _, _ = classifyTags("Game (Not A Region, USA)")
	if region != "" {
		t.Errorf("a list with an unknown member must not count as a region, got %q", region)
	}
}

package match

import (
	"path/filepath"
	"regexp"
	"strings"
)

// knownRegions are the region tags scene-release filenames commonly carry
// in parentheses, e.g. "Super Mario Bros. (USA).nes".
var knownRegions = map[string]bool{
	"USA": true, "Europe": true, "Japan": true, "World": true, "Australia": true,
	"Brazil": true, "Canada": true, "China": true, "France": true, "Germany": true,
	"Italy": true, "Korea": true, "Netherlands": true, "Spain": true, "Sweden": true,
	"Taiwan": true, "UK": true, "Asia": true, "Russia": true, "Scandinavia": true,
	"Hong Kong": true, "Mexico": true, "Argentina": true, "India": true, "Poland": true,
	"Portugal": true, "Greece": true, "Denmark": true, "Finland": true, "Norway": true,
}

// knownLanguageCodes are No-Intro-style two-letter language codes, used
// alone or comma-joined, e.g. "(En,Fr,De)".
var knownLanguageCodes = map[string]bool{
	"En": true, "Fr": true, "De": true, "Es": true, "It": true, "Nl": true,
	"Pt": true, "Sv": true, "No": true, "Da": true, "Fi": true, "Zh": true,
	"Ja": true, "Ko": true, "Ru": true, "Pl": true,
}

var (
	tagPattern      = regexp.MustCompile(`\(([^)]*)\)`)
	revisionPattern = regexp.MustCompile(`(?i)^(rev\s*[\da-z]+|v[\d.]+)$`)
	betaPattern     = regexp.MustCompile(`(?i)^beta\b`)
	protoPattern    = regexp.MustCompile(`(?i)^proto(type)?\b`)
	demoPattern     = regexp.MustCompile(`(?i)^demo\b`)
)

// MatchHeuristic derives a best-effort title (and region/language/
// revision/beta-proto-demo when recognized) from filename using common
// scene-release naming conventions — parenthesized tags after the title.
// This is what keeps a DAT-less folder browsable (spec §2.6): finding at
// least one parenthesized tag at all is what sets Confidence to heuristic
// rather than unidentified, since it means the name likely follows a
// recognized convention even if a specific tag isn't one this function
// otherwise classifies.
func MatchHeuristic(filename string) Result {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))

	title := base
	if loc := tagPattern.FindStringIndex(base); loc != nil {
		title = strings.TrimSpace(base[:loc[0]])
	}
	if title == "" {
		title = base
	}

	info := classify(base)

	confidence := ConfidenceUnidentified
	if info.hasAnyTag {
		confidence = ConfidenceHeuristic
	}

	return Result{
		Confidence:     confidence,
		Title:          title,
		Region:         info.region,
		Language:       info.language,
		Revision:       info.revision,
		IsBeta:         info.isBeta,
		IsProto:        info.isProto,
		IsDemo:         info.isDemo,
		VariantPenalty: info.penalty,
	}
}

// classifyTags scans every parenthesized tag in s and classifies it as a
// region, language, revision, or beta/proto/demo marker. hasAnyTag is
// true if s contains at least one parenthesized tag at all, regardless
// of whether any of them were individually recognized — see
// MatchHeuristic's doc comment for why that distinction matters.
func classifyTags(s string) (hasAnyTag bool, region, language, revision string, isBeta, isProto, isDemo bool) {
	matches := tagPattern.FindAllStringSubmatch(s, -1)
	hasAnyTag = len(matches) > 0

	for _, m := range matches {
		tag := strings.TrimSpace(m[1])
		switch {
		case isRegionList(tag):
			region = tag
		case isLanguageList(tag):
			language = tag
		case revisionPattern.MatchString(tag):
			revision = tag
		case betaPattern.MatchString(tag):
			isBeta = true
		case protoPattern.MatchString(tag):
			isProto = true
		case demoPattern.MatchString(tag):
			isDemo = true
		}
	}
	return
}

func isLanguageList(tag string) bool {
	if tag == "" {
		return false
	}
	for _, code := range strings.Split(tag, ",") {
		if !knownLanguageCodes[strings.TrimSpace(code)] {
			return false
		}
	}
	return true
}

// isRegionList reports whether tag is one known region or a comma-separated
// list of them, as in No-Intro/Redump names such as "(USA, Europe)".
func isRegionList(tag string) bool {
	if tag == "" {
		return false
	}
	for _, r := range strings.Split(tag, ",") {
		if !knownRegions[strings.TrimSpace(r)] {
			return false
		}
	}
	return true
}

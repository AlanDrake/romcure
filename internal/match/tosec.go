package match

import (
	"regexp"
	"strings"
)

// tagInfo is everything we read from a release name's tags. For No-Intro /
// Redump style names it comes from classifyTags; for TOSEC names from
// classifyTOSEC (selected by classify).
type tagInfo struct {
	hasAnyTag bool
	region    string
	language  string
	revision  string
	isBeta    bool
	isProto   bool
	isDemo    bool
	// penalty ranks otherwise-equal variants: 0 = clean release, 1 = an
	// alternate dump ([a]), 2 = modified or damaged (cracked, hacked,
	// trained, fixed, translated, bad/over/underdump...). Lower wins.
	penalty int
}

// classify reads a release name's tags with the right naming convention.
func classify(name string) tagInfo {
	if isTOSECName(name) {
		return classifyTOSEC(name)
	}
	hasAny, region, language, revision, isBeta, isProto, isDemo := classifyTags(name)
	return tagInfo{hasAnyTag: hasAny, region: region, language: language, revision: revision,
		isBeta: isBeta, isProto: isProto, isDemo: isDemo}
}

// TOSEC names: Title [vX.Y] (date)(publisher)(country)(language)... [flags]
var (
	tosecDate    = regexp.MustCompile(`^(?:\d{4}|\d{3}x|\d{2}xx)(?:-(?:\d{2}|xx)){0,2}$`)
	tosecVersion = regexp.MustCompile(`\s(v\d[\w.]*|Rev\s?[\w.]+)\s*$`)
	tosecFlag    = regexp.MustCompile(`\[([a-zA-Z!]+)\d*(?:\s[^\]]*)?\]`)
	countryCodes = map[string]string{
		"US": "USA", "EU": "Europe", "GB": "UK", "JP": "Japan", "DE": "Germany", "FR": "France",
		"ES": "Spain", "IT": "Italy", "NL": "Netherlands", "SE": "Sweden", "AU": "Australia",
		"CA": "Canada", "BR": "Brazil", "KR": "Korea", "CN": "China", "TW": "Taiwan",
		"HK": "Hong Kong", "RU": "Russia", "PL": "Poland", "PT": "Portugal", "DK": "Denmark",
		"FI": "Finland", "NO": "Norway", "GR": "Greece", "AS": "Asia", "MX": "Mexico",
		"AR": "Argentina", "IN": "India",
	}
	tosecLangs = map[string]string{
		"en": "En", "fr": "Fr", "de": "De", "es": "Es", "it": "It", "nl": "Nl", "pt": "Pt",
		"sv": "Sv", "no": "No", "da": "Da", "fi": "Fi", "zh": "Zh", "ja": "Ja", "ko": "Ko",
		"ru": "Ru", "pl": "Pl",
	}
	modifiedFlags = map[string]bool{"cr": true, "h": true, "m": true, "t": true, "f": true,
		"o": true, "u": true, "b": true, "p": true, "tr": true, "v": true}
)

// isTOSECName reports whether name follows the TOSEC convention: the first
// parenthesized tag is a date such as (1993), (19xx) or (1993-05).
func isTOSECName(name string) bool {
	m := tagPattern.FindStringSubmatch(name)
	return m != nil && tosecDate.MatchString(strings.TrimSpace(m[1]))
}

func classifyTOSEC(name string) tagInfo {
	info := tagInfo{hasAnyTag: true}

	title := name
	if i := strings.IndexAny(name, "(["); i >= 0 {
		title = name[:i]
	}
	if m := tosecVersion.FindStringSubmatch(title); m != nil {
		info.revision = m[1]
	}

	groups := tagPattern.FindAllStringSubmatch(name, -1)
	for i, g := range groups {
		tag := strings.TrimSpace(g[1])
		low := strings.ToLower(tag)
		switch {
		case low == "alpha" || low == "beta" || low == "preview" || low == "pre-release":
			info.isBeta = true
		case low == "proto":
			info.isProto = true
		case strings.HasPrefix(low, "demo"):
			info.isDemo = true
		case i >= 2 && info.region == "" && countryTag(tag) != "":
			info.region = countryTag(tag)
		case i >= 2 && info.language == "" && langTag(tag) != "":
			info.language = langTag(tag)
		}
	}

	for _, f := range tosecFlag.FindAllStringSubmatch(name, -1) {
		flag := strings.ToLower(f[1])
		switch {
		case modifiedFlags[flag]:
			info.penalty = 2
		case flag == "a" && info.penalty < 1:
			info.penalty = 1
		}
	}
	return info
}

// countryTag turns "US" or "US-EU" into "USA, Europe"; "" if any part is not a
// known country code.
func countryTag(tag string) string {
	var out []string
	for _, p := range strings.Split(tag, "-") {
		name, ok := countryCodes[p]
		if !ok {
			return ""
		}
		out = append(out, name)
	}
	return strings.Join(out, ", ")
}

// langTag turns "en" or "en-de" into "En,De"; "" if any part is unknown.
func langTag(tag string) string {
	var out []string
	for _, p := range strings.Split(tag, "-") {
		name, ok := tosecLangs[p]
		if !ok {
			return ""
		}
		out = append(out, name)
	}
	return strings.Join(out, ",")
}

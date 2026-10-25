package profile

import "testing"

func TestDecideArchivePolicy(t *testing.T) {
	tests := []struct {
		name         string
		kind         string
		isOnlyMember bool
		isDiscSystem bool
		want         ArchivePolicy
	}{
		{"disc system always transcodes, even a clean zip", "zip", true, true, PolicyTranscode},
		{"zip with only the matched rom passes through", "zip", true, false, PolicyPassthrough},
		{"zip with extra files must extract", "zip", false, false, PolicyExtract},
		{"7z always extracts even when it's the only member", "sevenzip", true, false, PolicyExtract},
		{"7z with extra files extracts too", "sevenzip", false, false, PolicyExtract},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideArchivePolicy(tt.kind, tt.isOnlyMember, tt.isDiscSystem)
			if got != tt.want {
				t.Errorf("DecideArchivePolicy(%s, %v, %v) = %v, want %v", tt.kind, tt.isOnlyMember, tt.isDiscSystem, got, tt.want)
			}
		})
	}
}

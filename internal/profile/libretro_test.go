package profile

import "testing"

func TestLibretroDBName(t *testing.T) {
	tests := []struct {
		systemID string
		wantOK   bool
		wantName string
	}{
		{"nes", true, "Nintendo - Nintendo Entertainment System"},
		{"psx", true, "Sony - PlayStation"},
		{"totally-unknown-system", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.systemID, func(t *testing.T) {
			name, ok := LibretroDBName(tt.systemID)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestIsDiscSystem(t *testing.T) {
	tests := []struct {
		systemID string
		want     bool
	}{
		{"psx", true},
		{"ps2", true},
		{"dreamcast", true},
		{"nes", false},
		{"gba", false},
	}
	for _, tt := range tests {
		t.Run(tt.systemID, func(t *testing.T) {
			if got := IsDiscSystem(tt.systemID); got != tt.want {
				t.Errorf("IsDiscSystem(%q) = %v, want %v", tt.systemID, got, tt.want)
			}
		})
	}
}

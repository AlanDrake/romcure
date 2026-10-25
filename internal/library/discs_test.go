package library

import "testing"

func TestParseDiscTag(t *testing.T) {
	tests := []struct {
		title    string
		wantNum  int
		wantBase string
		wantOK   bool
	}{
		{"Final Fantasy VII (USA) (Disc 1)", 1, "Final Fantasy VII (USA)", true},
		{"Final Fantasy VII (USA) (Disc 2 of 3)", 2, "Final Fantasy VII (USA)", true},
		{"Final Fantasy VII (USA) (disc 3)", 3, "Final Fantasy VII (USA)", true},
		{"Quest of Ages (1987)(Publisher)(Disk 1 of 4 Side A)[a4]", 1, "Quest of Ages (1987)(Publisher)[a4]", true},
		{"Quest of Ages (1987)(Publisher)(Disk 3 of 4 Side B)", 6, "Quest of Ages (1987)(Publisher)", true},
		{"Quest of Ages (Disk 1 of 2)(Side B)", 2, "Quest of Ages", true},
		{"Quest of Ages (Disk 2 of 2) (side a)", 3, "Quest of Ages", true},
		{"Quest of Ages (Disc 2)(Side B)", 4, "Quest of Ages", true},
		{"Quest of Ages (Side A)", 1, "Quest of Ages", true},
		{"Quest of Ages (1987)(Side B)(Foo)", 2, "Quest of Ages (1987)(Foo)", true},
		{"Quest of Ages (Tape 1 of 2)", 1, "Quest of Ages", true},
		{"Quest of Ages (Tape 2 of 2 Side A)", 3, "Quest of Ages", true},
		{"Quest of Ages (Disk 4)", 4, "Quest of Ages", true},
		{"Quest of Ages (Sidebar)", 0, "", false},
		{"Super Game (USA)", 0, "", false},
		{"Just A Game", 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			num, base, ok := ParseDiscTag(tt.title)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if num != tt.wantNum {
				t.Errorf("num = %d, want %d", num, tt.wantNum)
			}
			if base != tt.wantBase {
				t.Errorf("base = %q, want %q", base, tt.wantBase)
			}
		})
	}
}

func TestSetTitleDropsDumpFlags(t *testing.T) {
	for in, want := range map[string]string{
		"Quest of Ages (1987)(Publisher)[a4]":            "Quest of Ages (1987)(Publisher)",
		"Quest of Ages (1987)(Publisher) [cr Group] [!]": "Quest of Ages (1987)(Publisher)",
		"Final Fantasy VII (USA)":                        "Final Fantasy VII (USA)",
	} {
		if got := SetTitle(in); got != want {
			t.Errorf("SetTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

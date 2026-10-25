package dat

import (
	"strings"
	"testing"
)

const logiqxNonPC = `<?xml version="1.0"?>
<!DOCTYPE datafile PUBLIC "-//Logiqx//DTD ROM Management Datafile//EN" "http://www.logiqx.com/Dats/datafile.dtd">
<datafile>
	<header>
		<name>Nintendo - Nintendo Entertainment System</name>
	</header>
	<game name="Super Game (USA)">
		<description>Super Game (USA)</description>
		<rom name="Super Game (USA).nes" size="131088" crc="a1b2c3d4" md5="aabbccdd" sha1="112233"/>
	</game>
	<game name="Super Game (Japan)">
		<description>Super Game (Japan)</description>
		<rom name="Super Game (Japan).nes" size="131088" crc="e5f6a7b8"/>
	</game>
</datafile>`

const logiqxPC = `<?xml version="1.0"?>
<datafile>
	<header>
		<name>Nintendo - Nintendo Entertainment System (Parent-Clone)</name>
	</header>
	<game name="Super Game (USA)">
		<rom name="Super Game (USA).nes" size="131088" crc="a1b2c3d4"/>
	</game>
	<game name="Super Game (Japan)" cloneof="Super Game (USA)">
		<rom name="Super Game (Japan).nes" size="131088" crc="e5f6a7b8"/>
	</game>
</datafile>`

const clrMameProNonPC = `clrmamepro (
	name "Nintendo - Nintendo Entertainment System"
	description "Nintendo - NES"
	version 20261001
)

game (
	name "Super Game (USA)"
	description "Super Game (USA)"
	rom ( name "Super Game (USA).nes" size 131088 crc a1b2c3d4 md5 aabbccdd sha1 112233 )
)

// a comment between entries
game (
	name "Super Game (Japan)"
	description "Super Game (Japan)"
	rom ( name "Super Game (Japan).nes" size 131088 crc e5f6a7b8 )
)
`

const clrMameProPC = `clrmamepro (
	name "Nintendo - NES (Parent-Clone)"
)

game (
	name "Super Game (USA)"
	rom ( name "Super Game (USA).nes" size 131088 crc a1b2c3d4 )
)

game (
	name "Super Game (Japan)"
	cloneof "Super Game (USA)"
	rom ( name "Super Game (Japan).nes" size 131088 crc e5f6a7b8 )
)
`

func TestParseLogiqxXMLNonPC(t *testing.T) {
	df, err := Parse(strings.NewReader(logiqxNonPC))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checkParsedNonPC(t, df, FormatLogiqxXML, "Nintendo - Nintendo Entertainment System")
}

func TestParseLogiqxXMLPC(t *testing.T) {
	df, err := Parse(strings.NewReader(logiqxPC))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checkParsedPC(t, df, FormatLogiqxXML)
}

func TestParseClrMameProNonPC(t *testing.T) {
	df, err := Parse(strings.NewReader(clrMameProNonPC))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checkParsedNonPC(t, df, FormatClrMamePro, "Nintendo - Nintendo Entertainment System")
}

func TestParseClrMameProPC(t *testing.T) {
	df, err := Parse(strings.NewReader(clrMameProPC))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checkParsedPC(t, df, FormatClrMamePro)
}

// checkParsedNonPC and checkParsedPC assert the same structural
// expectations regardless of which format produced them — the two
// formats must parse to equivalent Datafile values.

func checkParsedNonPC(t *testing.T, df *Datafile, wantFormat Format, wantSystemName string) {
	t.Helper()
	if df.Format != wantFormat {
		t.Errorf("Format = %v, want %v", df.Format, wantFormat)
	}
	if df.SystemName != wantSystemName {
		t.Errorf("SystemName = %q, want %q", df.SystemName, wantSystemName)
	}
	if df.HasParentClone {
		t.Error("HasParentClone = true, want false (no cloneof in this DAT)")
	}
	if len(df.Games) != 2 {
		t.Fatalf("len(Games) = %d, want 2", len(df.Games))
	}

	usa := df.Games[0]
	if usa.Name != "Super Game (USA)" {
		t.Errorf("Games[0].Name = %q, want %q", usa.Name, "Super Game (USA)")
	}
	if usa.CloneOf != "" {
		t.Errorf("Games[0].CloneOf = %q, want \"\"", usa.CloneOf)
	}
	if len(usa.Roms) != 1 {
		t.Fatalf("len(Games[0].Roms) = %d, want 1", len(usa.Roms))
	}
	rom := usa.Roms[0]
	if rom.Name != "Super Game (USA).nes" || rom.Size != 131088 || rom.CRC32 != "a1b2c3d4" || rom.MD5 != "aabbccdd" || rom.SHA1 != "112233" {
		t.Errorf("Games[0].Roms[0] = %+v, unexpected", rom)
	}

	jpn := df.Games[1]
	if jpn.Name != "Super Game (Japan)" || jpn.Roms[0].CRC32 != "e5f6a7b8" {
		t.Errorf("Games[1] = %+v, unexpected", jpn)
	}
}

func checkParsedPC(t *testing.T, df *Datafile, wantFormat Format) {
	t.Helper()
	if df.Format != wantFormat {
		t.Errorf("Format = %v, want %v", df.Format, wantFormat)
	}
	if !df.HasParentClone {
		t.Error("HasParentClone = false, want true (Japan is a clone of USA)")
	}
	if len(df.Games) != 2 {
		t.Fatalf("len(Games) = %d, want 2", len(df.Games))
	}
	if df.Games[0].CloneOf != "" {
		t.Errorf("Games[0] (USA, the parent).CloneOf = %q, want \"\"", df.Games[0].CloneOf)
	}
	if df.Games[1].CloneOf != "Super Game (USA)" {
		t.Errorf("Games[1] (Japan, the clone).CloneOf = %q, want %q", df.Games[1].CloneOf, "Super Game (USA)")
	}
}

func TestParseClrMameProEscapedQuotes(t *testing.T) {
	src := `game (
	name "Super Game \"Classic Edition\" (USA)"
	rom ( name "Super Game.nes" size 1 crc 00000000 )
)
`
	df, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := `Super Game "Classic Edition" (USA)`
	if len(df.Games) != 1 || df.Games[0].Name != want {
		t.Errorf("Games = %+v, want a single game named %q", df.Games, want)
	}
}

func TestParseClrMameProUnknownBlockIsSkipped(t *testing.T) {
	src := `resource (
	name "some resource"
	nested ( a b )
)

game (
	name "Super Game (USA)"
	rom ( name "Super Game.nes" size 1 crc 00000000 )
)
`
	df, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(df.Games) != 1 || df.Games[0].Name != "Super Game (USA)" {
		t.Errorf("Games = %+v, want a single game named %q", df.Games, "Super Game (USA)")
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"malformed xml", `<datafile><game name="x"`},
		{"unterminated clrmamepro block", `game ( name "x"`},
		{"unterminated quoted string", `game ( name "x )`},
		{"missing paren after block name", `game name "x" )`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(tt.src)); err == nil {
				t.Errorf("Parse(%q) = nil error, want an error", tt.src)
			}
		})
	}
}

func TestParseDetectsFormatFromLeadingAngleBracket(t *testing.T) {
	// Leading whitespace/BOM before the detection byte must not confuse
	// format detection.
	src := "\n\n   " + logiqxNonPC
	df, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if df.Format != FormatLogiqxXML {
		t.Errorf("Format = %v, want %v", df.Format, FormatLogiqxXML)
	}
}

func TestSerialsAreParsed(t *testing.T) {
	clr := `clrmamepro ( name "GBA" )
game (
	name "Example Quest (USA, Europe)"
	serial "AXQE"
	rom ( name "Example Quest (USA, Europe).gba" size 8 crc a1b2c3d4 )
	rom ( name "Example Extra.gba" size 8 crc a1b2c3d5 serial "XTRA" )
)
game (
	name "No Serial (USA)"
	rom ( name "No Serial (USA).gba" size 8 crc a1b2c3d6 )
)`
	xml := `<datafile><header><name>GBA</name></header>
	<game name="Example Quest (USA, Europe)"><serial>AXQE</serial>
		<rom name="a.gba" size="8" crc="a1b2c3d4"/>
		<rom name="b.gba" size="8" crc="a1b2c3d5" serial="XTRA"/></game>
	<game name="No Serial (USA)"><rom name="c.gba" size="8" crc="a1b2c3d6"/></game>
</datafile>`
	for name, src := range map[string]string{"clrmamepro": clr, "logiqx": xml} {
		t.Run(name, func(t *testing.T) {
			df, err := Parse(strings.NewReader(src))
			if err != nil {
				t.Fatal(err)
			}
			got := []string{df.Games[0].Roms[0].Serial, df.Games[0].Roms[1].Serial, df.Games[1].Roms[0].Serial}
			want := []string{"AXQE", "XTRA", ""}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("serials = %q, want %q", got, want)
					break
				}
			}
		})
	}
}

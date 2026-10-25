package library

import "testing"

func TestGameKeyDeterministic(t *testing.T) {
	a := GameKey([]string{"a1b2c3d4"})
	b := GameKey([]string{"a1b2c3d4"})
	if a != b {
		t.Errorf("GameKey is not deterministic: %q != %q", a, b)
	}
}

func TestGameKeyOrderIndependent(t *testing.T) {
	a := GameKey([]string{"track1crc", "track2crc", "track3crc"})
	b := GameKey([]string{"track3crc", "track1crc", "track2crc"})
	if a != b {
		t.Errorf("GameKey depends on input order: %q != %q", a, b)
	}
}

func TestGameKeyDiffersForDifferentContent(t *testing.T) {
	a := GameKey([]string{"a1b2c3d4"})
	b := GameKey([]string{"e5f6a7b8"})
	if a == b {
		t.Error("GameKey gave the same key for different content hashes")
	}
}

func TestGameKeyNoMemberConcatenationCollision(t *testing.T) {
	// Without a separator between hashes, ["ab","c"] and ["a","bc"] would
	// hash identically after concatenation. The separator byte must
	// prevent that.
	a := GameKey([]string{"ab", "c"})
	b := GameKey([]string{"a", "bc"})
	if a == b {
		t.Error("GameKey collided for differently-split inputs that concatenate to the same bytes")
	}
}

func TestGameKeySingleHashIsTheUnidentifiedCase(t *testing.T) {
	// spec §2.11: heuristic/unidentified identity is "content hash of the
	// file" — the trivial one-member case of the general rule.
	single := GameKey([]string{"deadbeef"})
	if single == "" {
		t.Error("GameKey of a single hash is empty")
	}
}

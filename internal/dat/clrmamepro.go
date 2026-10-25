package dat

import (
	"fmt"
	"strconv"
	"strings"
)

type tokenKind int

const (
	tokWord tokenKind = iota
	tokQuoted
	tokLParen
	tokRParen
)

type token struct {
	kind  tokenKind
	value string
}

func isDelim(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '(' || c == ')'
}

// tokenizeClrMamePro lexes a ClrMamePro-format DAT: `//` line comments,
// `(` and `)`, double-quoted strings (with `\"` escaping), and bare words.
func tokenizeClrMamePro(data []byte) ([]token, error) {
	var toks []token
	i, n := 0, len(data)

	for i < n {
		c := data[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '/' && i+1 < n && data[i+1] == '/':
			for i < n && data[i] != '\n' {
				i++
			}
		case c == '(':
			toks = append(toks, token{kind: tokLParen})
			i++
		case c == ')':
			toks = append(toks, token{kind: tokRParen})
			i++
		case c == '"':
			var sb strings.Builder
			j := i + 1
			for j < n && data[j] != '"' {
				if data[j] == '\\' && j+1 < n {
					sb.WriteByte(data[j+1])
					j += 2
					continue
				}
				sb.WriteByte(data[j])
				j++
			}
			if j >= n {
				return nil, fmt.Errorf("unterminated quoted string starting at byte %d", i)
			}
			toks = append(toks, token{kind: tokQuoted, value: sb.String()})
			i = j + 1
		default:
			j := i
			for j < n && !isDelim(data[j]) {
				j++
			}
			toks = append(toks, token{kind: tokWord, value: string(data[i:j])})
			i = j
		}
	}
	return toks, nil
}

// readKeyValues reads flat `key value key value ...` pairs starting at
// pos until the matching ')' (consumed), returning the pairs and the
// position just after it.
func readKeyValues(toks []token, pos int) (map[string]string, int, error) {
	kv := make(map[string]string)
	for pos < len(toks) && toks[pos].kind != tokRParen {
		if toks[pos].kind == tokLParen {
			return nil, 0, fmt.Errorf("unexpected nested '(' at token %d", pos)
		}
		key := toks[pos].value
		pos++
		if pos >= len(toks) {
			return nil, 0, fmt.Errorf("unexpected end of input after key %q", key)
		}
		if toks[pos].kind == tokLParen || toks[pos].kind == tokRParen {
			return nil, 0, fmt.Errorf("expected a value after key %q", key)
		}
		kv[key] = toks[pos].value
		pos++
	}
	if pos >= len(toks) {
		return nil, 0, fmt.Errorf("unterminated block")
	}
	return kv, pos + 1, nil // +1 to consume ')'
}

// skipBlock advances pos past a block whose '(' has already been
// consumed, to just after its matching ')'. Used for top-level block
// kinds this parser doesn't otherwise care about.
func skipBlock(toks []token, pos int) (int, error) {
	depth := 1
	for pos < len(toks) && depth > 0 {
		switch toks[pos].kind {
		case tokLParen:
			depth++
		case tokRParen:
			depth--
		}
		pos++
	}
	if depth != 0 {
		return 0, fmt.Errorf("unterminated block")
	}
	return pos, nil
}

func parseGameBlock(toks []token, pos int) (Game, int, error) {
	var game Game
	// A game-level serial applies to the ROMs that do not carry their own.
	var gameSerial string

	for pos < len(toks) && toks[pos].kind != tokRParen {
		if toks[pos].kind == tokWord && toks[pos].value == "rom" {
			pos++
			if pos >= len(toks) || toks[pos].kind != tokLParen {
				return Game{}, 0, fmt.Errorf("expected '(' after \"rom\"")
			}
			pos++

			romKV, newPos, err := readKeyValues(toks, pos)
			if err != nil {
				return Game{}, 0, fmt.Errorf("parse rom block: %w", err)
			}
			pos = newPos

			rom := Rom{
				Name:   romKV["name"],
				CRC32:  strings.ToLower(romKV["crc"]),
				SHA1:   strings.ToLower(romKV["sha1"]),
				MD5:    strings.ToLower(romKV["md5"]),
				Status: romKV["status"],
				Serial: romKV["serial"],
			}
			if s, ok := romKV["size"]; ok {
				if n, err := strconv.ParseInt(s, 10, 64); err == nil {
					rom.Size = n
				}
			}
			game.Roms = append(game.Roms, rom)
			continue
		}

		// Flat key-value pair at game level (name, description, cloneof, ...).
		key := toks[pos].value
		pos++
		if pos >= len(toks) {
			return Game{}, 0, fmt.Errorf("unexpected end of input after key %q", key)
		}
		val := toks[pos].value
		pos++
		switch key {
		case "name":
			game.Name = val
		case "cloneof":
			game.CloneOf = val
		case "serial":
			gameSerial = val
		}
	}
	for i := range game.Roms {
		if game.Roms[i].Serial == "" {
			game.Roms[i].Serial = gameSerial
		}
	}
	if pos >= len(toks) {
		return Game{}, 0, fmt.Errorf("unterminated game block")
	}
	return game, pos + 1, nil // +1 to consume ')'
}

func parseClrMamePro(data []byte) (*Datafile, error) {
	toks, err := tokenizeClrMamePro(data)
	if err != nil {
		return nil, fmt.Errorf("tokenize clrmamepro dat: %w", err)
	}

	df := &Datafile{Format: FormatClrMamePro}

	pos := 0
	for pos < len(toks) {
		if toks[pos].kind != tokWord {
			return nil, fmt.Errorf("expected a block name, got %q", toks[pos].value)
		}
		blockName := toks[pos].value
		pos++

		if pos >= len(toks) || toks[pos].kind != tokLParen {
			return nil, fmt.Errorf("expected '(' after %q", blockName)
		}
		pos++

		switch blockName {
		case "clrmamepro":
			kv, newPos, err := readKeyValues(toks, pos)
			if err != nil {
				return nil, fmt.Errorf("parse clrmamepro header: %w", err)
			}
			pos = newPos
			df.SystemName = kv["name"]

		case "game", "machine":
			game, newPos, err := parseGameBlock(toks, pos)
			if err != nil {
				return nil, fmt.Errorf("parse %s block: %w", blockName, err)
			}
			pos = newPos
			if game.CloneOf != "" {
				df.HasParentClone = true
			}
			df.Games = append(df.Games, game)

		default:
			pos, err = skipBlock(toks, pos)
			if err != nil {
				return nil, fmt.Errorf("skip %s block: %w", blockName, err)
			}
		}
	}
	return df, nil
}

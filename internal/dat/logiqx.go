package dat

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

type xmlDatafile struct {
	Header xmlHeader `xml:"header"`
	Games  []xmlGame `xml:"game"`
}

type xmlHeader struct {
	Name string `xml:"name"`
}

type xmlGame struct {
	Name    string   `xml:"name,attr"`
	CloneOf string   `xml:"cloneof,attr"`
	Roms    []xmlRom `xml:"rom"`
	Serial  string   `xml:"serial"` // <serial> child, used when a rom has no serial attribute
}

type xmlRom struct {
	Name   string `xml:"name,attr"`
	Size   int64  `xml:"size,attr"`
	CRC32  string `xml:"crc,attr"`
	SHA1   string `xml:"sha1,attr"`
	MD5    string `xml:"md5,attr"`
	Status string `xml:"status,attr"`
	Serial string `xml:"serial,attr"`
}

func parseLogiqxXML(data []byte) (*Datafile, error) {
	var x xmlDatafile
	if err := xml.NewDecoder(bytes.NewReader(data)).Decode(&x); err != nil {
		return nil, fmt.Errorf("decode logiqx xml: %w", err)
	}

	df := &Datafile{Format: FormatLogiqxXML, SystemName: x.Header.Name}
	for _, g := range x.Games {
		game := Game{Name: g.Name, CloneOf: g.CloneOf}
		if game.CloneOf != "" {
			df.HasParentClone = true
		}
		for _, r := range g.Roms {
			serial := r.Serial
			if serial == "" {
				serial = strings.TrimSpace(g.Serial)
			}
			game.Roms = append(game.Roms, Rom{
				Name:   r.Name,
				Size:   r.Size,
				CRC32:  strings.ToLower(r.CRC32),
				SHA1:   strings.ToLower(r.SHA1),
				MD5:    strings.ToLower(r.MD5),
				Status: r.Status,
				Serial: serial,
			})
		}
		df.Games = append(df.Games, game)
	}
	return df, nil
}

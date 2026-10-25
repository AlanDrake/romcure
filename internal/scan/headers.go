package scan

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"romcure/internal/discid"
	"romcure/internal/rararc"
	"romcure/internal/romheader"
	"romcure/internal/store"

	"github.com/bodgit/sevenzip"
)

// hashFile returns the CRC32 of the whole file and, for a cartridge format
// that may carry a copier header (romheader), the CRC of the data after it;
// both in one read.
func hashFile(path string, size int64) (crc string, alt romheader.Result, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", alt, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := crc32.NewIEEE()
	r := io.TeeReader(f, h)
	if romheader.MayHaveHeader(path) {
		if alt, _, err = romheader.Hash(path, r, size); err != nil {
			return "", alt, fmt.Errorf("read %s: %w", path, err)
		}
	}
	if _, err := io.Copy(io.Discard, r); err != nil { // whatever Hash did not read
		return "", alt, fmt.Errorf("read %s: %w", path, err)
	}
	return fmt.Sprintf("%08x", h.Sum32()), alt, nil
}

// addHeaderless fills AltCRC32/AltSize for the members that may carry a
// copier header, reading only those (they are small cartridge ROMs). open
// returns a member's content.
func addHeaderless(members []store.ArchiveMemberRecord, open func(name string) (io.ReadCloser, error)) error {
	for i := range members {
		m := &members[i]
		if !romheader.MayHaveHeader(m.MemberName) {
			continue
		}
		rc, err := open(m.MemberName)
		if err != nil {
			if skippable(err) {
				continue // packed with a method Go cannot read: no copier-header check
			}
			return fmt.Errorf("read %s: %w", m.MemberName, err)
		}
		res, ok, err := romheader.Hash(m.MemberName, rc, m.Size)
		rc.Close()
		if err != nil {
			return fmt.Errorf("read %s: %w", m.MemberName, err)
		}
		if ok {
			m.AltCRC32, m.AltSize = res.CRC32, res.Size
		}
	}
	return nil
}

// readDiscFile reads a raw file's disc header (GameCube/Wii ID and title, or
// a PlayStation serial), only the few sectors that hold it. An Info with an
// empty ID means the file has none.
func readDiscFile(path string) (discid.Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return discid.Info{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return discid.Info{}, err
	}
	info, _ := discid.ReadAt(path, f, st.Size())
	return info, nil
}

// memberOpener opens members of the archive at path, plus a closer for the
// archive itself.
func (s *Scanner) memberOpener(path, kind string) (func(string) (io.ReadCloser, error), func() error, error) {
	switch kind {
	case "zip":
		zr, err := zip.OpenReader(path)
		if err != nil {
			return nil, nil, fmt.Errorf("open zip %s: %w", path, err)
		}
		return func(n string) (io.ReadCloser, error) { return zr.Open(n) }, zr.Close, nil
	case "sevenzip":
		r, err := sevenzip.OpenReader(path)
		if err != nil {
			return nil, nil, fmt.Errorf("open 7z %s: %w", path, err)
		}
		return func(n string) (io.ReadCloser, error) { return r.Open(n) }, r.Close, nil
	case "rar":
		tool := rararc.Tool{Path: s.SevenZip}
		return func(n string) (io.ReadCloser, error) { return tool.Open(context.Background(), path, n) },
			func() error { return nil }, nil
	}
	return nil, nil, fmt.Errorf("%s is not an archive", path)
}

// addDiscHeaders reads the disc header of each member that may be a
// GameCube/Wii/PlayStation image and has not been looked at yet (only its start).
func (s *Scanner) addDiscHeaders(path, kind string, members []store.ArchiveMemberRecord) error {
	var todo []int
	for i, m := range members {
		if !m.DiscID.Valid && discid.MayHaveAny(m.MemberName) {
			todo = append(todo, i)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	open, closeArchive, err := s.memberOpener(path, kind)
	if err != nil {
		return err
	}
	defer closeArchive()
	for _, i := range todo {
		rc, err := open(members[i].MemberName)
		if err != nil {
			if skippable(err) { // unreadable here: record "no header" so it is not retried
				members[i].DiscID = sql.NullString{Valid: true}
				continue
			}
			return fmt.Errorf("read %s: %w", members[i].MemberName, err)
		}
		info, _ := discid.ReadAny(members[i].MemberName, rc)
		rc.Close()
		members[i].DiscID, members[i].DiscTitle, members[i].DiscNumber = sql.NullString{String: info.ID, Valid: true}, info.Title, info.Disc
	}
	return nil
}

// fillDiscHeaders records the disc headers (and a zip's compression methods) of a file scanned before they were
// recorded, without rehashing it.
func (s *Scanner) fillDiscHeaders(f store.FileRecord) error {
	if f.Kind == "zip" && !f.ZipMethod.Valid {
		method, err := zipOldMethod(f.Path)
		if err != nil {
			return err
		}
		if err := s.db.SetFileZipMethod(f.ID, method); err != nil {
			return err
		}
	}
	if f.Kind == "raw" {
		info, err := readDiscFile(f.Path)
		if err != nil {
			return err
		}
		return s.db.SetFileDisc(f.ID, info.ID, info.Title, info.Disc)
	}
	members, err := s.db.ListArchiveMembers(f.ID)
	if err != nil {
		return err
	}
	before := make([]bool, len(members))
	for i, m := range members {
		before[i] = m.DiscID.Valid
	}
	if err := s.addDiscHeaders(f.Path, f.Kind, members); err != nil {
		return err
	}
	for i, m := range members {
		if !before[i] && m.DiscID.Valid {
			if err := s.db.SetMemberDisc(f.ID, m.MemberName, m.DiscID.String, m.DiscTitle, m.DiscNumber); err != nil {
				return err
			}
		}
	}
	return nil
}

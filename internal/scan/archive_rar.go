package scan

import (
	"context"
	"fmt"
	"io"

	"romcure/internal/rararc"
	"romcure/internal/store"
)

// readRARMembers lists a RAR's contents from its headers through 7-Zip, which
// reads them without decompressing anything (spec §2.8).
func (s *Scanner) readRARMembers(path string) ([]store.ArchiveMemberRecord, error) {
	list, err := rararc.Tool{Path: s.SevenZip}.List(context.Background(), path)
	if err != nil {
		return nil, fmt.Errorf("open rar %s: %w", path, err)
	}
	members := make([]store.ArchiveMemberRecord, 0, len(list))
	for _, m := range list {
		members = append(members, store.ArchiveMemberRecord{MemberName: m.Name, Size: m.Size, CRC32: m.CRC32})
	}
	tool := rararc.Tool{Path: s.SevenZip}
	err = addHeaderless(members, func(name string) (io.ReadCloser, error) { return tool.Open(context.Background(), path, name) })
	return members, err
}

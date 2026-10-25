package scan

import (
	"fmt"
	"io"

	"github.com/bodgit/sevenzip"

	"romcure/internal/store"
)

// readSevenZipMembers lists a 7z's contents from its header — CRC32 and
// uncompressed size are stored there, so this never decompresses a member
// (spec §2.8).
func readSevenZipMembers(path string) ([]store.ArchiveMemberRecord, error) {
	r, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open 7z %s: %w", path, err)
	}
	defer r.Close()

	var members []store.ArchiveMemberRecord
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		members = append(members, store.ArchiveMemberRecord{
			MemberName: f.Name,
			Size:       int64(f.UncompressedSize),
			CRC32:      fmt.Sprintf("%08x", f.CRC32),
		})
	}
	err = addHeaderless(members, func(name string) (io.ReadCloser, error) { return r.Open(name) })
	return members, err
}

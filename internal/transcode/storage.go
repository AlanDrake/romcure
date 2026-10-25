package transcode

import (
	"io/fs"
	"os"
	"path/filepath"

	"romcure/internal/store"
)

// Storage is the storage panel's numbers (spec M4): what the sources
// weigh, what the transcode cache weighs, and what the profile outputs
// actually occupy on disk.
type Storage struct {
	SourceBytes  int64
	CacheBytes   int64
	CacheEntries int
	// OutputBytes counts only real files in the outputs (copies, generated
	// playlists). Symlinks and hardlinks to sources/cache are not counted a
	// second time.
	OutputBytes int64
}

// MeasureStorage totals the scanned size under sourceRoots, the cache, and
// the real files under outputRoots.
func MeasureStorage(db *store.DB, cache *Cache, sourceRoots, outputRoots []string) (Storage, error) {
	var s Storage
	for _, root := range sourceRoots {
		files, err := db.ListFilesUnderRoot(root)
		if err != nil {
			return s, err
		}
		for _, f := range files {
			s.SourceBytes += f.Size
		}
	}
	u, err := cache.Usage()
	if err != nil {
		return s, err
	}
	s.CacheBytes, s.CacheEntries = u.Bytes, u.Entries

	for _, root := range outputRoots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.Type().IsRegular() {
				if info, err := d.Info(); err == nil {
					s.OutputBytes += info.Size()
				}
			}
			return nil
		})
		if err != nil {
			return s, err
		}
	}
	return s, nil
}

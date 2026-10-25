package transcode

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"romcure/internal/store"
)

// Format is a transcode output format (matches transcode_cache.format).
type Format string

const (
	FormatCHD Format = "chd"
	FormatRVZ Format = "rvz"
	FormatCSO Format = "cso"
)

// CacheKey is the content-keyed identity of a transcode result (spec
// §2.10): the hash of the *uncompressed disc content*, not of the source
// archive, plus everything that changes the output bytes. A re-zipped
// source therefore hits the cache; a new tool version or codec flags miss.
func CacheKey(contentHash string, format Format, toolVersion string, codecFlags []string) string {
	h := sha256.New()
	for _, part := range []string{strings.ToLower(contentHash), string(format), toolVersion, strings.Join(codecFlags, " ")} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// CacheEntry mirrors a transcode_cache row.
type CacheEntry struct {
	Key         string
	ContentHash string
	Format      Format
	ToolName    string
	ToolVersion string
	CodecFlags  string
	Path        string
	Verified    bool
}

// Cache is the transcode cache: files under Dir, indexed in the DB.
type Cache struct {
	Dir string
	DB  *store.DB
}

// PathFor returns where the output for key lives. Sharded by the first
// two key characters so a huge library doesn't produce one giant directory.
func (c *Cache) PathFor(key string, format Format) string {
	return filepath.Join(c.Dir, key[:2], key+"."+string(format))
}

// Lookup returns the verified entry for key whose file still exists. An
// unverified row or a missing file is a miss, so the job re-runs rather
// than linking something unproven (spec §2.10).
func (c *Cache) Lookup(key string) (*CacheEntry, error) {
	var e CacheEntry
	var verified int
	var format string
	err := c.DB.QueryRow(`SELECT cache_key, content_hash, format, tool_name, tool_version,
		codec_flags, cache_path, verified FROM transcode_cache WHERE cache_key = ?`, key).
		Scan(&e.Key, &e.ContentHash, &format, &e.ToolName, &e.ToolVersion, &e.CodecFlags, &e.Path, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup transcode cache %s: %w", key, err)
	}
	e.Format, e.Verified = Format(format), verified == 1
	if !e.Verified {
		return nil, nil
	}
	if _, err := os.Stat(e.Path); err != nil {
		return nil, nil
	}
	return &e, nil
}

// Put records (or replaces) an entry.
func (c *Cache) Put(e CacheEntry) error {
	verified := 0
	if e.Verified {
		verified = 1
	}
	_, err := c.DB.Exec(`INSERT INTO transcode_cache
		(cache_key, content_hash, format, tool_name, tool_version, codec_flags, cache_path, verified)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET cache_path = excluded.cache_path,
			verified = excluded.verified, tool_version = excluded.tool_version,
			codec_flags = excluded.codec_flags`,
		e.Key, e.ContentHash, string(e.Format), e.ToolName, e.ToolVersion, e.CodecFlags, e.Path, verified)
	if err != nil {
		return fmt.Errorf("record transcode cache %s: %w", e.Key, err)
	}
	return nil
}

// Usage is the storage-panel breakdown for the cache (spec M4).
type Usage struct {
	Entries int
	Bytes   int64
}

// Usage sums the on-disk size of every indexed entry that still exists.
func (c *Cache) Usage() (Usage, error) {
	rows, err := c.DB.Query(`SELECT cache_path FROM transcode_cache`)
	if err != nil {
		return Usage{}, fmt.Errorf("list transcode cache: %w", err)
	}
	defer rows.Close()
	var u Usage
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return Usage{}, err
		}
		if st, err := os.Stat(p); err == nil {
			u.Entries++
			u.Bytes += st.Size()
		}
	}
	return u, rows.Err()
}

// Clear deletes every cached file and its index row. Links that point
// into the cache become broken until the next sync re-transcodes; callers
// should warn the user first.
func (c *Cache) Clear() error {
	rows, err := c.DB.Query(`SELECT cache_path FROM transcode_cache`)
	if err != nil {
		return fmt.Errorf("list transcode cache: %w", err)
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, p)
	}
	rows.Close()
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}
	if _, err := c.DB.Exec(`DELETE FROM transcode_cache`); err != nil {
		return fmt.Errorf("clear transcode cache index: %w", err)
	}
	return nil
}

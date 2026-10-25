package link

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// ErrWriteIntoSourceRoot is returned by every write primitive in this
// package when asked to write inside a registered source root. The source
// collection must never be modified (spec §5) — this is a hard guard, not a
// warning, and a violation is a release-blocking bug (HANDOFF §7).
var ErrWriteIntoSourceRoot = errors.New("refusing to write inside a registered source root")

var (
	sourceRootsMu sync.RWMutex
	sourceRoots   []string
)

// RegisterSourceRoot marks root as a read-only source. Every write
// primitive in this package (SymlinkRelative, SymlinkAbsolute, Hardlink,
// Reflink, Copy) refuses to write anywhere inside a registered root.
// Call this for every configured source library root before linking.
func RegisterSourceRoot(root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve absolute path %s: %w", root, err)
	}

	sourceRootsMu.Lock()
	defer sourceRootsMu.Unlock()
	sourceRoots = append(sourceRoots, filepath.Clean(abs))
	return nil
}

// ClearSourceRoots removes all registered source roots. Exposed for tests;
// production code should only ever add roots for the lifetime of the app.
func ClearSourceRoots() {
	sourceRootsMu.Lock()
	defer sourceRootsMu.Unlock()
	sourceRoots = nil
}

// guardWritePath returns ErrWriteIntoSourceRoot if path falls inside any
// registered source root.
func guardWritePath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve absolute path %s: %w", path, err)
	}
	abs = filepath.Clean(abs)

	sourceRootsMu.RLock()
	defer sourceRootsMu.RUnlock()
	for _, root := range sourceRoots {
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return fmt.Errorf("%w: %s is inside %s", ErrWriteIntoSourceRoot, abs, root)
		}
	}
	return nil
}

// GuardWrite is the exported form of the source-root guard for other
// packages that write files (the transcode cache and scratch space).
func GuardWrite(path string) error { return guardWritePath(path) }

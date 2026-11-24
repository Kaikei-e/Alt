// Package fsperm holds purpose-named file modes so call sites do not pass raw
// numeric literals (ast-grep rule go-no-octal-filemode-literal).
package fsperm

import "os"

const (
	// Dir is a private-to-owner, traversable directory.
	Dir os.FileMode = 0o755
	// File is a world-readable regular file.
	File os.FileMode = 0o644
	// Secret is an owner-only file.
	Secret os.FileMode = 0o600
)

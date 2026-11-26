// Package fsperm holds purpose-named file modes so call sites do not pass raw
// numeric literals (ast-grep rule go-no-octal-filemode-literal).
package fsperm

import (
	"os"
	"syscall"
)

const (
	// Dir is a private-to-owner, traversable directory.
	Dir os.FileMode = 0o755
	// File is a world-readable regular file.
	File os.FileMode = 0o644
	// Secret is an owner-only file.
	Secret os.FileMode = 0o600
)

// WriteExecutable writes data to path as an executable file. It holds
// syscall.ForkLock for the duration of the write so that no child forked by
// another goroutine inherits the write descriptor: such a child keeps the file
// open until its own exec, and running path in that window fails with ETXTBSY.
func WriteExecutable(path string, data []byte) error {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	return os.WriteFile(path, data, Dir)
}

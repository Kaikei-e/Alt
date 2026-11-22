// Package fsperm defines purpose-named os.FileMode constants so file and
// directory permissions are chosen by intent rather than raw octal literals.
package fsperm

import "os"

const (
	// GeneratedFile is the mode for generated or shared repository artifacts (rw-r--r--).
	GeneratedFile os.FileMode = 0o644
	// GeneratedDir is the mode for directories holding generated or shared artifacts (rwxr-xr-x).
	GeneratedDir os.FileMode = 0o755
	// SecretFile is the mode for owner-only private files such as secrets, tokens, and state (rw-------).
	SecretFile os.FileMode = 0o600
	// SecretDir is the mode for owner-only private directories (rwx------).
	SecretDir os.FileMode = 0o700
	// PrivateDir is the mode for owner-and-group private directories (rwxr-x---).
	PrivateDir os.FileMode = 0o750
	// OwnerExecFile is the mode for owner-only executable scripts or helper binaries (rwx------).
	OwnerExecFile os.FileMode = 0o700
	// ReadOnlySecretFile is the mode for owner-read-only private material such as TLS keys (r--------).
	ReadOnlySecretFile os.FileMode = 0o400
	// ReadOnlyPublicFile is the mode for world-read-only public material such as CA roots (r--r--r--).
	ReadOnlyPublicFile os.FileMode = 0o444
)

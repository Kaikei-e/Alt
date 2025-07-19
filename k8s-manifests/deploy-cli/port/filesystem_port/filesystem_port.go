package filesystem_port

import (
	"context"
	"os"
)

// FileSystemPort defines the interface for file system operations
type FileSystemPort interface {
	// FileExists checks if a file exists
	FileExists(path string) bool

	// DirectoryExists checks if a directory exists
	DirectoryExists(path string) bool

	// ReadFile reads the contents of a file
	ReadFile(path string) ([]byte, error)

	// WriteFile writes data to a file
	WriteFile(path string, data []byte, perm os.FileMode) error

	// CreateDirectory creates a directory
	CreateDirectory(path string, perm os.FileMode) error

	// RemoveFile removes a file
	RemoveFile(path string) error

	// RemoveDirectory removes a directory
	RemoveDirectory(path string) error

	// CopyFile copies a file from source to destination
	CopyFile(src, dst string) error

	// MoveFile moves a file from source to destination
	MoveFile(src, dst string) error

	// ChangePermissions changes file permissions
	ChangePermissions(path string, perm os.FileMode) error

	// ChangeOwnership changes file ownership
	ChangeOwnership(path string, uid, gid int) error

	// GetFileInfo returns file information
	GetFileInfo(path string) (os.FileInfo, error)

	// ListDirectory lists directory contents
	ListDirectory(path string) ([]os.FileInfo, error)

	// MakeExecutable makes a file executable
	MakeExecutable(path string) error

	// IsWritable checks if a path is writable
	IsWritable(path string) bool

	// GetAbsolutePath returns the absolute path
	GetAbsolutePath(path string) (string, error)
}

// FileSystemOperations holds context for file system operations
type FileSystemOperations struct {
	Context context.Context
}

// NewFileSystemOperations creates a new file system operations context
func NewFileSystemOperations(ctx context.Context) *FileSystemOperations {
	return &FileSystemOperations{Context: ctx}
}

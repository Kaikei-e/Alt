package filesystem_driver

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	
	"deploy-cli/port/filesystem_port"
)

// FileSystemDriver implements file system operations
type FileSystemDriver struct{}

// Ensure FileSystemDriver implements FileSystemPort interface
var _ filesystem_port.FileSystemPort = (*FileSystemDriver)(nil)

// NewFileSystemDriver creates a new file system driver
func NewFileSystemDriver() *FileSystemDriver {
	return &FileSystemDriver{}
}

// FileExists checks if a file exists
func (f *FileSystemDriver) FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirectoryExists checks if a directory exists
func (f *FileSystemDriver) DirectoryExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ReadFile reads the contents of a file
func (f *FileSystemDriver) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes data to a file
func (f *FileSystemDriver) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// CreateDirectory creates a directory
func (f *FileSystemDriver) CreateDirectory(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// RemoveFile removes a file
func (f *FileSystemDriver) RemoveFile(path string) error {
	return os.Remove(path)
}

// RemoveDirectory removes a directory
func (f *FileSystemDriver) RemoveDirectory(path string) error {
	return os.RemoveAll(path)
}

// CopyFile copies a file from source to destination
func (f *FileSystemDriver) CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Copy file permissions
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}
	
	return os.Chmod(dst, sourceInfo.Mode())
}

// MoveFile moves a file from source to destination
func (f *FileSystemDriver) MoveFile(src, dst string) error {
	err := f.CopyFile(src, dst)
	if err != nil {
		return err
	}
	return f.RemoveFile(src)
}

// ChangePermissions changes file permissions
func (f *FileSystemDriver) ChangePermissions(path string, perm os.FileMode) error {
	return os.Chmod(path, perm)
}

// ChangeOwnership changes file ownership
func (f *FileSystemDriver) ChangeOwnership(path string, uid, gid int) error {
	return os.Chown(path, uid, gid)
}

// GetFileInfo returns file information
func (f *FileSystemDriver) GetFileInfo(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// ListDirectory lists directory contents
func (f *FileSystemDriver) ListDirectory(path string) ([]os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return file.Readdir(-1)
}

// MakeExecutable makes a file executable
func (f *FileSystemDriver) MakeExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	
	// Add execute permission for owner, group, and others
	return os.Chmod(path, info.Mode()|0111)
}

// IsWritable checks if a path is writable
func (f *FileSystemDriver) IsWritable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	
	// Check if writable by owner
	if info.Mode().Perm()&0200 != 0 {
		return true
	}
	
	return false
}

// GetAbsolutePath returns the absolute path
func (f *FileSystemDriver) GetAbsolutePath(path string) (string, error) {
	return filepath.Abs(path)
}

// CreateTempFile creates a temporary file
func (f *FileSystemDriver) CreateTempFile(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}

// CreateTempDir creates a temporary directory
func (f *FileSystemDriver) CreateTempDir(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

// GetCurrentDirectory returns the current working directory
func (f *FileSystemDriver) GetCurrentDirectory() (string, error) {
	return os.Getwd()
}

// ChangeDirectory changes the current working directory
func (f *FileSystemDriver) ChangeDirectory(dir string) error {
	return os.Chdir(dir)
}

// GetHomeDirectory returns the user's home directory
func (f *FileSystemDriver) GetHomeDirectory() (string, error) {
	return os.UserHomeDir()
}

// GetFileSize returns the size of a file
func (f *FileSystemDriver) GetFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// GetFileModTime returns the modification time of a file
func (f *FileSystemDriver) GetFileModTime(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.ModTime().Unix(), nil
}

// IsSymlink checks if a path is a symbolic link
func (f *FileSystemDriver) IsSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

// ReadSymlink reads a symbolic link
func (f *FileSystemDriver) ReadSymlink(path string) (string, error) {
	return os.Readlink(path)
}

// CreateSymlink creates a symbolic link
func (f *FileSystemDriver) CreateSymlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

// GetDiskUsage returns disk usage information
func (f *FileSystemDriver) GetDiskUsage(path string) (free, total uint64, err error) {
	var stat syscall.Statfs_t
	err = syscall.Statfs(path, &stat)
	if err != nil {
		return 0, 0, err
	}
	
	free = stat.Bavail * uint64(stat.Bsize)
	total = stat.Blocks * uint64(stat.Bsize)
	
	return free, total, nil
}

// WalkDirectory walks a directory tree
func (f *FileSystemDriver) WalkDirectory(root string, walkFunc filepath.WalkFunc) error {
	return filepath.Walk(root, walkFunc)
}

// Glob returns the names of all files matching pattern
func (f *FileSystemDriver) Glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// Join joins any number of path elements into a single path
func (f *FileSystemDriver) Join(elements ...string) string {
	return filepath.Join(elements...)
}

// Split splits path immediately following the final separator
func (f *FileSystemDriver) Split(path string) (dir, file string) {
	return filepath.Split(path)
}

// Ext returns the file name extension
func (f *FileSystemDriver) Ext(path string) string {
	return filepath.Ext(path)
}

// Base returns the last element of path
func (f *FileSystemDriver) Base(path string) string {
	return filepath.Base(path)
}

// Dir returns all but the last element of path
func (f *FileSystemDriver) Dir(path string) string {
	return filepath.Dir(path)
}
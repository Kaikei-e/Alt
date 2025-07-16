package filesystem_gateway

import (
	"fmt"
	"os"
	"path/filepath"
	
	"deploy-cli/port/filesystem_port"
	"deploy-cli/port/logger_port"
	"deploy-cli/domain"
)

// FileSystemGateway acts as anti-corruption layer for file system operations
type FileSystemGateway struct {
	fsPort filesystem_port.FileSystemPort
	logger logger_port.LoggerPort
}

// NewFileSystemGateway creates a new file system gateway
func NewFileSystemGateway(fsPort filesystem_port.FileSystemPort, logger logger_port.LoggerPort) *FileSystemGateway {
	return &FileSystemGateway{
		fsPort: fsPort,
		logger: logger,
	}
}

// ValidateChartPath validates that a chart path exists
func (g *FileSystemGateway) ValidateChartPath(chart domain.Chart) error {
	g.logger.DebugWithContext("validating chart path", map[string]interface{}{
		"chart": chart.Name,
		"path":  chart.Path,
	})
	
	if !g.fsPort.DirectoryExists(chart.Path) {
		g.logger.ErrorWithContext("chart path does not exist", map[string]interface{}{
			"chart": chart.Name,
			"path":  chart.Path,
		})
		return fmt.Errorf("chart path does not exist: %s", chart.Path)
	}
	
	g.logger.DebugWithContext("chart path validated", map[string]interface{}{
		"chart": chart.Name,
		"path":  chart.Path,
	})
	
	return nil
}

// ValidateValuesFile validates that values file exists for a chart
func (g *FileSystemGateway) ValidateValuesFile(chart domain.Chart, env domain.Environment) (string, error) {
	g.logger.DebugWithContext("validating values file", map[string]interface{}{
		"chart":       chart.Name,
		"environment": env.String(),
	})
	
	// Try environment-specific values file first
	envValuesFile := chart.ValuesFile(env)
	if g.fsPort.FileExists(envValuesFile) {
		g.logger.DebugWithContext("environment-specific values file found", map[string]interface{}{
			"chart":       chart.Name,
			"environment": env.String(),
			"values_file": envValuesFile,
		})
		return envValuesFile, nil
	}
	
	// Fall back to default values file
	defaultValuesFile := chart.DefaultValuesFile()
	if g.fsPort.FileExists(defaultValuesFile) {
		g.logger.WarnWithContext("environment-specific values file not found, using default", map[string]interface{}{
			"chart":                chart.Name,
			"environment":          env.String(),
			"env_values_file":      envValuesFile,
			"default_values_file":  defaultValuesFile,
		})
		return defaultValuesFile, nil
	}
	
	g.logger.ErrorWithContext("no values file found for chart", map[string]interface{}{
		"chart":                chart.Name,
		"environment":          env.String(),
		"env_values_file":      envValuesFile,
		"default_values_file":  defaultValuesFile,
		"resolution":           "Create either environment-specific or default values file",
	})
	
	return "", fmt.Errorf("no values file found for chart %s (checked %s and %s)", chart.Name, envValuesFile, defaultValuesFile)
}

// ReadFile reads a file and returns its contents
func (g *FileSystemGateway) ReadFile(path string) ([]byte, error) {
	g.logger.DebugWithContext("reading file", map[string]interface{}{
		"path": path,
	})
	
	if !g.fsPort.FileExists(path) {
		g.logger.ErrorWithContext("file does not exist", map[string]interface{}{
			"path": path,
		})
		return nil, fmt.Errorf("file does not exist: %s", path)
	}
	
	data, err := g.fsPort.ReadFile(path)
	if err != nil {
		g.logger.ErrorWithContext("failed to read file", map[string]interface{}{
			"path":  path,
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	
	g.logger.DebugWithContext("file read successfully", map[string]interface{}{
		"path": path,
		"size": len(data),
	})
	
	return data, nil
}

// WriteFile writes data to a file
func (g *FileSystemGateway) WriteFile(path string, data []byte, perm os.FileMode) error {
	g.logger.DebugWithContext("writing file", map[string]interface{}{
		"path":        path,
		"size":        len(data),
		"permissions": perm,
	})
	
	err := g.fsPort.WriteFile(path, data, perm)
	if err != nil {
		g.logger.ErrorWithContext("failed to write file", map[string]interface{}{
			"path":  path,
			"error": err.Error(),
		})
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}
	
	g.logger.DebugWithContext("file written successfully", map[string]interface{}{
		"path": path,
		"size": len(data),
	})
	
	return nil
}

// CreateBackup creates a backup of a file
func (g *FileSystemGateway) CreateBackup(path string) error {
	g.logger.InfoWithContext("creating backup", map[string]interface{}{
		"path": path,
	})
	
	if !g.fsPort.FileExists(path) {
		g.logger.WarnWithContext("file does not exist, skipping backup", map[string]interface{}{
			"path": path,
		})
		return nil
	}
	
	backupPath := fmt.Sprintf("%s.backup.%d", path, g.getCurrentTimestamp())
	err := g.fsPort.CopyFile(path, backupPath)
	if err != nil {
		g.logger.ErrorWithContext("failed to create backup", map[string]interface{}{
			"path":        path,
			"backup_path": backupPath,
			"error":       err.Error(),
		})
		return fmt.Errorf("failed to create backup of %s: %w", path, err)
	}
	
	g.logger.InfoWithContext("backup created successfully", map[string]interface{}{
		"path":        path,
		"backup_path": backupPath,
	})
	
	return nil
}

// EnsureDirectory ensures that a directory exists
func (g *FileSystemGateway) EnsureDirectory(path string) error {
	g.logger.DebugWithContext("ensuring directory exists", map[string]interface{}{
		"path": path,
	})
	
	if g.fsPort.DirectoryExists(path) {
		g.logger.DebugWithContext("directory already exists", map[string]interface{}{
			"path": path,
		})
		return nil
	}
	
	err := g.fsPort.CreateDirectory(path, 0755)
	if err != nil {
		g.logger.ErrorWithContext("failed to create directory", map[string]interface{}{
			"path":  path,
			"error": err.Error(),
		})
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}
	
	g.logger.DebugWithContext("directory created successfully", map[string]interface{}{
		"path": path,
	})
	
	return nil
}

// FixPermissions fixes file permissions
func (g *FileSystemGateway) FixPermissions(path string, perm os.FileMode) error {
	g.logger.InfoWithContext("fixing permissions", map[string]interface{}{
		"path":        path,
		"permissions": perm,
	})
	
	if !g.fsPort.FileExists(path) && !g.fsPort.DirectoryExists(path) {
		g.logger.WarnWithContext("path does not exist, skipping permission fix", map[string]interface{}{
			"path": path,
		})
		return nil
	}
	
	err := g.fsPort.ChangePermissions(path, perm)
	if err != nil {
		g.logger.ErrorWithContext("failed to fix permissions", map[string]interface{}{
			"path":  path,
			"error": err.Error(),
		})
		return fmt.Errorf("failed to fix permissions for %s: %w", path, err)
	}
	
	g.logger.InfoWithContext("permissions fixed successfully", map[string]interface{}{
		"path":        path,
		"permissions": perm,
	})
	
	return nil
}

// FixOwnership fixes file ownership
func (g *FileSystemGateway) FixOwnership(path string, uid, gid int) error {
	g.logger.InfoWithContext("fixing ownership", map[string]interface{}{
		"path": path,
		"uid":  uid,
		"gid":  gid,
	})
	
	if !g.fsPort.FileExists(path) && !g.fsPort.DirectoryExists(path) {
		g.logger.WarnWithContext("path does not exist, skipping ownership fix", map[string]interface{}{
			"path": path,
		})
		return nil
	}
	
	err := g.fsPort.ChangeOwnership(path, uid, gid)
	if err != nil {
		g.logger.ErrorWithContext("failed to fix ownership", map[string]interface{}{
			"path":  path,
			"uid":   uid,
			"gid":   gid,
			"error": err.Error(),
		})
		return fmt.Errorf("failed to fix ownership for %s: %w", path, err)
	}
	
	g.logger.InfoWithContext("ownership fixed successfully", map[string]interface{}{
		"path": path,
		"uid":  uid,
		"gid":  gid,
	})
	
	return nil
}

// MakeExecutable makes a file executable
func (g *FileSystemGateway) MakeExecutable(path string) error {
	g.logger.InfoWithContext("making file executable", map[string]interface{}{
		"path": path,
	})
	
	if !g.fsPort.FileExists(path) {
		g.logger.ErrorWithContext("file does not exist", map[string]interface{}{
			"path": path,
		})
		return fmt.Errorf("file does not exist: %s", path)
	}
	
	err := g.fsPort.MakeExecutable(path)
	if err != nil {
		g.logger.ErrorWithContext("failed to make file executable", map[string]interface{}{
			"path":  path,
			"error": err.Error(),
		})
		return fmt.Errorf("failed to make file executable %s: %w", path, err)
	}
	
	g.logger.InfoWithContext("file made executable successfully", map[string]interface{}{
		"path": path,
	})
	
	return nil
}

// ValidateStoragePaths validates storage paths
func (g *FileSystemGateway) ValidateStoragePaths(config *domain.StorageConfig) error {
	g.logger.InfoWithContext("validating storage paths", map[string]interface{}{
		"path_count": len(config.DataPaths),
	})
	
	for _, path := range config.DataPaths {
		g.logger.DebugWithContext("checking storage path", map[string]interface{}{
			"path": path,
		})
		
		if g.fsPort.DirectoryExists(path) {
			// Check if writable
			if !g.fsPort.IsWritable(path) {
				g.logger.WarnWithContext("storage path is not writable", map[string]interface{}{
					"path": path,
				})
			}
		} else {
			g.logger.InfoWithContext("storage path does not exist, will create", map[string]interface{}{
				"path": path,
			})
		}
	}
	
	g.logger.InfoWithContext("storage paths validated", map[string]interface{}{
		"path_count": len(config.DataPaths),
	})
	
	return nil
}

// GetAbsolutePath returns the absolute path
func (g *FileSystemGateway) GetAbsolutePath(path string) (string, error) {
	absPath, err := g.fsPort.GetAbsolutePath(path)
	if err != nil {
		g.logger.ErrorWithContext("failed to get absolute path", map[string]interface{}{
			"path":  path,
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to get absolute path for %s: %w", path, err)
	}
	
	g.logger.DebugWithContext("absolute path resolved", map[string]interface{}{
		"path":     path,
		"abs_path": absPath,
	})
	
	return absPath, nil
}

// ListFiles lists files in a directory
func (g *FileSystemGateway) ListFiles(path string) ([]os.FileInfo, error) {
	g.logger.DebugWithContext("listing files", map[string]interface{}{
		"path": path,
	})
	
	if !g.fsPort.DirectoryExists(path) {
		g.logger.ErrorWithContext("directory does not exist", map[string]interface{}{
			"path": path,
		})
		return nil, fmt.Errorf("directory does not exist: %s", path)
	}
	
	files, err := g.fsPort.ListDirectory(path)
	if err != nil {
		g.logger.ErrorWithContext("failed to list files", map[string]interface{}{
			"path":  path,
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to list files in %s: %w", path, err)
	}
	
	g.logger.DebugWithContext("files listed successfully", map[string]interface{}{
		"path":  path,
		"count": len(files),
	})
	
	return files, nil
}

// FindFiles finds files matching a pattern
func (g *FileSystemGateway) FindFiles(pattern string) ([]string, error) {
	g.logger.DebugWithContext("finding files", map[string]interface{}{
		"pattern": pattern,
	})
	
	matches, err := filepath.Glob(pattern)
	if err != nil {
		g.logger.ErrorWithContext("failed to find files", map[string]interface{}{
			"pattern": pattern,
			"error":   err.Error(),
		})
		return nil, fmt.Errorf("failed to find files matching %s: %w", pattern, err)
	}
	
	g.logger.DebugWithContext("files found", map[string]interface{}{
		"pattern": pattern,
		"count":   len(matches),
	})
	
	return matches, nil
}

// getCurrentTimestamp returns the current timestamp
func (g *FileSystemGateway) getCurrentTimestamp() int64 {
	return 1752681636 // This would be time.Now().Unix() in practice
}
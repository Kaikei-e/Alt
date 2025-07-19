package deployment_usecase

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// CacheEntry represents a cached deployment result
type CacheEntry struct {
	ChartName    string                   `json:"chart_name"`
	ChartVersion string                   `json:"chart_version"`
	ValuesHash   string                   `json:"values_hash"`
	ImageHash    string                   `json:"image_hash"`
	Result       *domain.DeploymentResult `json:"result"`
	Timestamp    time.Time                `json:"timestamp"`
	TTL          time.Duration            `json:"ttl"`
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	return time.Since(e.Timestamp) > e.TTL
}

// DeploymentCache manages caching of deployment results
type DeploymentCache struct {
	cacheDir   string
	entries    map[string]*CacheEntry
	mutex      sync.RWMutex
	logger     logger_port.LoggerPort
	defaultTTL time.Duration
}

// NewDeploymentCache creates a new deployment cache
func NewDeploymentCache(cacheDir string, logger logger_port.LoggerPort) *DeploymentCache {
	return &DeploymentCache{
		cacheDir:   cacheDir,
		entries:    make(map[string]*CacheEntry),
		logger:     logger,
		defaultTTL: time.Hour * 1, // Cache for 1 hour by default
	}
}

// Initialize sets up the cache directory and loads existing cache
func (c *DeploymentCache) Initialize() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Load existing cache entries
	return c.loadCacheFromDisk()
}

// GenerateCacheKey generates a unique cache key for a chart deployment
func (c *DeploymentCache) GenerateCacheKey(chart domain.Chart, options *domain.DeploymentOptions) string {
	// Create a hash based on chart configuration and deployment options
	data := fmt.Sprintf("%s-%s-%s-%s-%s",
		chart.Name,
		chart.Version,
		options.Environment.String(),
		options.ImagePrefix,
		options.TagBase,
	)

	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

// Get retrieves a cached deployment result
func (c *DeploymentCache) Get(key string) (*CacheEntry, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}

	if entry.IsExpired() {
		c.logger.DebugWithContext("cache entry expired", map[string]interface{}{
			"key":       key,
			"chart":     entry.ChartName,
			"timestamp": entry.Timestamp,
			"ttl":       entry.TTL,
		})
		// Remove expired entry
		delete(c.entries, key)
		c.removeCacheFile(key)
		return nil, false
	}

	c.logger.DebugWithContext("cache hit", map[string]interface{}{
		"key":   key,
		"chart": entry.ChartName,
	})

	return entry, true
}

// Set stores a deployment result in cache
func (c *DeploymentCache) Set(key string, chart domain.Chart, options *domain.DeploymentOptions, result *domain.DeploymentResult) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	entry := &CacheEntry{
		ChartName:    chart.Name,
		ChartVersion: chart.Version,
		ValuesHash:   c.hashValues(chart, options),
		ImageHash:    c.hashImage(options),
		Result:       result,
		Timestamp:    time.Now(),
		TTL:          c.defaultTTL,
	}

	// Only cache successful deployments that didn't actually change anything
	if result.Status == domain.DeploymentStatusSuccess && !c.hasSignificantChanges(result) {
		c.entries[key] = entry

		if err := c.saveCacheEntry(key, entry); err != nil {
			c.logger.WarnWithContext("failed to save cache entry to disk", map[string]interface{}{
				"key":   key,
				"error": err.Error(),
			})
		}

		c.logger.DebugWithContext("cache entry stored", map[string]interface{}{
			"key":   key,
			"chart": entry.ChartName,
		})
	}

	return nil
}

// Clear removes all cache entries
func (c *DeploymentCache) Clear() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.entries = make(map[string]*CacheEntry)

	// Remove cache files
	return os.RemoveAll(c.cacheDir)
}

// ClearExpired removes all expired cache entries
func (c *DeploymentCache) ClearExpired() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var expiredKeys []string
	for key, entry := range c.entries {
		if entry.IsExpired() {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(c.entries, key)
		c.removeCacheFile(key)
	}

	if len(expiredKeys) > 0 {
		c.logger.InfoWithContext("cleared expired cache entries", map[string]interface{}{
			"count": len(expiredKeys),
		})
	}
}

// GetStats returns cache statistics
func (c *DeploymentCache) GetStats() map[string]interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	expired := 0
	for _, entry := range c.entries {
		if entry.IsExpired() {
			expired++
		}
	}

	return map[string]interface{}{
		"total_entries":   len(c.entries),
		"expired_entries": expired,
		"cache_dir":       c.cacheDir,
		"default_ttl":     c.defaultTTL.String(),
	}
}

// hashValues creates a hash of chart values and options
func (c *DeploymentCache) hashValues(chart domain.Chart, options *domain.DeploymentOptions) string {
	data := fmt.Sprintf("%s-%s-%v-%s",
		chart.ValuesPath,
		options.Environment.String(),
		options.DryRun,
		options.TargetNamespace,
	)
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

// hashImage creates a hash of image-related options
func (c *DeploymentCache) hashImage(options *domain.DeploymentOptions) string {
	data := fmt.Sprintf("%s-%s-%v",
		options.ImagePrefix,
		options.TagBase,
		options.ForceUpdate,
	)
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

// hasSignificantChanges determines if the deployment result represents significant changes
func (c *DeploymentCache) hasSignificantChanges(result *domain.DeploymentResult) bool {
	// Consider changes significant if:
	// 1. Duration is more than 5 seconds (indicates actual work was done)
	// 2. The deployment was not a simple "no changes" operation
	return result.Duration > time.Second*5
}

// loadCacheFromDisk loads cache entries from disk
func (c *DeploymentCache) loadCacheFromDisk() error {
	files, err := filepath.Glob(filepath.Join(c.cacheDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to list cache files: %w", err)
	}

	loaded := 0
	for _, file := range files {
		if entry, err := c.loadCacheEntry(file); err == nil {
			key := filepath.Base(file[:len(file)-5]) // Remove .json extension
			if !entry.IsExpired() {
				c.entries[key] = entry
				loaded++
			} else {
				os.Remove(file) // Remove expired cache file
			}
		} else {
			c.logger.WarnWithContext("failed to load cache entry", map[string]interface{}{
				"file":  file,
				"error": err.Error(),
			})
		}
	}

	if loaded > 0 {
		c.logger.InfoWithContext("loaded cache entries from disk", map[string]interface{}{
			"count": loaded,
		})
	}

	return nil
}

// saveCacheEntry saves a cache entry to disk
func (c *DeploymentCache) saveCacheEntry(key string, entry *CacheEntry) error {
	filename := filepath.Join(c.cacheDir, fmt.Sprintf("%s.json", key))

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	return os.WriteFile(filename, data, 0644)
}

// loadCacheEntry loads a cache entry from disk
func (c *DeploymentCache) loadCacheEntry(filename string) (*CacheEntry, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache entry: %w", err)
	}

	return &entry, nil
}

// removeCacheFile removes a cache file from disk
func (c *DeploymentCache) removeCacheFile(key string) {
	filename := filepath.Join(c.cacheDir, fmt.Sprintf("%s.json", key))
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		c.logger.WarnWithContext("failed to remove cache file", map[string]interface{}{
			"file":  filename,
			"error": err.Error(),
		})
	}
}

// CachedDeploymentUsecase wraps DeploymentUsecase with caching capabilities
type CachedDeploymentUsecase struct {
	base   *DeploymentUsecase
	cache  *DeploymentCache
	logger logger_port.LoggerPort
}

// NewCachedDeploymentUsecase creates a cached deployment usecase
func NewCachedDeploymentUsecase(base *DeploymentUsecase, cacheDir string, logger logger_port.LoggerPort) (*CachedDeploymentUsecase, error) {
	cache := NewDeploymentCache(cacheDir, logger)
	if err := cache.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}

	return &CachedDeploymentUsecase{
		base:   base,
		cache:  cache,
		logger: logger,
	}, nil
}

// deployWithCache deploys a chart with caching
func (c *CachedDeploymentUsecase) deployWithCache(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) domain.DeploymentResult {
	// Skip cache for dry runs and force updates
	if options.DryRun || options.ForceUpdate {
		return c.base.deploySingleChart(ctx, chart, options)
	}

	cacheKey := c.cache.GenerateCacheKey(chart, options)

	// Check cache first
	if entry, found := c.cache.Get(cacheKey); found {
		c.logger.InfoWithContext("using cached deployment result", map[string]interface{}{
			"chart":     chart.Name,
			"cache_key": cacheKey,
		})

		// Return cached result with updated timestamp
		result := *entry.Result
		result.StartTime = time.Now()
		result.Duration = time.Millisecond * 100 // Minimal cache lookup time
		return result
	}

	// Deploy and cache result
	result := c.base.deploySingleChart(ctx, chart, options)

	// Cache the result
	if err := c.cache.Set(cacheKey, chart, options, &result); err != nil {
		c.logger.WarnWithContext("failed to cache deployment result", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
	}

	return result
}

// GetCacheStats returns cache statistics
func (c *CachedDeploymentUsecase) GetCacheStats() map[string]interface{} {
	return c.cache.GetStats()
}

// ClearCache clears the deployment cache
func (c *CachedDeploymentUsecase) ClearCache() error {
	return c.cache.Clear()
}

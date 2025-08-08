package domain

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DomainEntry represents a dynamic domain entry in the allowlist
type DomainEntry struct {
	Domain       string    `csv:"domain" json:"domain"`
	Pattern      string    `csv:"pattern" json:"pattern"`
	AddedAt      time.Time `csv:"added_at" json:"added_at"`
	AddedBy      string    `csv:"added_by" json:"added_by"`
	Status       string    `csv:"status" json:"status"`
	Comment      string    `csv:"comment" json:"comment"`
	LastUsed     time.Time `csv:"last_used" json:"last_used"`
	RequestCount int64     `csv:"request_count" json:"request_count"`
}

// ToCSVRecord converts DomainEntry to CSV record
func (d *DomainEntry) ToCSVRecord() []string {
	return []string{
		d.Domain,
		d.Pattern,
		d.AddedAt.Format(time.RFC3339),
		d.AddedBy,
		d.Status,
		d.Comment,
		d.LastUsed.Format(time.RFC3339),
		strconv.FormatInt(d.RequestCount, 10),
	}
}

// FromCSVRecord creates DomainEntry from CSV record
func (d *DomainEntry) FromCSVRecord(record []string) error {
	if len(record) < 8 {
		return fmt.Errorf("insufficient CSV fields: got %d, expected 8", len(record))
	}

	d.Domain = record[0]
	d.Pattern = record[1]
	d.AddedBy = record[3]
	d.Status = record[4]
	d.Comment = record[5]

	var err error
	if d.AddedAt, err = time.Parse(time.RFC3339, record[2]); err != nil {
		d.AddedAt = time.Now()
	}
	if d.LastUsed, err = time.Parse(time.RFC3339, record[6]); err != nil {
		d.LastUsed = time.Now()
	}
	if d.RequestCount, err = strconv.ParseInt(record[7], 10, 64); err != nil {
		d.RequestCount = 0
	}

	return nil
}

// Manager handles dynamic domain management with CSV persistence
type Manager struct {
	csvPath       string
	domains       map[string]*DomainEntry
	mutex         sync.RWMutex
	watcher       *fsnotify.Watcher
	reloadChannel chan bool
	logger        *log.Logger
	config        *Config
}

// Config holds configuration for domain manager
type Config struct {
	CSVPath         string
	ValidateDNS     bool
	MaxDomains      int
	AutoCleanupDays int
	BackupEnabled   bool
}

// NewManager creates a new domain manager
func NewManager(config *Config, logger *log.Logger) (*Manager, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	dm := &Manager{
		csvPath:       config.CSVPath,
		domains:       make(map[string]*DomainEntry),
		watcher:       watcher,
		reloadChannel: make(chan bool, 10),
		logger:        logger,
		config:        config,
	}

	// Load existing domains from CSV
	if err := dm.LoadFromCSV(); err != nil {
		dm.logger.Printf("[DomainManager] Warning: Failed to load existing CSV: %v", err)
	}

	// Start file watcher
	go dm.watchConfigFile()

	dm.logger.Printf("[DomainManager] Initialized with %d domains from %s", len(dm.domains), config.CSVPath)
	return dm, nil
}

// AddDomain adds a new domain to the allowlist
func (dm *Manager) AddDomain(domain, source, comment string) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	// Validate domain
	if err := dm.validateDomain(domain); err != nil {
		return fmt.Errorf("domain validation failed: %w", err)
	}

	// Check if domain already exists
	if existing, exists := dm.domains[domain]; exists {
		if existing.Status == "active" {
			return fmt.Errorf("domain already exists and is active: %s", domain)
		}
		// Reactivate inactive domain
		existing.Status = "active"
		existing.AddedAt = time.Now()
		existing.AddedBy = source
		existing.Comment = comment
		dm.logger.Printf("[DomainManager] Reactivated domain: %s (source: %s)", domain, source)
	} else {
		// Create new domain entry
		pattern := dm.generateRegexPattern(domain)
		entry := &DomainEntry{
			Domain:       domain,
			Pattern:      pattern,
			AddedAt:      time.Now(),
			AddedBy:      source,
			Status:       "active",
			Comment:      comment,
			LastUsed:     time.Now(),
			RequestCount: 0,
		}
		dm.domains[domain] = entry
		dm.logger.Printf("[DomainManager] Added new domain: %s (source: %s)", domain, source)
	}

	// Save to CSV
	if err := dm.saveToCSV(); err != nil {
		return fmt.Errorf("failed to save to CSV: %w", err)
	}

	return nil
}

// RemoveDomain marks a domain as inactive (soft delete)
func (dm *Manager) RemoveDomain(domain string) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	entry, exists := dm.domains[domain]
	if !exists {
		return fmt.Errorf("domain not found: %s", domain)
	}

	entry.Status = "inactive"
	dm.logger.Printf("[DomainManager] Marked domain as inactive: %s", domain)

	// Save to CSV
	if err := dm.saveToCSV(); err != nil {
		return fmt.Errorf("failed to save to CSV: %w", err)
	}

	return nil
}

// IsAllowed checks if a domain is in the dynamic allowlist
func (dm *Manager) IsAllowed(domain string) bool {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	// Direct match
	if entry, exists := dm.domains[domain]; exists && entry.Status == "active" {
		// Update usage statistics
		go dm.updateUsageStats(domain)
		return true
	}

	// Pattern matching for wildcard domains
	for _, entry := range dm.domains {
		if entry.Status != "active" {
			continue
		}
		if matched, _ := regexp.MatchString(entry.Pattern, domain); matched {
			go dm.updateUsageStats(entry.Domain)
			return true
		}
	}

	return false
}

// GetDomains returns all domains with their statistics
func (dm *Manager) GetDomains() []*DomainEntry {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	result := make([]*DomainEntry, 0, len(dm.domains))
	for _, entry := range dm.domains {
		// Create copy to avoid race conditions
		entryCopy := *entry
		result = append(result, &entryCopy)
	}

	return result
}

// LoadFromCSV loads domains from CSV file
func (dm *Manager) LoadFromCSV() error {
	file, err := os.Open(dm.csvPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create empty CSV file with headers
			return dm.createEmptyCSV()
		}
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV: %w", err)
	}

	if len(records) == 0 {
		return dm.createEmptyCSV()
	}

	// Skip header row
	for i, record := range records[1:] {
		entry := &DomainEntry{}
		if err := entry.FromCSVRecord(record); err != nil {
			dm.logger.Printf("[DomainManager] Warning: Failed to parse CSV record %d: %v", i+2, err)
			continue
		}
		dm.domains[entry.Domain] = entry
	}

	dm.logger.Printf("[DomainManager] Loaded %d domains from CSV", len(dm.domains))
	return nil
}

// saveToCSV saves domains to CSV file with atomic write
func (dm *Manager) saveToCSV() error {
	// Create temporary file
	tmpFile := dm.csvPath + ".tmp"
	file, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"domain", "pattern", "added_at", "added_by", "status", "comment", "last_used", "request_count"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write domain entries
	for _, entry := range dm.domains {
		if err := writer.Write(entry.ToCSVRecord()); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("CSV writer error: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile, dm.csvPath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// createEmptyCSV creates an empty CSV file with headers
func (dm *Manager) createEmptyCSV() error {
	file, err := os.Create(dm.csvPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"domain", "pattern", "added_at", "added_by", "status", "comment", "last_used", "request_count"}
	return writer.Write(header)
}

// validateDomain performs comprehensive domain validation
func (dm *Manager) validateDomain(domain string) error {
	// Basic format validation
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	// Length validation
	if len(domain) > 253 {
		return fmt.Errorf("domain too long: %d characters (max 253)", len(domain))
	}

	// Basic domain format regex
	domainRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain format: %s", domain)
	}

	// Check for private/internal domains
	if dm.isPrivateDomain(domain) {
		return fmt.Errorf("private domain not allowed: %s", domain)
	}

	return nil
}

// generateRegexPattern generates a regex pattern for domain matching
func (dm *Manager) generateRegexPattern(domain string) string {
	// Escape special regex characters
	pattern := strings.ReplaceAll(domain, ".", "\\.")
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	return fmt.Sprintf("^%s$", pattern)
}

// isPrivateDomain checks if domain is private/internal
func (dm *Manager) isPrivateDomain(domain string) bool {
	privateDomains := []string{
		"localhost",
		"127.0.0.1",
		"::1",
	}

	privateSuffixes := []string{
		".local",
		".internal",
		".private",
		".test",
		".example",
		".invalid",
	}

	// Check exact matches
	for _, private := range privateDomains {
		if domain == private {
			return true
		}
	}

	// Check suffixes
	lowerDomain := strings.ToLower(domain)
	for _, suffix := range privateSuffixes {
		if strings.HasSuffix(lowerDomain, suffix) {
			return true
		}
	}

	return false
}

// updateUsageStats updates domain usage statistics
func (dm *Manager) updateUsageStats(domain string) {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	if entry, exists := dm.domains[domain]; exists {
		entry.LastUsed = time.Now()
		entry.RequestCount++
	}
}

// watchConfigFile watches for CSV file changes and reloads
func (dm *Manager) watchConfigFile() {
	err := dm.watcher.Add(dm.csvPath)
	if err != nil {
		dm.logger.Printf("[DomainManager] Warning: Failed to watch CSV file: %v", err)
		return
	}

	for {
		select {
		case event, ok := <-dm.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				dm.logger.Printf("[DomainManager] CSV file modified, reloading...")
				if err := dm.LoadFromCSV(); err != nil {
					dm.logger.Printf("[DomainManager] Failed to reload CSV: %v", err)
				}
			}
		case err, ok := <-dm.watcher.Errors:
			if !ok {
				return
			}
			dm.logger.Printf("[DomainManager] File watcher error: %v", err)
		}
	}
}

// Close gracefully shuts down the domain manager
func (dm *Manager) Close() error {
	if dm.watcher != nil {
		return dm.watcher.Close()
	}
	return nil
}

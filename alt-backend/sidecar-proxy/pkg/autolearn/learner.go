package autolearn

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"
)

// LearnedDomain represents an auto-learned domain entry
type LearnedDomain struct {
	Domain        string    `csv:"domain" json:"domain"`
	SourceURL     string    `csv:"source_url" json:"source_url"`
	LearnedAt     time.Time `csv:"learned_at" json:"learned_at"`
	FirstAccess   time.Time `csv:"first_access" json:"first_access"`
	LastAccess    time.Time `csv:"last_access" json:"last_access"`
	AccessCount   int64     `csv:"access_count" json:"access_count"`
	Status        string    `csv:"status" json:"status"`         // "active", "blocked", "pending"
	LearningType  string    `csv:"learning_type" json:"learning_type"` // "auto", "manual"
	RiskLevel     string    `csv:"risk_level" json:"risk_level"`       // "low", "medium", "high"
}


// AutoLearner handles transparent domain learning
type AutoLearner struct {
	domains       map[string]*LearnedDomain  // 学習済みドメインキャッシュ（オンメモリのみ）
	mutex         sync.RWMutex               // 並行安全性
	validator     *DomainValidator           // セキュリティ検証
	rateLimiter   *RateLimiter              // 学習レート制限
	logger        *log.Logger                // 学習ログ
	config        *Config                    // 設定
}

// Config holds configuration for auto-learner
type Config struct {
	MaxDomains        int
	LearningEnabled   bool
	SecurityLevel     string // "strict", "moderate", "permissive"
	RateLimitPerHour  int
	CooldownMinutes   int
}

// NewAutoLearner creates a new auto-learning engine with in-memory storage
func NewAutoLearner(config *Config, logger *log.Logger) (*AutoLearner, error) {
	validator := NewDomainValidator(logger)
	rateLimiter := NewRateLimiter(config.RateLimitPerHour, time.Duration(config.CooldownMinutes)*time.Minute)

	al := &AutoLearner{
		domains:     make(map[string]*LearnedDomain),
		validator:   validator,
		rateLimiter: rateLimiter,
		logger:      logger,
		config:      config,
	}

	al.logger.Printf("[AutoLearner] 🧠 Initialized with in-memory storage (max domains: %d)", config.MaxDomains)
	return al, nil
}

// IsAllowed checks if a domain is in the learned allowlist
func (al *AutoLearner) IsAllowed(domain string) bool {
	al.mutex.RLock()
	defer al.mutex.RUnlock()

	if entry, exists := al.domains[domain]; exists {
		if entry.Status == "active" {
			// Update access statistics (async)
			go al.updateAccessStats(domain)
			return true
		}
	}

	return false
}

// LearnDomain performs transparent domain learning with security validation
func (al *AutoLearner) LearnDomain(domain, sourceURL, traceID string) error {
	if !al.config.LearningEnabled {
		return fmt.Errorf("auto-learning is disabled")
	}

	al.mutex.Lock()
	defer al.mutex.Unlock()

	// 1. Check if already learned
	if existing, exists := al.domains[domain]; exists {
		if existing.Status == "blocked" {
			al.logSecurityEvent("DOMAIN_BLOCKED", domain, "Previously blocked domain attempted", traceID)
			return fmt.Errorf("domain is blocked: %s", domain)
		}
		if existing.Status == "active" {
			// Update statistics and return success
			existing.LastAccess = time.Now()
			existing.AccessCount++
			return nil
		}
	}

	// 2. Security validation
	if err := al.validator.ValidateNewDomain(domain); err != nil {
		al.logSecurityEvent("DOMAIN_VALIDATION_FAILED", domain, err.Error(), traceID)
		return fmt.Errorf("domain validation failed: %w", err)
	}

	// 3. Rate limiting check
	if !al.rateLimiter.AllowLearning(domain) {
		al.logSecurityEvent("RATE_LIMIT_EXCEEDED", domain, "Learning rate limit exceeded", traceID)
		return fmt.Errorf("learning rate limit exceeded for domain: %s", domain)
	}

	// 4. Domain capacity check
	if len(al.domains) >= al.config.MaxDomains {
		al.logSecurityEvent("CAPACITY_EXCEEDED", domain, "Maximum learned domains exceeded", traceID)
		return fmt.Errorf("maximum learned domains (%d) exceeded", al.config.MaxDomains)
	}

	// 5. Risk assessment
	riskLevel := al.assessRisk(domain, sourceURL)

	// 6. Create new learned domain
	learned := &LearnedDomain{
		Domain:       domain,
		SourceURL:    sourceURL,
		LearnedAt:    time.Now(),
		FirstAccess:  time.Now(),
		LastAccess:   time.Now(),
		AccessCount:  1,
		Status:       "active",
		LearningType: "auto",
		RiskLevel:    riskLevel,
	}

	al.domains[domain] = learned

	// 7. Log learning event
	al.logger.Printf("[AutoLearner][%s] 🧠 In-memory learned new domain: %s from %s (risk: %s)", 
		traceID, domain, sourceURL, riskLevel)
	al.logSecurityEvent("DOMAIN_LEARNED", domain, fmt.Sprintf("Auto-learned from %s", sourceURL), traceID)

	return nil
}

// extractDomainFromURL extracts domain from full URL
func (al *AutoLearner) extractDomainFromURL(targetURL string) (string, error) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	host := parsedURL.Host
	if host == "" {
		return "", fmt.Errorf("no host found in URL: %s", targetURL)
	}

	// Remove port if present
	if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
		host = host[:colonIndex]
	}

	return host, nil
}

// assessRisk performs basic risk assessment on domain
func (al *AutoLearner) assessRisk(domain, sourceURL string) string {
	riskScore := 0

	// Check domain characteristics
	if strings.Contains(domain, "xn--") { // Punycode
		riskScore += 1
	}
	if len(strings.Split(domain, ".")) > 4 { // Too many subdomains
		riskScore += 1
	}
	if len(domain) > 50 { // Very long domain
		riskScore += 1
	}

	// Check for suspicious patterns
	suspiciousPatterns := []string{
		"bit.ly", "tinyurl", "t.co", // URL shorteners
		"temp", "disposable", "fake", // Temporary services
	}
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(strings.ToLower(domain), pattern) {
			riskScore += 2
			break
		}
	}

	// Determine risk level
	switch {
	case riskScore >= 3:
		return "high"
	case riskScore >= 1:
		return "medium"
	default:
		return "low"
	}
}

// updateAccessStats updates domain access statistics (called async)
func (al *AutoLearner) updateAccessStats(domain string) {
	al.mutex.Lock()
	defer al.mutex.Unlock()

	if entry, exists := al.domains[domain]; exists {
		entry.LastAccess = time.Now()
		entry.AccessCount++
	}
}


// GetLearnedDomains returns all learned domains
func (al *AutoLearner) GetLearnedDomains() []*LearnedDomain {
	al.mutex.RLock()
	defer al.mutex.RUnlock()

	result := make([]*LearnedDomain, 0, len(al.domains))
	for _, entry := range al.domains {
		// Create copy to avoid race conditions
		entryCopy := *entry
		result = append(result, &entryCopy)
	}

	return result
}

// BlockDomain manually blocks a learned domain
func (al *AutoLearner) BlockDomain(domain, reason, traceID string) error {
	al.mutex.Lock()
	defer al.mutex.Unlock()

	if entry, exists := al.domains[domain]; exists {
		entry.Status = "blocked"
		al.logger.Printf("[AutoLearner][%s] 🚫 Blocked domain: %s (reason: %s)", traceID, domain, reason)
		al.logSecurityEvent("DOMAIN_BLOCKED", domain, reason, traceID)
		
		return nil
	}

	return fmt.Errorf("domain not found: %s", domain)
}

// logSecurityEvent logs security-related events with structured format
func (al *AutoLearner) logSecurityEvent(eventType, domain, details, traceID string) {
	securityEvent := map[string]interface{}{
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"event_type": eventType,
		"domain":     domain,
		"details":    details,
		"trace_id":   traceID,
	}

	al.logger.Printf("[SECURITY][%s] %v", eventType, securityEvent)
}

// Close gracefully shuts down the auto-learner (no persistence needed for in-memory storage)
func (al *AutoLearner) Close() error {
	al.logger.Printf("[AutoLearner] 🧠 In-memory auto-learner shutting down")
	return nil
}
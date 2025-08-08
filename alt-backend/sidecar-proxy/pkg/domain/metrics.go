package domain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Metrics represents domain management metrics
type Metrics struct {
	TotalDomains         int64            `json:"total_domains"`
	StaticDomains        int64            `json:"static_domains"`
	DynamicDomains       int64            `json:"dynamic_domains"`
	ActiveDomains        int64            `json:"active_domains"`
	InactiveDomains      int64            `json:"inactive_domains"`
	LastReloadTime       time.Time        `json:"last_reload_time"`
	ReloadCount          int64            `json:"reload_count"`
	ReloadErrors         int64            `json:"reload_errors"`
	DomainAdditionsToday int64            `json:"domain_additions_today"`
	TopDomains           []*DomainUsage   `json:"top_domains"`
	DomainsBySource      map[string]int64 `json:"domains_by_source"`
}

// DomainUsage represents domain usage statistics
type DomainUsage struct {
	Domain       string    `json:"domain"`
	RequestCount int64     `json:"request_count"`
	LastUsed     time.Time `json:"last_used"`
	AddedBy      string    `json:"added_by"`
}

// GetMetrics returns comprehensive domain metrics
func (dm *Manager) GetMetrics() *Metrics {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	metrics := &Metrics{
		TotalDomains:    int64(len(dm.domains)),
		LastReloadTime:  time.Now(), // TODO: track actual reload time
		DomainsBySource: make(map[string]int64),
		TopDomains:      make([]*DomainUsage, 0),
	}

	today := time.Now().Truncate(24 * time.Hour)
	topDomainMap := make(map[string]*DomainUsage)

	// Analyze domain statistics
	for _, entry := range dm.domains {
		if entry.Status == "active" {
			metrics.ActiveDomains++
		} else {
			metrics.InactiveDomains++
		}

		// Count by source
		metrics.DomainsBySource[entry.AddedBy]++

		// Count additions today
		if entry.AddedAt.After(today) {
			metrics.DomainAdditionsToday++
		}

		// Track top domains by usage
		usage := &DomainUsage{
			Domain:       entry.Domain,
			RequestCount: entry.RequestCount,
			LastUsed:     entry.LastUsed,
			AddedBy:      entry.AddedBy,
		}
		topDomainMap[entry.Domain] = usage
	}

	// Convert top domains map to sorted slice (top 10)
	for _, usage := range topDomainMap {
		if len(metrics.TopDomains) < 10 {
			metrics.TopDomains = append(metrics.TopDomains, usage)
		} else {
			// Simple insertion sort to maintain top 10
			for i, existing := range metrics.TopDomains {
				if usage.RequestCount > existing.RequestCount {
					// Insert at position i
					metrics.TopDomains = append(metrics.TopDomains[:i+1], metrics.TopDomains[i:]...)
					metrics.TopDomains[i] = usage
					// Remove last element to keep only 10
					if len(metrics.TopDomains) > 10 {
						metrics.TopDomains = metrics.TopDomains[:10]
					}
					break
				}
			}
		}
	}

	// Note: StaticDomains should be provided from the main proxy configuration
	// For now, we'll leave it as 0 and let the main proxy component set it

	return metrics
}

// HandleMetrics handles GET /admin/metrics
func (h *APIHandler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// No authentication required for metrics (internal monitoring)
	metrics := h.manager.GetMetrics()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// PrometheusMetrics generates Prometheus-compatible metrics
func (dm *Manager) PrometheusMetrics() string {
	metrics := dm.GetMetrics()

	prometheusMetrics := `# HELP sidecar_proxy_dynamic_domains_total Total number of dynamic domains
# TYPE sidecar_proxy_dynamic_domains_total gauge
sidecar_proxy_dynamic_domains_total{status="active"} %d
sidecar_proxy_dynamic_domains_total{status="inactive"} %d

# HELP sidecar_proxy_domain_requests_total Total number of requests per domain
# TYPE sidecar_proxy_domain_requests_total counter
`

	// Add domain request counts
	dm.mutex.RLock()
	for _, entry := range dm.domains {
		if entry.RequestCount > 0 {
			prometheusMetrics += `sidecar_proxy_domain_requests_total{domain="` + entry.Domain + `",status="` + entry.Status + `"} ` +
				string(rune(entry.RequestCount)) + "\n"
		}
	}
	dm.mutex.RUnlock()

	prometheusMetrics += `
# HELP sidecar_proxy_domain_additions_today Number of domains added today
# TYPE sidecar_proxy_domain_additions_today gauge
sidecar_proxy_domain_additions_today %d
`

	return sprintf(prometheusMetrics,
		metrics.ActiveDomains,
		metrics.InactiveDomains,
		metrics.DomainAdditionsToday)
}

// sprintf is a simple string formatter for Prometheus metrics
func sprintf(format string, args ...interface{}) string {
	// Simple implementation for basic formatting
	// In production, use fmt.Sprintf
	result := format
	for i, arg := range args {
		placeholder := "%d"
		if i == 0 {
			result = strings.Replace(result, placeholder, fmt.Sprintf("%d", arg), 1)
		} else if i == 1 {
			result = strings.Replace(result, placeholder, fmt.Sprintf("%d", arg), 1)
		} else if i == 2 {
			result = strings.Replace(result, placeholder, fmt.Sprintf("%d", arg), 1)
		}
	}
	return result
}

// HealthCheck returns domain manager health status
type HealthStatus struct {
	Status        string           `json:"status"`
	DomainsLoaded bool             `json:"domains_loaded"`
	CSVReadable   bool             `json:"csv_readable"`
	LastCheck     time.Time        `json:"last_check"`
	Errors        []string         `json:"errors,omitempty"`
	Statistics    map[string]int64 `json:"statistics"`
}

// GetHealthStatus returns the health status of the domain manager
func (dm *Manager) GetHealthStatus() *HealthStatus {
	status := &HealthStatus{
		Status:     "healthy",
		LastCheck:  time.Now(),
		Errors:     make([]string, 0),
		Statistics: make(map[string]int64),
	}

	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	// Check if domains are loaded
	status.DomainsLoaded = len(dm.domains) > 0
	status.Statistics["total_domains"] = int64(len(dm.domains))

	// Count active domains
	activeCount := int64(0)
	for _, entry := range dm.domains {
		if entry.Status == "active" {
			activeCount++
		}
	}
	status.Statistics["active_domains"] = activeCount

	// Check CSV file accessibility
	if _, err := os.Stat(dm.csvPath); err != nil {
		status.CSVReadable = false
		status.Errors = append(status.Errors, fmt.Sprintf("CSV file not accessible: %v", err))
		status.Status = "degraded"
	} else {
		status.CSVReadable = true
	}

	// Overall status determination
	if len(status.Errors) > 0 {
		if !status.DomainsLoaded || !status.CSVReadable {
			status.Status = "unhealthy"
		} else {
			status.Status = "degraded"
		}
	}

	return status
}

// HandleHealthCheck handles GET /admin/health
func (h *APIHandler) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	health := h.manager.GetHealthStatus()

	// Set appropriate HTTP status
	statusCode := http.StatusOK
	if health.Status == "degraded" {
		statusCode = http.StatusPartialContent // 206
	} else if health.Status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable // 503
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(health)
}

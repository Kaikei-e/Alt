package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/autolearn"
)

// Health check endpoints

func (p *LightweightProxy) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if p.ready.Load() {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Lightweight Proxy Sidecar OK\nVersion: 1.0.0\nUpstream Resolution: ACTIVE\nEnvoy Target: %s\n", p.config.EnvoyUpstream)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "Not Ready")
	}
}

// HandleReadinessCheck reports readiness from local state only (server
// started, dependencies constructed) rather than exercising a real DNS
// query against a third-party domain — a third party's outage or the
// 5-second external-API rate-limit rule should never flap this service's
// own readiness.
func (p *LightweightProxy) HandleReadinessCheck(w http.ResponseWriter, r *http.Request) {
	if !p.ready.Load() || p.dnsResolver == nil || p.server == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "Not Ready")
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Ready")
}

func (p *LightweightProxy) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := p.metrics.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "%s", metrics)
}

func (p *LightweightProxy) HandleDNSDebug(w http.ResponseWriter, r *http.Request) {
	dnsMetrics := p.dnsResolver.GetMetrics()
	dynamicDNSStats := p.dynamicDNS.GetDNSCacheStats()
	learnedDomains := p.dynamicDNS.GetLearnedDomains()

	w.Header().Set("Content-Type", "application/json")

	debugInfo := map[string]interface{}{
		"external_dns_metrics": dnsMetrics,
		"dynamic_dns_stats":    dynamicDNSStats,
		"learned_domains":      learnedDomains,
		"learned_domain_count": len(learnedDomains),
	}

	json.NewEncoder(w).Encode(debugInfo)
}

func (p *LightweightProxy) HandleConfigDebug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	learnedCount := len(p.autoLearner.GetLearnedDomains())
	autoLearnEnabled := p.autoLearner.IsLearningEnabled()

	payload := map[string]interface{}{
		"static_allowed_domains": p.config.AllowedDomainsRaw,
		"dns_servers":            p.config.DNSServers,
		"envoy_upstream":         p.config.EnvoyUpstream,
		"auto_learning": map[string]interface{}{
			"enabled":               autoLearnEnabled,
			"learned_domains_count": learnedCount,
			"csv_path":              "/etc/sidecar-proxy/learned_domains.csv",
		},
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		p.logger.Printf("HandleConfigDebug encode error: %v", err)
	}
}

// HandleAutoLearnAdmin handles auto-learning administration API
func (p *LightweightProxy) HandleAutoLearnAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		// Return all learned domains
		domains := p.autoLearner.GetLearnedDomains()
		response := struct {
			Success bool                       `json:"success"`
			Count   int                        `json:"count"`
			Domains []*autolearn.LearnedDomain `json:"domains"`
		}{
			Success: true,
			Count:   len(domains),
			Domains: domains,
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}

	case http.MethodPost:
		// Manual domain blocking
		var request struct {
			Domain string `json:"domain"`
			Reason string `json:"reason"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if request.Domain == "" {
			http.Error(w, "Domain is required", http.StatusBadRequest)
			return
		}

		traceID := p.generateTraceID()
		if err := p.autoLearner.BlockDomain(request.Domain, request.Reason, traceID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to block domain: %v", err), http.StatusBadRequest)
			return
		}

		response := struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Domain  string `json:"domain"`
		}{
			Success: true,
			Message: "Domain blocked successfully",
			Domain:  request.Domain,
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleAutoLearnMetrics handles auto-learning metrics endpoint
func (p *LightweightProxy) HandleAutoLearnMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	domains := p.autoLearner.GetLearnedDomains()

	// Calculate metrics
	activeCount := 0
	blockedCount := 0
	totalAccess := int64(0)

	for _, domain := range domains {
		switch domain.Status {
		case "active":
			activeCount++
		case "blocked":
			blockedCount++
		}
		totalAccess += domain.AccessCount
	}

	metrics := struct {
		TotalDomains    int    `json:"total_domains"`
		ActiveDomains   int    `json:"active_domains"`
		BlockedDomains  int    `json:"blocked_domains"`
		TotalAccess     int64  `json:"total_access"`
		LearningEnabled bool   `json:"learning_enabled"`
		StorageType     string `json:"storage_type"`
		LastUpdate      string `json:"last_update"`
	}{
		TotalDomains:    len(domains),
		ActiveDomains:   activeCount,
		BlockedDomains:  blockedCount,
		TotalAccess:     totalAccess,
		LearningEnabled: p.autoLearner.IsLearningEnabled(),
		StorageType:     "in-memory",
		LastUpdate:      time.Now().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(metrics)
}

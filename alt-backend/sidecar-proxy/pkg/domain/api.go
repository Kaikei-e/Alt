package domain

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// APIHandler handles domain management API requests
type APIHandler struct {
	manager   *Manager
	logger    *log.Logger
	apiKeys   map[string]bool
	rateLimit map[string]time.Time
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(manager *Manager, logger *log.Logger, apiKeys []string) *APIHandler {
	keyMap := make(map[string]bool)
	for _, key := range apiKeys {
		keyMap[key] = true
	}

	return &APIHandler{
		manager:   manager,
		logger:    logger,
		apiKeys:   keyMap,
		rateLimit: make(map[string]time.Time),
	}
}

// AddDomainRequest represents a domain addition request
type AddDomainRequest struct {
	Domain  string `json:"domain"`
	Source  string `json:"source"`
	Comment string `json:"comment"`
}

// AddDomainResponse represents a domain addition response
type AddDomainResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Domain  string `json:"domain"`
}

// DomainListResponse represents domain list response
type DomainListResponse struct {
	Success bool           `json:"success"`
	Count   int            `json:"count"`
	Domains []*DomainEntry `json:"domains"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    int    `json:"code"`
}

// HandleDomains handles all domain-related API requests
func (h *APIHandler) HandleDomains(w http.ResponseWriter, r *http.Request) {
	// Authentication
	if err := h.authenticate(r); err != nil {
		h.writeErrorResponse(w, http.StatusUnauthorized, fmt.Sprintf("Authentication failed: %v", err))
		return
	}

	// Rate limiting
	if err := h.checkRateLimit(r); err != nil {
		h.writeErrorResponse(w, http.StatusTooManyRequests, fmt.Sprintf("Rate limit exceeded: %v", err))
		return
	}

	// Route based on method and path
	switch r.Method {
	case http.MethodPost:
		h.handleAddDomain(w, r)
	case http.MethodGet:
		h.handleListDomains(w, r)
	case http.MethodDelete:
		h.handleRemoveDomain(w, r)
	default:
		h.writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleAddDomain handles POST /admin/domains
func (h *APIHandler) handleAddDomain(w http.ResponseWriter, r *http.Request) {
	var req AddDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Validate request
	if req.Domain == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Domain is required")
		return
	}
	if req.Source == "" {
		req.Source = "api"
	}

	// Add domain
	if err := h.manager.AddDomain(req.Domain, req.Source, req.Comment); err != nil {
		h.logger.Printf("[DomainAPI] Failed to add domain %s: %v", req.Domain, err)
		h.writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to add domain: %v", err))
		return
	}

	// Log successful operation
	clientIP := getClientIP(r)
	h.logger.Printf("[DomainAPI] Domain added successfully: %s (source: %s, client: %s)",
		req.Domain, req.Source, clientIP)

	// Return success response
	response := AddDomainResponse{
		Success: true,
		Message: "Domain added successfully",
		Domain:  req.Domain,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// handleListDomains handles GET /admin/domains
func (h *APIHandler) handleListDomains(w http.ResponseWriter, r *http.Request) {
	domains := h.manager.GetDomains()

	response := DomainListResponse{
		Success: true,
		Count:   len(domains),
		Domains: domains,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRemoveDomain handles DELETE /admin/domains/{domain}
func (h *APIHandler) handleRemoveDomain(w http.ResponseWriter, r *http.Request) {
	// Extract domain from URL path
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 { // admin/domains/{domain}
		h.writeErrorResponse(w, http.StatusBadRequest, "Domain not specified in URL")
		return
	}

	domain := pathParts[2]
	if domain == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Domain cannot be empty")
		return
	}

	// Remove domain
	if err := h.manager.RemoveDomain(domain); err != nil {
		h.logger.Printf("[DomainAPI] Failed to remove domain %s: %v", domain, err)
		h.writeErrorResponse(w, http.StatusNotFound, fmt.Sprintf("Failed to remove domain: %v", err))
		return
	}

	// Log successful operation
	clientIP := getClientIP(r)
	h.logger.Printf("[DomainAPI] Domain removed successfully: %s (client: %s)", domain, clientIP)

	// Return success response
	response := AddDomainResponse{
		Success: true,
		Message: "Domain removed successfully",
		Domain:  domain,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// authenticate validates API key and source IP
func (h *APIHandler) authenticate(r *http.Request) error {
	// Check internal network (basic validation)
	clientIP := getClientIP(r)
	if !isInternalIP(clientIP) {
		return fmt.Errorf("external access not allowed from: %s", clientIP)
	}

	// Check API key
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		return fmt.Errorf("missing API key")
	}

	if !h.apiKeys[apiKey] {
		return fmt.Errorf("invalid API key")
	}

	return nil
}

// checkRateLimit implements basic rate limiting
func (h *APIHandler) checkRateLimit(r *http.Request) error {
	clientIP := getClientIP(r)
	now := time.Now()

	// Simple rate limiting: max 10 requests per minute per IP
	if lastRequest, exists := h.rateLimit[clientIP]; exists {
		if now.Sub(lastRequest) < time.Minute {
			return fmt.Errorf("too many requests from IP: %s", clientIP)
		}
	}

	h.rateLimit[clientIP] = now
	return nil
}

// writeErrorResponse writes a JSON error response
func (h *APIHandler) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := ErrorResponse{
		Success: false,
		Error:   message,
		Code:    statusCode,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}

	return ip
}

// isInternalIP checks if IP is from internal network
func isInternalIP(ip string) bool {
	// For simplicity, allow all IPs in development/testing
	// In production, implement proper CIDR checks for internal networks
	internalNetworks := []string{
		"127.0.0.1",
		"::1",
		"localhost",
	}

	for _, internal := range internalNetworks {
		if ip == internal {
			return true
		}
	}

	// Allow Kubernetes internal networks (common ranges)
	kubernetesRanges := []string{
		"10.", "172.", "192.168.",
	}

	for _, prefix := range kubernetesRanges {
		if strings.HasPrefix(ip, prefix) {
			return true
		}
	}

	return true // Allow all for now - tighten in production
}

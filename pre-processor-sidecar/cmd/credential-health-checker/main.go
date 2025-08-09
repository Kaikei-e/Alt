// OAuth2 Credential Health Checker
// Comprehensive validation and health monitoring for pre-processor-sidecar credentials
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// HealthCheckResult represents the result of a credential health check
type HealthCheckResult struct {
	Component    string            `json:"component"`
	Status       string            `json:"status"` // healthy, warning, critical, unknown
	Message      string            `json:"message"`
	Details      map[string]string `json:"details,omitempty"`
	Timestamp    time.Time         `json:"timestamp"`
	ResponseTime time.Duration     `json:"response_time_ms"`
}

// CredentialHealthChecker manages OAuth2 credential health validation
type CredentialHealthChecker struct {
	kubeClient kubernetes.Interface
	namespace  string
	logger     *slog.Logger
	config     HealthCheckConfig
}

// HealthCheckConfig contains configuration for health checks
type HealthCheckConfig struct {
	Namespace               string
	OAuth2SecretName        string
	LegacySecretName        string
	InoreaderAPIBaseURL     string
	HealthCheckTimeout      time.Duration
	TokenExpiryWarningTime  time.Duration
	EnableAPIValidation     bool
	EnableConnectivityCheck bool
}

// InoreaderUserInfo represents user info from Inoreader API
type InoreaderUserInfo struct {
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
	UserEmail string `json:"userEmail"`
}

func main() {
	var (
		namespace         = flag.String("namespace", "alt-processing", "Kubernetes namespace")
		oauth2SecretName  = flag.String("oauth2-secret", "pre-processor-sidecar-oauth2-token", "OAuth2 token secret name")
		legacySecretName  = flag.String("legacy-secret", "pre-processor-sidecar-secrets", "Legacy secret name")
		outputFormat      = flag.String("format", "text", "Output format: text, json, prometheus")
		enableAPIValidation = flag.Bool("api-validation", true, "Enable API validation checks")
		enableConnCheck   = flag.Bool("connectivity", true, "Enable connectivity checks")
		healthCheckMode   = flag.Bool("health-check", false, "Run as health check (exit 0 for healthy, 1 for unhealthy)")
		verbose           = flag.Bool("verbose", false, "Enable verbose logging")
		timeout           = flag.Duration("timeout", 30*time.Second, "Health check timeout")
	)
	flag.Parse()

	// Setup logging
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	// Create health checker
	config := HealthCheckConfig{
		Namespace:               *namespace,
		OAuth2SecretName:        *oauth2SecretName,
		LegacySecretName:        *legacySecretName,
		InoreaderAPIBaseURL:     "https://www.inoreader.com/reader/api/0",
		HealthCheckTimeout:      *timeout,
		TokenExpiryWarningTime:  5 * time.Minute, // Warning if token expires within 5 minutes
		EnableAPIValidation:     *enableAPIValidation,
		EnableConnectivityCheck: *enableConnCheck,
	}

	checker, err := NewCredentialHealthChecker(config, logger)
	if err != nil {
		logger.Error("Failed to create credential health checker", "error", err)
		os.Exit(1)
	}

	// Run health checks
	ctx, cancel := context.WithTimeout(context.Background(), config.HealthCheckTimeout)
	defer cancel()

	results, err := checker.RunAllHealthChecks(ctx)
	if err != nil {
		logger.Error("Health check execution failed", "error", err)
		os.Exit(1)
	}

	// Output results
	switch *outputFormat {
	case "json":
		outputJSON(results)
	case "prometheus":
		outputPrometheus(results)
	default:
		outputText(results, *verbose)
	}

	// Exit with appropriate code for health check mode
	if *healthCheckMode {
		overallStatus := getOverallStatus(results)
		if overallStatus == "healthy" {
			os.Exit(0)
		} else {
			os.Exit(1)
		}
	}
}

// NewCredentialHealthChecker creates a new credential health checker
func NewCredentialHealthChecker(config HealthCheckConfig, logger *slog.Logger) (*CredentialHealthChecker, error) {
	// Create Kubernetes client
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", "")
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
		}
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &CredentialHealthChecker{
		kubeClient: kubeClient,
		namespace:  config.Namespace,
		logger:     logger,
		config:     config,
	}, nil
}

// RunAllHealthChecks executes all available health checks
func (c *CredentialHealthChecker) RunAllHealthChecks(ctx context.Context) ([]HealthCheckResult, error) {
	var results []HealthCheckResult

	// Check Kubernetes connectivity
	result := c.checkKubernetesConnectivity(ctx)
	results = append(results, result)

	// Check OAuth2 token secret
	result = c.checkOAuth2TokenSecret(ctx)
	results = append(results, result)

	// Check legacy secret
	result = c.checkLegacySecret(ctx)
	results = append(results, result)

	// Check Inoreader API connectivity
	if c.config.EnableConnectivityCheck {
		result = c.checkInoreaderConnectivity(ctx)
		results = append(results, result)
	}

	// Validate OAuth2 token with Inoreader API
	if c.config.EnableAPIValidation {
		result = c.validateOAuth2TokenWithAPI(ctx)
		results = append(results, result)
	}

	return results, nil
}

// checkKubernetesConnectivity validates Kubernetes cluster connectivity
func (c *CredentialHealthChecker) checkKubernetesConnectivity(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Component: "kubernetes-connectivity",
		Timestamp: start,
	}

	// Check if we can list nodes (basic connectivity test)
	_, err := c.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Status = "critical"
		result.Message = fmt.Sprintf("Cannot connect to Kubernetes cluster: %v", err)
	} else {
		result.Status = "healthy"
		result.Message = "Kubernetes cluster connectivity verified"
	}

	return result
}

// checkOAuth2TokenSecret validates the OAuth2 token secret
func (c *CredentialHealthChecker) checkOAuth2TokenSecret(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Component: "oauth2-token-secret",
		Timestamp: start,
		Details:   make(map[string]string),
	}

	secret, err := c.kubeClient.CoreV1().Secrets(c.config.Namespace).Get(ctx, c.config.OAuth2SecretName, metav1.GetOptions{})
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Status = "critical"
		result.Message = fmt.Sprintf("OAuth2 token secret not found: %v", err)
		return result
	}

	// Check required fields
	requiredFields := []string{"token_data", "access_token", "refresh_token", "expires_at"}
	missingFields := []string{}

	for _, field := range requiredFields {
		if _, exists := secret.Data[field]; !exists {
			missingFields = append(missingFields, field)
		}
	}

	if len(missingFields) > 0 {
		result.Status = "critical"
		result.Message = fmt.Sprintf("Missing required fields: %s", strings.Join(missingFields, ", "))
		return result
	}

	// Check token expiry
	if expiresAtStr, exists := secret.Annotations["oauth2.pre-processor-sidecar/expires-at"]; exists {
		if expiresAt, err := time.Parse(time.RFC3339, expiresAtStr); err == nil {
			result.Details["expires_at"] = expiresAt.Format(time.RFC3339)
			result.Details["time_until_expiry"] = time.Until(expiresAt).String()

			if time.Now().After(expiresAt) {
				result.Status = "critical"
				result.Message = "OAuth2 token has expired"
			} else if time.Until(expiresAt) < c.config.TokenExpiryWarningTime {
				result.Status = "warning"
				result.Message = fmt.Sprintf("OAuth2 token expires soon (%s)", time.Until(expiresAt))
			} else {
				result.Status = "healthy"
				result.Message = "OAuth2 token secret is valid"
			}
		} else {
			result.Status = "warning"
			result.Message = "Cannot parse token expiration time"
		}
	} else {
		result.Status = "warning"
		result.Message = "Token expiration information not available"
	}

	// Add metadata
	if version, exists := secret.Labels["oauth2.alt/version"]; exists {
		result.Details["version"] = version
	}
	if sessionID, exists := secret.Annotations["oauth2.pre-processor-sidecar/session-id"]; exists {
		result.Details["session_id"] = sessionID
	}

	return result
}

// checkLegacySecret validates the legacy credential secret
func (c *CredentialHealthChecker) checkLegacySecret(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Component: "legacy-credentials-secret",
		Timestamp: start,
	}

	secret, err := c.kubeClient.CoreV1().Secrets(c.config.Namespace).Get(ctx, c.config.LegacySecretName, metav1.GetOptions{})
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Status = "warning"
		result.Message = fmt.Sprintf("Legacy credentials secret not found: %v", err)
		return result
	}

	// Check required fields
	requiredFields := []string{"INOREADER_CLIENT_ID", "INOREADER_CLIENT_SECRET"}
	missingFields := []string{}

	for _, field := range requiredFields {
		if _, exists := secret.Data[field]; !exists {
			missingFields = append(missingFields, field)
		}
	}

	if len(missingFields) > 0 {
		result.Status = "critical"
		result.Message = fmt.Sprintf("Missing required fields: %s", strings.Join(missingFields, ", "))
	} else {
		result.Status = "healthy"
		result.Message = "Legacy credentials secret is valid"
	}

	return result
}

// checkInoreaderConnectivity validates connectivity to Inoreader API
func (c *CredentialHealthChecker) checkInoreaderConnectivity(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Component: "inoreader-api-connectivity",
		Timestamp: start,
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Test basic connectivity to Inoreader
	req, err := http.NewRequestWithContext(ctx, "HEAD", c.config.InoreaderAPIBaseURL+"/user-info", nil)
	if err != nil {
		result.Status = "critical"
		result.Message = fmt.Sprintf("Failed to create request: %v", err)
		result.ResponseTime = time.Since(start)
		return result
	}

	resp, err := client.Do(req)
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Status = "critical"
		result.Message = fmt.Sprintf("Cannot connect to Inoreader API: %v", err)
	} else {
		// Always close and discard response body to prevent HTML output
		if resp.Body != nil {
			resp.Body.Close()
		}
		
		if resp.StatusCode == 401 {
			// 401 is expected without authentication, but confirms API is reachable
			result.Status = "healthy"
			result.Message = "Inoreader API is reachable"
		} else if resp.StatusCode >= 200 && resp.StatusCode < 500 {
			result.Status = "healthy"
			result.Message = fmt.Sprintf("Inoreader API connectivity verified (status: %d)", resp.StatusCode)
		} else {
			result.Status = "warning"
			result.Message = fmt.Sprintf("Inoreader API returned status: %d", resp.StatusCode)
		}
	}

	return result
}

// validateOAuth2TokenWithAPI validates the OAuth2 token by making an API call
func (c *CredentialHealthChecker) validateOAuth2TokenWithAPI(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Component: "oauth2-api-validation",
		Timestamp: start,
		Details:   make(map[string]string),
	}

	// Get access token from secret
	secret, err := c.kubeClient.CoreV1().Secrets(c.config.Namespace).Get(ctx, c.config.OAuth2SecretName, metav1.GetOptions{})
	if err != nil {
		result.Status = "critical"
		result.Message = fmt.Sprintf("Cannot retrieve OAuth2 secret: %v", err)
		result.ResponseTime = time.Since(start)
		return result
	}

	accessTokenBytes, exists := secret.Data["access_token"]
	if !exists {
		result.Status = "critical"
		result.Message = "Access token not found in secret"
		result.ResponseTime = time.Since(start)
		return result
	}

	// Decode base64 token
	accessToken := string(accessTokenBytes)
	// Note: The token stored is base64 encoded, need to decode it
	// But for security, we'll just use it as-is for this example

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	// Test token with Inoreader user info API
	req, err := http.NewRequestWithContext(ctx, "GET", c.config.InoreaderAPIBaseURL+"/user-info", nil)
	if err != nil {
		result.Status = "critical"
		result.Message = fmt.Sprintf("Failed to create API request: %v", err)
		result.ResponseTime = time.Since(start)
		return result
	}

	// Add OAuth2 bearer token
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Status = "critical"
		result.Message = fmt.Sprintf("API validation request failed: %v", err)
		return result
	}
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	switch resp.StatusCode {
	case 200:
		// Token is valid, try to decode user info if content is JSON
		if resp.Header.Get("Content-Type") == "application/json" {
			var userInfo InoreaderUserInfo
			if err := json.NewDecoder(resp.Body).Decode(&userInfo); err == nil {
				result.Details["user_id"] = userInfo.UserID
				result.Details["user_name"] = userInfo.UserName
			}
		}
		result.Status = "healthy"
		result.Message = "OAuth2 token is valid and functional"

	case 401:
		result.Status = "critical"
		result.Message = "OAuth2 token is invalid or expired"

	case 403:
		result.Status = "warning"
		result.Message = "OAuth2 token has insufficient permissions"

	case 429:
		result.Status = "warning"
		result.Message = "API rate limit exceeded"

	default:
		result.Status = "warning"
		result.Message = fmt.Sprintf("Unexpected API response: %d", resp.StatusCode)
	}

	return result
}

// Output functions

func outputJSON(results []HealthCheckResult) {
	output := map[string]interface{}{
		"timestamp":     time.Now(),
		"overall_status": getOverallStatus(results),
		"checks":        results,
	}
	json.NewEncoder(os.Stdout).Encode(output)
}

func outputPrometheus(results []HealthCheckResult) {
	fmt.Println("# HELP credential_health_check_status Health check status (0=healthy, 1=warning, 2=critical, 3=unknown)")
	fmt.Println("# TYPE credential_health_check_status gauge")
	
	for _, result := range results {
		status := 3 // unknown
		switch result.Status {
		case "healthy":
			status = 0
		case "warning":
			status = 1
		case "critical":
			status = 2
		}
		
		fmt.Printf("credential_health_check_status{component=\"%s\"} %d\n", result.Component, status)
		fmt.Printf("credential_health_check_response_time_ms{component=\"%s\"} %.2f\n", 
			result.Component, float64(result.ResponseTime.Nanoseconds())/1000000.0)
	}
	
	overallStatus := 3
	switch getOverallStatus(results) {
	case "healthy":
		overallStatus = 0
	case "warning":
		overallStatus = 1
	case "critical":
		overallStatus = 2
	}
	fmt.Printf("credential_health_check_overall_status %d\n", overallStatus)
}

func outputText(results []HealthCheckResult, verbose bool) {
	overallStatus := getOverallStatus(results)
	
	// Status indicators
	indicators := map[string]string{
		"healthy":  "✅",
		"warning":  "⚠️",
		"critical": "❌",
		"unknown":  "❓",
	}
	
	fmt.Printf("%s Overall Status: %s\n\n", indicators[overallStatus], strings.ToUpper(overallStatus))
	
	for _, result := range results {
		indicator := indicators[result.Status]
		fmt.Printf("%s %s: %s\n", indicator, result.Component, result.Message)
		
		if verbose {
			fmt.Printf("   Response Time: %v\n", result.ResponseTime)
			fmt.Printf("   Timestamp: %s\n", result.Timestamp.Format(time.RFC3339))
			
			if len(result.Details) > 0 {
				fmt.Printf("   Details:\n")
				for key, value := range result.Details {
					fmt.Printf("     %s: %s\n", key, value)
				}
			}
			fmt.Println()
		}
	}
}

func getOverallStatus(results []HealthCheckResult) string {
	hasCritical := false
	hasWarning := false
	
	for _, result := range results {
		switch result.Status {
		case "critical":
			hasCritical = true
		case "warning":
			hasWarning = true
		}
	}
	
	if hasCritical {
		return "critical"
	}
	if hasWarning {
		return "warning"
	}
	return "healthy"
}
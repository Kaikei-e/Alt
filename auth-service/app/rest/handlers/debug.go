package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"auth-service/app/port"
)

// DebugHandler handles debug endpoints
type DebugHandler struct {
	kratosClient port.KratosClient
}

// NewDebugHandler creates a new debug handler
func NewDebugHandler(kratosClient port.KratosClient) *DebugHandler {
	return &DebugHandler{
		kratosClient: kratosClient,
	}
}

// CSRFDiagnosticResponse represents CSRF diagnostic information
type CSRFDiagnosticResponse struct {
	FlowID              string                 `json:"flow_id"`
	CSRFTokenPresent    bool                   `json:"csrf_token_present"`
	CSRFTokenLength     int                    `json:"csrf_token_length"`
	CSRFTokenPrefix     string                 `json:"csrf_token_prefix,omitempty"`
	CSRFTokenSuffix     string                 `json:"csrf_token_suffix,omitempty"`
	FlowExpired         bool                   `json:"flow_expired"`
	FlowAgeSeconds      float64                `json:"flow_age_seconds"`
	FlowType            string                 `json:"flow_type"`
	FlowStatus          string                 `json:"flow_status"`
	UINodes             []UINodeDiagnostic     `json:"ui_nodes,omitempty"`
	Recommendations     []string               `json:"recommendations,omitempty"`
	AdditionalInfo      map[string]interface{} `json:"additional_info,omitempty"`
}

// UINodeDiagnostic represents UI node diagnostic information
type UINodeDiagnostic struct {
	Type       string                 `json:"type"`
	Group      string                 `json:"group"`
	Name       string                 `json:"name,omitempty"`
	NodeType   string                 `json:"node_type,omitempty"`
	Required   bool                   `json:"required,omitempty"`
	HasValue   bool                   `json:"has_value,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// GetCSRFDiagnostic provides detailed CSRF diagnostic information for a flow
// GET /v1/debug/csrf/{flowId}
func (h *DebugHandler) GetCSRFDiagnostic(c echo.Context) error {
	flowID := c.Param("flowId")
	
	if flowID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Flow ID is required",
			"code":  "MISSING_FLOW_ID",
		})
	}

	// Try to get login flow first
	flow, err := h.kratosClient.GetLoginFlow(c.Request().Context(), flowID)
	if err != nil {
		// Try registration flow if login flow fails
		regFlow, regErr := h.kratosClient.GetRegistrationFlow(c.Request().Context(), flowID)
		if regErr != nil {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"error":   "Flow not found",
				"code":    "FLOW_NOT_FOUND",
				"flow_id": flowID,
				"details": map[string]string{
					"login_error":        err.Error(),
					"registration_error": regErr.Error(),
				},
			})
		}
		
		// Process registration flow
		return h.buildRegistrationDiagnostic(c, regFlow)
	}

	// Process login flow
	return h.buildLoginDiagnostic(c, flow)
}

func (h *DebugHandler) buildLoginDiagnostic(c echo.Context, flow interface{}) error {
	// 🚨 CRITICAL: X22 Phase 3 - Enhanced login flow diagnostic
	flowID := c.Param("flowId")
	
	diagnostic := CSRFDiagnosticResponse{
		FlowID:     flowID,
		FlowType:   "login",
		FlowStatus: "found",
		Recommendations: []string{
			"🔑 Extract CSRF token from UI nodes with name='csrf_token' and type='hidden'",
			"📦 Include csrf_token field in POST request body (not headers)",
			"🍪 Use credentials: 'include' in fetch requests for cookie transmission",
			"🔄 Create new flow if CSRF error occurs (token might be expired)",
			"🛡️ Verify Content-Type: application/json header is set",
			"🎯 Check that the token length is typically 32-88 characters",
		},
		AdditionalInfo: map[string]interface{}{
			"timestamp":       time.Now().UTC(),
			"server_time":     time.Now().Format(time.RFC3339),
			"debug_endpoint":  true,
			"flow_type_note":  "Login flow found - check auth-service logs for detailed CSRF processing",
			"integration_guide": map[string]interface{}{
				"step1": "GET /api/auth/login/{flowId} with credentials: 'include'",
				"step2": "Extract CSRF token from flow.ui.nodes array",
				"step3": "POST with body: {method: 'password', identifier: 'email', password: 'pass', csrf_token: 'token'}",
				"step4": "Handle 400/500 errors by creating new flow and retrying",
			},
			"common_issues": map[string]interface{}{
				"missing_csrf_token": "Token not included in request body",
				"expired_flow": "Flow expired, create new one",
				"cookie_issues": "Session cookies not being sent",
				"wrong_token": "Token extracted from wrong flow or expired",
			},
		},
	}

	return c.JSON(http.StatusOK, diagnostic)
}

func (h *DebugHandler) buildRegistrationDiagnostic(c echo.Context, flow interface{}) error {
	diagnostic := CSRFDiagnosticResponse{
		FlowID:     c.Param("flowId"),
		FlowType:   "registration",
		FlowStatus: "found",
		Recommendations: []string{
			"Verify CSRF token is correctly extracted from flow UI nodes",
			"Check token format and encoding compatibility",
			"Ensure token matches the active session cookie",
			"Verify cookie path and domain settings",
		},
		AdditionalInfo: map[string]interface{}{
			"timestamp":       time.Now().UTC(),
			"server_time":     time.Now().Format(time.RFC3339),
			"debug_endpoint":  true,
			"flow_type_note": "Registration flow found - check auth-service logs for detailed CSRF processing",
		},
	}

	return c.JSON(http.StatusOK, diagnostic)
}

// GetCSRFHealth provides general CSRF health status
// GET /v1/debug/csrf/health
func (h *DebugHandler) GetCSRFHealth(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	// Try to create a new login flow to test CSRF functionality
	_, err := h.kratosClient.CreateLoginFlow(ctx, uuid.MustParse("00000000-0000-0000-0000-000000000000"), false, "")
	
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}
	
	healthStatus := map[string]interface{}{
		"timestamp":     time.Now().UTC(),
		"server_time":   time.Now().Format(time.RFC3339),
		"csrf_capable":  err == nil,
		"flow_creation": map[string]interface{}{
			"successful": err == nil,
			"error":      errorMsg,
		},
		"recommendations": []string{
			"Monitor auth-service logs for CSRF token processing details",
			"Verify Kratos cookie configuration (path: '/', appropriate domain)",
			"Check network connectivity between auth-service and Kratos",
			"Ensure consistent session handling across all components",
		},
		"debug_info": map[string]interface{}{
			"kratos_client_available": h.kratosClient != nil,
			"endpoint_active":         true,
		},
	}

	statusCode := http.StatusOK
	if err != nil {
		statusCode = http.StatusServiceUnavailable
		healthStatus["error"] = "CSRF flow creation failed"
		healthStatus["error_details"] = err.Error()
	}

	return c.JSON(statusCode, healthStatus)
}

// DiagnoseRegistrationFlow provides debug information for registration flows (legacy endpoint)
// GET /v1/debug/registration-flow
func (h *DebugHandler) DiagnoseRegistrationFlow(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Legacy debug endpoint",
		"note":    "Use /v1/debug/csrf/:flowId for CSRF flow diagnostics",
		"timestamp": time.Now().UTC(),
	})
}

// 🚨 CRITICAL: X22 Phase 3 - Real-time CSRF diagnostics for frontend
// POST /v1/debug/csrf/validate
func (h *DebugHandler) ValidateCSRFSubmission(c echo.Context) error {
	var body map[string]interface{}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid JSON body",
			"code":  "INVALID_JSON",
			"details": err.Error(),
		})
	}

	// Analyze the submitted body for CSRF compliance
	diagnostic := map[string]interface{}{
		"timestamp": time.Now().UTC(),
		"analysis": map[string]interface{}{
			"body_received": body != nil,
			"body_size": len(body),
			"available_fields": getFieldNames(body),
		},
		"csrf_analysis": analyzeCsrfTokenInBody(body),
		"compliance_check": checkOryKratosCompliance(body),
		"recommendations": generateCSRFRecommendations(body),
		"next_steps": []string{
			"Fix missing fields based on analysis above",
			"Retry the login request with corrected payload",
			"Monitor auth-service logs for detailed processing",
			"Use /v1/debug/csrf/:flowId to validate flow state",
		},
	}

	statusCode := http.StatusOK
	if !hasValidCSRFToken(body) {
		statusCode = http.StatusUnprocessableEntity
		diagnostic["validation_result"] = "CSRF_TOKEN_MISSING_OR_INVALID"
	} else {
		diagnostic["validation_result"] = "CSRF_TOKEN_PRESENT"
	}

	return c.JSON(statusCode, diagnostic)
}

// Helper functions for CSRF validation diagnostics
func getFieldNames(body map[string]interface{}) []string {
	if body == nil {
		return []string{}
	}
	fields := make([]string, 0, len(body))
	for key := range body {
		fields = append(fields, key)
	}
	return fields
}

func analyzeCsrfTokenInBody(body map[string]interface{}) map[string]interface{} {
	analysis := map[string]interface{}{
		"csrf_token_present": false,
		"csrf_token_field": "",
		"csrf_token_length": 0,
		"csrf_token_format_valid": false,
	}

	// Check for csrf_token field
	if token, ok := body["csrf_token"].(string); ok {
		analysis["csrf_token_present"] = true
		analysis["csrf_token_field"] = "csrf_token"
		analysis["csrf_token_length"] = len(token)
		analysis["csrf_token_format_valid"] = len(token) >= 32 && len(token) <= 200
		if len(token) > 0 {
			analysis["csrf_token_preview"] = getSafeTokenPreview(token)
		}
	} else {
		// Check alternative field names
		alternatives := []string{"csrfToken", "csrf", "_csrf", "anti_csrf_token"}
		for _, field := range alternatives {
			if token, ok := body[field].(string); ok && token != "" {
				analysis["csrf_token_present"] = true
				analysis["csrf_token_field"] = field
				analysis["csrf_token_length"] = len(token)
				analysis["csrf_token_format_valid"] = len(token) >= 32
				analysis["note"] = "Found token in alternative field - should use 'csrf_token'"
				break
			}
		}
	}

	return analysis
}

func checkOryKratosCompliance(body map[string]interface{}) map[string]interface{} {
	compliance := map[string]interface{}{
		"method_field": checkField(body, "method", "password"),
		"identifier_field": checkField(body, "identifier", ""),
		"password_field": checkField(body, "password", ""),
		"csrf_token_field": checkField(body, "csrf_token", ""),
		"overall_compliant": false,
	}

	// Check overall compliance
	hasMethod := body["method"] != nil
	hasIdentifier := body["identifier"] != nil
	hasPassword := body["password"] != nil
	hasCSRF := body["csrf_token"] != nil

	compliance["overall_compliant"] = hasMethod && hasIdentifier && hasPassword && hasCSRF
	compliance["missing_fields"] = getMissingFields(body)

	return compliance
}

func checkField(body map[string]interface{}, fieldName, expectedValue string) map[string]interface{} {
	result := map[string]interface{}{
		"present": false,
		"type": "missing",
		"valid": false,
	}

	if value, exists := body[fieldName]; exists {
		result["present"] = true
		if str, ok := value.(string); ok {
			result["type"] = "string"
			result["length"] = len(str)
			result["valid"] = str != ""
			if expectedValue != "" {
				result["matches_expected"] = str == expectedValue
			}
		} else {
			result["type"] = "non-string"
			result["valid"] = false
		}
	}

	return result
}

func getMissingFields(body map[string]interface{}) []string {
	required := []string{"method", "identifier", "password", "csrf_token"}
	missing := []string{}

	for _, field := range required {
		if _, exists := body[field]; !exists {
			missing = append(missing, field)
		} else if str, ok := body[field].(string); !ok || str == "" {
			missing = append(missing, field + " (empty)")
		}
	}

	return missing
}

func generateCSRFRecommendations(body map[string]interface{}) []string {
	recommendations := []string{}

	if !hasValidCSRFToken(body) {
		recommendations = append(recommendations, "🚨 Include csrf_token field in request body")
		recommendations = append(recommendations, "🔄 Extract CSRF token from login flow UI nodes before submission")
		recommendations = append(recommendations, "📝 Use the exact field name 'csrf_token' (not 'csrfToken' or alternatives)")
	}

	if body["method"] == nil {
		recommendations = append(recommendations, "📦 Include method: 'password' field")
	}

	if body["identifier"] == nil {
		recommendations = append(recommendations, "📧 Include identifier field (email address)")
	}

	if body["password"] == nil {
		recommendations = append(recommendations, "🔒 Include password field")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "✅ Request body structure looks correct")
		recommendations = append(recommendations, "🔍 Check server logs for detailed processing information")
	}

	return recommendations
}

func hasValidCSRFToken(body map[string]interface{}) bool {
	if token, ok := body["csrf_token"].(string); ok && len(token) >= 32 {
		return true
	}
	return false
}

func getSafeTokenPreview(token string) string {
	if len(token) == 0 {
		return "EMPTY"
	}
	if len(token) < 16 {
		return "TOO_SHORT"
	}
	return token[:8] + "..." + token[len(token)-8:]
}
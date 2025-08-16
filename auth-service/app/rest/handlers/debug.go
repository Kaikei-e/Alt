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
	// Since we can't directly access domain.LoginFlow properties,
	// we'll build a generic diagnostic response
	diagnostic := CSRFDiagnosticResponse{
		FlowID:     c.Param("flowId"),
		FlowType:   "login",
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
			"flow_type_note": "Login flow found - check auth-service logs for detailed CSRF processing",
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
package kratos

import (
	"fmt"
	"net/http"
	"strings"

	"auth-service/app/domain"
	"github.com/google/uuid"
	kratosclient "github.com/ory/kratos-client-go"
)

// transformToKratosRegistrationBody transforms our request body to Kratos registration body
func (a *KratosClientAdapter) transformToKratosRegistrationBody(body map[string]interface{}) (interface{}, error) {
	// Extract traits
	traits, err := a.extractTraits(body)
	if err != nil {
		return nil, fmt.Errorf("failed to extract traits: %w", err)
	}

	// Extract password
	password, err := a.extractPassword(body)
	if err != nil {
		return nil, fmt.Errorf("failed to extract password: %w", err)
	}

	// Extract method (default to "password")
	method := "password"
	if m, ok := body["method"].(string); ok {
		method = m
	}

	// Create Kratos registration body
	kratosBody := kratosclient.UpdateRegistrationFlowWithPasswordMethod{
		Traits:   traits,
		Password: password,
		Method:   method,
	}

	a.logger.Debug("transformed registration body",
		"method", method,
		"traits_keys", getMapKeys(traits))

	return kratosBody, nil
}

// transformToKratosLoginBody transforms our request body to Kratos login body
func (a *KratosClientAdapter) transformToKratosLoginBody(body map[string]interface{}) (interface{}, error) {
	// Extract identifier (email)
	identifier, err := a.extractIdentifier(body)
	if err != nil {
		return nil, fmt.Errorf("failed to extract identifier: %w", err)
	}

	// Extract password
	password, err := a.extractPassword(body)
	if err != nil {
		return nil, fmt.Errorf("failed to extract password: %w", err)
	}

	// Extract method (default to "password")
	method := "password"
	if m, ok := body["method"].(string); ok {
		method = m
	}

	// Extract CSRF token with enhanced debugging
	csrfToken, err := a.extractCSRFToken(body)
	if err != nil {
		a.logger.Error("CSRF token extraction failed",
			"error", err,
			"body_keys", getBodyKeys(body),
			"body_preview", truncateMap(body, 5))
		return nil, fmt.Errorf("failed to extract CSRF token: %w", err)
	}

	// 🚨 CRITICAL: Enhanced CSRF token validation and logging
	if csrfToken == "" {
		a.logger.Error("CSRF token is empty after extraction",
			"body_csrf_fields", getCSRFRelatedFields(body))
		return nil, fmt.Errorf("CSRF token is empty")
	}

	// Create Kratos login body
	kratosBody := kratosclient.UpdateLoginFlowWithPasswordMethod{
		Identifier: identifier,
		Password:   password,
		Method:     method,
		CsrfToken:  &csrfToken,
	}

	// 🎯 CRITICAL: Detailed CSRF token logging for debugging
	a.logger.Info("transformed login body with CSRF details",
		"method", method,
		"identifier_present", identifier != "",
		"identifier_masked", maskEmail(identifier),
		"csrf_token_present", csrfToken != "",
		"csrf_token_length", len(csrfToken),
		"csrf_token_prefix", getSafePrefix(csrfToken, 8),
		"csrf_token_suffix", getSafeSuffix(csrfToken, 8),
		"kratosBody_csrf_ptr", kratosBody.CsrfToken != nil)

	return kratosBody, nil
}

// extractTraits extracts traits from request body
func (a *KratosClientAdapter) extractTraits(body map[string]interface{}) (map[string]interface{}, error) {
	// First, try to get traits directly
	if traits, ok := body["traits"].(map[string]interface{}); ok {
		// Validate email is present
		if _, hasEmail := traits["email"]; !hasEmail {
			return nil, fmt.Errorf("traits missing required email field")
		}
		return traits, nil
	}

	// Fallback: construct traits from individual fields
	traits := make(map[string]interface{})

	// Extract email
	if email, ok := body["email"].(string); ok && email != "" {
		traits["email"] = email
	} else {
		return nil, fmt.Errorf("email is required for registration")
	}

	// Extract name if present
	if name, ok := body["name"].(string); ok && name != "" {
		// Parse name into first/last if it contains spaces
		nameParts := strings.Fields(name)
		if len(nameParts) > 1 {
			traits["name"] = map[string]interface{}{
				"first": nameParts[0],
				"last":  strings.Join(nameParts[1:], " "),
			}
		} else {
			traits["name"] = map[string]interface{}{
				"first": name,
				"last":  "",
			}
		}
	}

	return traits, nil
}

// extractPassword extracts password from request body
func (a *KratosClientAdapter) extractPassword(body map[string]interface{}) (string, error) {
	password, ok := body["password"].(string)
	if !ok || password == "" {
		return "", fmt.Errorf("password is required")
	}
	return password, nil
}

// extractIdentifier extracts identifier (email) from request body for login
func (a *KratosClientAdapter) extractIdentifier(body map[string]interface{}) (string, error) {
	// Try "identifier" field first (Kratos standard)
	if identifier, ok := body["identifier"].(string); ok && identifier != "" {
		return identifier, nil
	}

	// Fallback to "email" field
	if email, ok := body["email"].(string); ok && email != "" {
		return email, nil
	}

	return "", fmt.Errorf("identifier (email) is required for login")
}

// transformKratosSessionResponse transforms Kratos session response to domain session
func (a *KratosClientAdapter) transformKratosSessionResponse(resp interface{}) (*domain.KratosSession, error) {
	// Handle different response types from Kratos
	switch r := resp.(type) {
	case *kratosclient.SuccessfulNativeLogin:
		session := r.GetSession()
		return a.transformSessionToDomain(&session)

	case *kratosclient.SuccessfulNativeRegistration:
		sess := r.GetSession()
		return a.transformSessionToDomain(&sess)

	case *kratosclient.Session:
		return a.transformSessionToDomain(r)

	default:
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
}

// transformSessionToDomain transforms Kratos session to domain session
func (a *KratosClientAdapter) transformSessionToDomain(kratosSession *kratosclient.Session) (*domain.KratosSession, error) {
	session := &domain.KratosSession{
		ID:     kratosSession.Id,
		Active: kratosSession.GetActive(),
	}

	// Transform timestamps
	if kratosSession.ExpiresAt != nil {
		session.ExpiresAt = *kratosSession.ExpiresAt
	}
	if kratosSession.AuthenticatedAt != nil {
		session.AuthenticatedAt = *kratosSession.AuthenticatedAt
	}
	if kratosSession.IssuedAt != nil {
		session.IssuedAt = *kratosSession.IssuedAt
	}

	// Transform AAL
	if aal := kratosSession.GetAuthenticatorAssuranceLevel(); aal != "" {
		session.AuthenticatorAssuranceLevel = string(aal)
	}

	// Transform identity
	if identity := kratosSession.GetIdentity(); &identity != nil {
		session.Identity = a.transformIdentityToDomain(&identity)
	}

	// Transform authentication methods
	if methods := kratosSession.GetAuthenticationMethods(); len(methods) > 0 {
		session.AuthenticationMethods = make([]domain.AuthenticationMethod, 0, len(methods))
		for _, method := range methods {
			domainMethod := domain.AuthenticationMethod{
				Method: method.GetMethod(),
				AAL:    string(method.GetAal()),
			}
			if completedAt := method.GetCompletedAt(); !completedAt.IsZero() {
				domainMethod.CompletedAt = completedAt
			}
			session.AuthenticationMethods = append(session.AuthenticationMethods, domainMethod)
		}
	}

	return session, nil
}

// transformIdentityToDomain transforms Kratos identity to domain identity
func (a *KratosClientAdapter) transformIdentityToDomain(kratosIdentity *kratosclient.Identity) *domain.KratosIdentity {
	identity := &domain.KratosIdentity{
		ID:           kratosIdentity.Id,
		SchemaID:     kratosIdentity.GetSchemaId(),
		SchemaURL:    kratosIdentity.GetSchemaUrl(),
		State:        string(kratosIdentity.GetState()),
	}

	// Safely convert traits
	if traits := kratosIdentity.GetTraits(); traits != nil {
		if traitsMap, ok := traits.(map[string]interface{}); ok {
			identity.Traits = traitsMap
		}
	}

	// Transform timestamps
	if stateChangedAt := kratosIdentity.GetStateChangedAt(); !stateChangedAt.IsZero() {
		identity.StateChangedAt = stateChangedAt
	}
	if createdAt := kratosIdentity.GetCreatedAt(); !createdAt.IsZero() {
		identity.CreatedAt = createdAt
	}
	if updatedAt := kratosIdentity.GetUpdatedAt(); !updatedAt.IsZero() {
		identity.UpdatedAt = updatedAt
	}

	// Transform metadata
	if metadataPublic := kratosIdentity.GetMetadataPublic(); metadataPublic != nil {
		if pubMap, ok := metadataPublic.(map[string]interface{}); ok {
			identity.MetadataPublic = pubMap
		}
	}
	if metadataAdmin := kratosIdentity.GetMetadataAdmin(); metadataAdmin != nil {
		if adminMap, ok := metadataAdmin.(map[string]interface{}); ok {
			identity.MetadataAdmin = adminMap
		}
	}

	return identity
}

// transformKratosRegistrationFlowResponse transforms Kratos registration flow to domain
func (a *KratosClientAdapter) transformKratosRegistrationFlowResponse(kratosFlow *kratosclient.RegistrationFlow, tenantID uuid.UUID) (*domain.RegistrationFlow, error) {
	flow := &domain.RegistrationFlow{
		ID:        kratosFlow.Id,
		Type:      kratosFlow.GetType(),
		TenantID:  tenantID,
	}

	// Transform timestamps
	if expiresAt := kratosFlow.GetExpiresAt(); !expiresAt.IsZero() {
		flow.ExpiresAt = expiresAt
	}
	if issuedAt := kratosFlow.GetIssuedAt(); !issuedAt.IsZero() {
		flow.IssuedAt = issuedAt
	}

	// Transform URLs
	if requestURL := kratosFlow.GetRequestUrl(); requestURL != "" {
		flow.RequestURL = requestURL
	}
	if returnTo := kratosFlow.GetReturnTo(); returnTo != "" {
		flow.ReturnTo = returnTo
	}

	// Transform UI
	ui := kratosFlow.GetUi()
	flow.UI = a.transformUIToDomain(&ui)

	return flow, nil
}

// transformKratosLoginFlowResponse transforms Kratos login flow to domain
func (a *KratosClientAdapter) transformKratosLoginFlowResponse(kratosFlow *kratosclient.LoginFlow, tenantID uuid.UUID) (*domain.LoginFlow, error) {
	flow := &domain.LoginFlow{
		ID:       kratosFlow.Id,
		Type:     kratosFlow.GetType(),
		TenantID: tenantID,
	}

	// Transform timestamps
	if expiresAt := kratosFlow.GetExpiresAt(); !expiresAt.IsZero() {
		flow.ExpiresAt = expiresAt
	}
	if issuedAt := kratosFlow.GetIssuedAt(); !issuedAt.IsZero() {
		flow.IssuedAt = issuedAt
	}

	// Transform URLs
	if requestURL := kratosFlow.GetRequestUrl(); requestURL != "" {
		flow.RequestURL = requestURL
	}
	if returnTo := kratosFlow.GetReturnTo(); returnTo != "" {
		flow.ReturnTo = returnTo
	}

	// Transform flow-specific fields
	flow.Refresh = kratosFlow.GetRefresh()
	// Note: Forced field may not exist in newer Kratos versions
	// flow.Forced = kratosFlow.GetForced()
	if aal := kratosFlow.GetRequestedAal(); aal != "" {
		flow.RequestedAAL = string(aal)
	}

	// Transform UI
	ui := kratosFlow.GetUi()
	flow.UI = a.transformUIToDomain(&ui)

	// Extract CSRF token from UI nodes
	csrfToken := a.extractCSRFTokenFromUI(&ui)
	if csrfToken != "" {
		flow.CSRFToken = csrfToken
		a.logger.Debug("extracted CSRF token from login flow",
			"flow_id", flow.ID,
			"csrf_token_present", true)
	} else {
		a.logger.Warn("no CSRF token found in login flow UI",
			"flow_id", flow.ID)
	}

	return flow, nil
}

// transformUIToDomain transforms Kratos UI to domain UI
func (a *KratosClientAdapter) transformUIToDomain(kratosUI *kratosclient.UiContainer) *domain.AuthFlowUI {
	ui := &domain.AuthFlowUI{
		Action: kratosUI.GetAction(),
		Method: kratosUI.GetMethod(),
	}

	// Transform nodes
	if nodes := kratosUI.GetNodes(); len(nodes) > 0 {
		ui.Nodes = make([]*domain.AuthFlowNode, 0, len(nodes))
		for _, node := range nodes {
			domainNode := &domain.AuthFlowNode{
				Type:  node.GetType(),
				Group: node.GetGroup(),
			}
			
			// Convert attributes and meta to map[string]interface{}
			domainNode.Attributes = a.convertToMap(node.GetAttributes())
			domainNode.Meta = a.convertToMap(node.GetMeta())

			// Transform messages
			if messages := node.GetMessages(); len(messages) > 0 {
				domainNode.Messages = make([]*domain.AuthFlowMessage, 0, len(messages))
				for _, msg := range messages {
					domainMessage := &domain.AuthFlowMessage{
						ID:      msg.GetId(),
						Text:    msg.GetText(),
						Type:    msg.GetType(),
						Context: msg.GetContext(),
					}
					domainNode.Messages = append(domainNode.Messages, domainMessage)
				}
			}

			ui.Nodes = append(ui.Nodes, domainNode)
		}
	}

	// Transform messages
	if messages := kratosUI.GetMessages(); len(messages) > 0 {
		ui.Messages = make([]*domain.AuthFlowMessage, 0, len(messages))
		for _, msg := range messages {
			domainMessage := &domain.AuthFlowMessage{
				ID:      msg.GetId(),
				Text:    msg.GetText(),
				Type:    msg.GetType(),
				Context: msg.GetContext(),
			}
			ui.Messages = append(ui.Messages, domainMessage)
		}
	}

	return ui
}

// Utility functions for logging and debugging

// getBodyFieldNames returns field names from body for logging
func getBodyFieldNames(body map[string]interface{}) []string {
	fields := make([]string, 0, len(body))
	for key := range body {
		if key == "password" {
			fields = append(fields, "password:***")
		} else {
			fields = append(fields, key)
		}
	}
	return fields
}

// getMapKeys returns keys from a map for logging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// getHTTPStatus returns HTTP status from response for logging
func getHTTPStatus(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

// getSessionID returns session ID from response for logging
func getSessionID(resp interface{}) string {
	switch r := resp.(type) {
	case *kratosclient.SuccessfulNativeLogin:
		session := r.GetSession()
		return session.Id
	case *kratosclient.SuccessfulNativeRegistration:
		session := r.GetSession()
		return session.Id
	case *kratosclient.Session:
		return r.Id
	}
	return "unknown"
}

// getIdentityID returns identity ID for logging
func getIdentityID(identity interface{}) string {
	switch i := identity.(type) {
	case *kratosclient.Identity:
		return i.Id
	case kratosclient.Identity:
		return i.Id
	}
	return "unknown"
}

// convertToMap converts various types to map[string]interface{}
func (a *KratosClientAdapter) convertToMap(v interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	
	// Try direct type assertion first
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	
	// Handle Kratos UiNodeAttributes specifically
	switch attr := v.(type) {
	case kratosclient.UiNodeAttributes:
		// Check if it has UiNodeInputAttributes
		if inputAttrs := attr.UiNodeInputAttributes; inputAttrs != nil {
			result["name"] = inputAttrs.GetName()
			result["type"] = inputAttrs.GetType()
			result["value"] = inputAttrs.GetValue()
			result["required"] = inputAttrs.GetRequired()
			result["disabled"] = inputAttrs.GetDisabled()
			result["node_type"] = inputAttrs.GetNodeType()
			
			// Add autocomplete if present
			if autocomplete := inputAttrs.GetAutocomplete(); autocomplete != "" {
				result["autocomplete"] = autocomplete
			}
		}
		
		// Check other attribute types as needed
		if textAttrs := attr.UiNodeTextAttributes; textAttrs != nil {
			result["id"] = textAttrs.GetId()
			result["text"] = textAttrs.GetText()
			result["node_type"] = textAttrs.GetNodeType()
		}
		
		if anchorAttrs := attr.UiNodeAnchorAttributes; anchorAttrs != nil {
			result["href"] = anchorAttrs.GetHref()
			result["title"] = anchorAttrs.GetTitle()
			result["node_type"] = anchorAttrs.GetNodeType()
		}
		
		if imageAttrs := attr.UiNodeImageAttributes; imageAttrs != nil {
			result["src"] = imageAttrs.GetSrc()
			result["id"] = imageAttrs.GetId()
			result["width"] = imageAttrs.GetWidth()
			result["height"] = imageAttrs.GetHeight()
			result["node_type"] = imageAttrs.GetNodeType()
		}
		
		if scriptAttrs := attr.UiNodeScriptAttributes; scriptAttrs != nil {
			result["src"] = scriptAttrs.GetSrc()
			result["async"] = scriptAttrs.GetAsync()
			result["referrerpolicy"] = scriptAttrs.GetReferrerpolicy()
			result["type"] = scriptAttrs.GetType()
			result["node_type"] = scriptAttrs.GetNodeType()
		}
		
		return result
	}
	
	// For other struct types, use a simplified approach
	return result
}

// 🎯 CRITICAL: Helper functions for enhanced CSRF debugging
func getBodyKeys(body map[string]interface{}) []string {
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	return keys
}

func getCSRFRelatedFields(body map[string]interface{}) map[string]interface{} {
	csrfFields := make(map[string]interface{})
	for k, v := range body {
		if strings.Contains(strings.ToLower(k), "csrf") || 
		   strings.Contains(strings.ToLower(k), "token") {
			csrfFields[k] = v
		}
	}
	return csrfFields
}

func truncateMap(m map[string]interface{}, maxFields int) map[string]interface{} {
	result := make(map[string]interface{})
	count := 0
	for k, v := range m {
		if count >= maxFields {
			break
		}
		// Safely truncate string values
		if str, ok := v.(string); ok && len(str) > 50 {
			result[k] = str[:50] + "..."
		} else {
			result[k] = v
		}
		count++
	}
	return result
}

func maskEmail(email string) string {
	if email == "" {
		return ""
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***"
	}
	if len(parts[0]) <= 2 {
		return "*@" + parts[1]
	}
	return string(parts[0][0]) + "***@" + parts[1]
}

func getSafePrefix(token string, length int) string {
	if len(token) <= length {
		return "***"
	}
	return token[:length]
}

func getSafeSuffix(token string, length int) string {
	if len(token) <= length {
		return "***"
	}
	return token[len(token)-length:]
}

// extractCSRFToken extracts CSRF token from request body
func (a *KratosClientAdapter) extractCSRFToken(body map[string]interface{}) (string, error) {
	// First, try to get csrf_token directly
	if csrfToken, ok := body["csrf_token"].(string); ok && csrfToken != "" {
		a.logger.Debug("extracted CSRF token from request body",
			"token_length", len(csrfToken))
		return csrfToken, nil
	}

	// Alternative field names that might be used
	alternativeFields := []string{"csrfToken", "csrf", "_csrf", "anti_csrf_token"}
	for _, field := range alternativeFields {
		if csrfToken, ok := body[field].(string); ok && csrfToken != "" {
			a.logger.Debug("extracted CSRF token from alternative field",
				"field", field,
				"token_length", len(csrfToken))
			return csrfToken, nil
		}
	}

	return "", fmt.Errorf("CSRF token is required but not found in request body")
}

// extractCSRFTokenFromUI extracts CSRF token from Kratos UI nodes
func (a *KratosClientAdapter) extractCSRFTokenFromUI(ui *kratosclient.UiContainer) string {
	nodes := ui.GetNodes()
	for _, node := range nodes {
		// Skip non-input nodes
		if node.GetType() != "input" {
			continue
		}
		
		attributes := node.GetAttributes()
		
		// Check if this is an input node with csrf_token name
		if attributes.UiNodeInputAttributes != nil {
			inputAttrs := attributes.UiNodeInputAttributes
			if inputAttrs.GetName() == "csrf_token" {
				// Extract the value
				if value := inputAttrs.GetValue(); value != nil {
					if tokenValue, ok := value.(string); ok && tokenValue != "" {
						a.logger.Debug("found CSRF token in UI node via UiNodeInputAttributes",
							"node_type", node.GetType(),
							"node_group", node.GetGroup(),
							"token_length", len(tokenValue))
						return tokenValue
					}
				}
			}
		}
		
		// Alternative approach: use convertToMap to get structured attributes
		attrMap := a.convertToMap(attributes)
		if name, nameOk := attrMap["name"].(string); nameOk && name == "csrf_token" {
			if value, valueOk := attrMap["value"].(string); valueOk && value != "" {
				a.logger.Debug("found CSRF token in UI node via convertToMap",
					"node_type", node.GetType(),
					"node_group", node.GetGroup(),
					"token_length", len(value))
				return value
			}
		}
	}
	
	a.logger.Warn("no CSRF token found in UI nodes",
		"total_nodes", len(nodes))
	return ""
}
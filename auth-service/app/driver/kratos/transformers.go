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

	// Create Kratos login body
	kratosBody := kratosclient.UpdateLoginFlowWithPasswordMethod{
		Identifier: identifier,
		Password:   password,
		Method:     method,
	}

	a.logger.Debug("transformed login body",
		"method", method,
		"identifier_present", identifier != "")

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
	
	// For struct types, we'll create a basic conversion
	// This is a simplified approach - in production you might want to use reflection
	return result
}
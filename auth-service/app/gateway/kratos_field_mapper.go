package gateway

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"time"
)

// KratosFieldMapper manages field mapping between frontend and Kratos
type KratosFieldMapper struct {
	mappingRules    map[string]FieldMapping
	validationRules map[string][]ValidationRule
	logger          *slog.Logger
	mutex           sync.RWMutex
}

// FieldMapping defines how frontend fields map to Kratos fields
type FieldMapping struct {
	KratosPath     string      `json:"kratos_path"`     // "traits.email", "password"
	FrontendField  string      `json:"frontend_field"`  // "email", "password"
	Required       bool        `json:"required"`
	DefaultValue   interface{} `json:"default_value,omitempty"`
	Transform      string      `json:"transform,omitempty"` // "trim", "lowercase", "name_split"
}

// ValidationRule defines validation rules for fields
type ValidationRule struct {
	Type    string      `json:"type"`    // "email", "minLength", "maxLength", "required"
	Value   interface{} `json:"value,omitempty"`
	Message string      `json:"message"`
}

// MappingResult contains the result of field mapping
type MappingResult struct {
	Success      bool                   `json:"success"`
	KratosData   map[string]interface{} `json:"kratos_data"`
	Warnings     []string               `json:"warnings"`
	Errors       []string               `json:"errors"`
	FieldsUsed   []string               `json:"fields_used"`
	FieldsIgnored []string              `json:"fields_ignored"`
}

// NewKratosFieldMapper creates a new field mapper
func NewKratosFieldMapper(logger *slog.Logger) *KratosFieldMapper {
	mapper := &KratosFieldMapper{
		mappingRules:    make(map[string]FieldMapping),
		validationRules: make(map[string][]ValidationRule),
		logger:          logger,
	}
	
	// Initialize default mapping rules
	mapper.initializeDefaultMappings()
	
	return mapper
}

// initializeDefaultMappings sets up default field mappings for common scenarios
func (m *KratosFieldMapper) initializeDefaultMappings() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Registration mappings
	m.mappingRules["email"] = FieldMapping{
		KratosPath:    "traits.email",
		FrontendField: "email",
		Required:      true,
		Transform:     "trim,lowercase",
	}

	m.mappingRules["password"] = FieldMapping{
		KratosPath:    "password",
		FrontendField: "password",
		Required:      true,
	}

	m.mappingRules["name"] = FieldMapping{
		KratosPath:    "traits.name",
		FrontendField: "name",
		Required:      false,
		Transform:     "name_split",
	}

	m.mappingRules["firstName"] = FieldMapping{
		KratosPath:    "traits.name.first",
		FrontendField: "firstName",
		Required:      false,
		Transform:     "trim",
	}

	m.mappingRules["lastName"] = FieldMapping{
		KratosPath:    "traits.name.last",
		FrontendField: "lastName",
		Required:      false,
		Transform:     "trim",
	}

	// Login mappings
	m.mappingRules["identifier"] = FieldMapping{
		KratosPath:    "identifier",
		FrontendField: "email", // Frontend sends "email" but Kratos expects "identifier"
		Required:      true,
		Transform:     "trim,lowercase",
	}

	// Validation rules
	m.validationRules["email"] = []ValidationRule{
		{Type: "required", Message: "Email is required"},
		{Type: "email", Message: "Invalid email format"},
		{Type: "maxLength", Value: 254, Message: "Email too long"},
	}

	m.validationRules["password"] = []ValidationRule{
		{Type: "required", Message: "Password is required"},
		{Type: "minLength", Value: 8, Message: "Password must be at least 8 characters"},
		{Type: "maxLength", Value: 128, Message: "Password too long"},
	}

	m.validationRules["name"] = []ValidationRule{
		{Type: "maxLength", Value: 100, Message: "Name too long"},
	}

	m.logger.Info("Default field mappings initialized",
		"mapping_count", len(m.mappingRules),
		"validation_count", len(m.validationRules))
}

// MapRegistrationPayload maps frontend registration data to Kratos format
func (m *KratosFieldMapper) MapRegistrationPayload(frontendData map[string]interface{}) (*MappingResult, error) {
	mappingId := fmt.Sprintf("REG-MAP-%d", time.Now().UnixNano())
	m.logger.Info("🔄 Starting registration payload mapping", "mapping_id", mappingId)

	result := &MappingResult{
		KratosData:    make(map[string]interface{}),
		Warnings:      make([]string, 0),
		Errors:        make([]string, 0),
		FieldsUsed:    make([]string, 0),
		FieldsIgnored: make([]string, 0),
		Success:       true,
	}

	// Initialize Kratos structure
	traits := make(map[string]interface{})
	result.KratosData["traits"] = traits
	result.KratosData["method"] = "password"

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Process each frontend field
	for frontendKey, frontendValue := range frontendData {
		if mapping, exists := m.mappingRules[frontendKey]; exists {
			m.logger.Debug("Processing mapped field",
				"mapping_id", mappingId,
				"frontend_field", frontendKey,
				"kratos_path", mapping.KratosPath)

			// Transform value if needed
			transformedValue, err := m.transformValue(frontendValue, mapping.Transform)
			if err != nil {
				errorMsg := fmt.Sprintf("Failed to transform field %s: %v", frontendKey, err)
				result.Errors = append(result.Errors, errorMsg)
				result.Success = false
				continue
			}

			// Validate transformed value
			if validationRules, hasRules := m.validationRules[frontendKey]; hasRules {
				if validationErr := m.validateField(frontendKey, transformedValue, validationRules); validationErr != nil {
					result.Errors = append(result.Errors, validationErr.Error())
					result.Success = false
					continue
				}
			}

			// Set value in Kratos structure
			err = m.setNestedValue(result.KratosData, mapping.KratosPath, transformedValue)
			if err != nil {
				errorMsg := fmt.Sprintf("Failed to set Kratos field %s: %v", mapping.KratosPath, err)
				result.Errors = append(result.Errors, errorMsg)
				result.Success = false
				continue
			}

			result.FieldsUsed = append(result.FieldsUsed, frontendKey)
		} else {
			result.FieldsIgnored = append(result.FieldsIgnored, frontendKey)
			m.logger.Warn("Unknown frontend field ignored",
				"mapping_id", mappingId,
				"field", frontendKey)
		}
	}

	// Check for required fields
	m.checkRequiredFields(result, "registration")

	m.logger.Info("Registration payload mapping completed",
		"mapping_id", mappingId,
		"success", result.Success,
		"fields_used", len(result.FieldsUsed),
		"fields_ignored", len(result.FieldsIgnored),
		"errors", len(result.Errors))

	return result, nil
}

// MapLoginPayload maps frontend login data to Kratos format
func (m *KratosFieldMapper) MapLoginPayload(frontendData map[string]interface{}) (*MappingResult, error) {
	mappingId := fmt.Sprintf("LOGIN-MAP-%d", time.Now().UnixNano())
	m.logger.Info("🔄 Starting login payload mapping", "mapping_id", mappingId)

	result := &MappingResult{
		KratosData:    make(map[string]interface{}),
		Warnings:      make([]string, 0),
		Errors:        make([]string, 0),
		FieldsUsed:    make([]string, 0),
		FieldsIgnored: make([]string, 0),
		Success:       true,
	}

	// Set method for login
	result.KratosData["method"] = "password"

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Process each frontend field
	for frontendKey, frontendValue := range frontendData {
		var mapping FieldMapping
		var exists bool

		// Special handling for login - map "email" to "identifier"
		if frontendKey == "email" {
			mapping = m.mappingRules["identifier"]
			exists = true
		} else {
			mapping, exists = m.mappingRules[frontendKey]
		}

		if exists {
			m.logger.Debug("Processing mapped field",
				"mapping_id", mappingId,
				"frontend_field", frontendKey,
				"kratos_path", mapping.KratosPath)

			// Transform value if needed
			transformedValue, err := m.transformValue(frontendValue, mapping.Transform)
			if err != nil {
				errorMsg := fmt.Sprintf("Failed to transform field %s: %v", frontendKey, err)
				result.Errors = append(result.Errors, errorMsg)
				result.Success = false
				continue
			}

			// Validate transformed value
			validationKey := frontendKey
			if frontendKey == "email" {
				validationKey = "email" // Use email validation for identifier
			}
			if validationRules, hasRules := m.validationRules[validationKey]; hasRules {
				if validationErr := m.validateField(validationKey, transformedValue, validationRules); validationErr != nil {
					result.Errors = append(result.Errors, validationErr.Error())
					result.Success = false
					continue
				}
			}

			// Set value directly (login doesn't use nested structure like traits)
			result.KratosData[mapping.KratosPath] = transformedValue
			result.FieldsUsed = append(result.FieldsUsed, frontendKey)
		} else {
			result.FieldsIgnored = append(result.FieldsIgnored, frontendKey)
			m.logger.Warn("Unknown frontend field ignored",
				"mapping_id", mappingId,
				"field", frontendKey)
		}
	}

	// Check for required fields
	m.checkRequiredFields(result, "login")

	m.logger.Info("Login payload mapping completed",
		"mapping_id", mappingId,
		"success", result.Success,
		"fields_used", len(result.FieldsUsed),
		"fields_ignored", len(result.FieldsIgnored),
		"errors", len(result.Errors))

	return result, nil
}

// transformValue applies transformations to field values
func (m *KratosFieldMapper) transformValue(value interface{}, transforms string) (interface{}, error) {
	if transforms == "" || value == nil {
		return value, nil
	}

	strValue, ok := value.(string)
	if !ok {
		return value, nil // Only transform string values
	}

	transformList := strings.Split(transforms, ",")
	result := strValue

	for _, transform := range transformList {
		transform = strings.TrimSpace(transform)
		switch transform {
		case "trim":
			result = strings.TrimSpace(result)
		case "lowercase":
			result = strings.ToLower(result)
		case "uppercase":
			result = strings.ToUpper(result)
		case "name_split":
			return m.transformNameSplit(result), nil
		}
	}

	return result, nil
}

// transformNameSplit transforms a full name into name structure
func (m *KratosFieldMapper) transformNameSplit(fullName string) map[string]interface{} {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return map[string]interface{}{
			"first": "",
			"last":  "",
		}
	}

	parts := strings.Fields(fullName)
	if len(parts) == 0 {
		return map[string]interface{}{
			"first": "",
			"last":  "",
		}
	}

	firstName := parts[0]
	lastName := ""
	if len(parts) > 1 {
		lastName = strings.Join(parts[1:], " ")
	}

	return map[string]interface{}{
		"first": firstName,
		"last":  lastName,
	}
}

// validateField validates a field value against rules
func (m *KratosFieldMapper) validateField(fieldName string, value interface{}, rules []ValidationRule) error {
	for _, rule := range rules {
		if err := m.applyValidationRule(fieldName, value, rule); err != nil {
			return err
		}
	}
	return nil
}

// applyValidationRule applies a single validation rule
func (m *KratosFieldMapper) applyValidationRule(fieldName string, value interface{}, rule ValidationRule) error {
	switch rule.Type {
	case "required":
		if value == nil || (reflect.TypeOf(value).Kind() == reflect.String && strings.TrimSpace(value.(string)) == "") {
			return fmt.Errorf("%s: %s", fieldName, rule.Message)
		}
	case "email":
		if strValue, ok := value.(string); ok {
			if !m.isValidEmail(strValue) {
				return fmt.Errorf("%s: %s", fieldName, rule.Message)
			}
		}
	case "minLength":
		if strValue, ok := value.(string); ok {
			if minLen, ok := rule.Value.(int); ok {
				if len(strValue) < minLen {
					return fmt.Errorf("%s: %s (minimum %d characters)", fieldName, rule.Message, minLen)
				}
			}
		}
	case "maxLength":
		if strValue, ok := value.(string); ok {
			if maxLen, ok := rule.Value.(int); ok {
				if len(strValue) > maxLen {
					return fmt.Errorf("%s: %s (maximum %d characters)", fieldName, rule.Message, maxLen)
				}
			}
		}
	}
	return nil
}

// isValidEmail performs basic email validation
func (m *KratosFieldMapper) isValidEmail(email string) bool {
	// Basic email validation - more sophisticated validation can be added
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	
	atIndex := strings.Index(email, "@")
	if atIndex <= 0 || atIndex >= len(email)-1 {
		return false
	}
	
	// Check for multiple @ symbols
	if strings.Count(email, "@") != 1 {
		return false
	}
	
	localPart := email[:atIndex]
	domainPart := email[atIndex+1:]
	
	// Basic checks
	if len(localPart) == 0 || len(domainPart) == 0 {
		return false
	}
	
	// Domain must contain at least one dot
	if !strings.Contains(domainPart, ".") {
		return false
	}
	
	return true
}

// setNestedValue sets a value in a nested map structure using dot notation
func (m *KratosFieldMapper) setNestedValue(data map[string]interface{}, path string, value interface{}) error {
	keys := strings.Split(path, ".")
	current := data

	// Navigate to the parent of the target key
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		
		if existing, ok := current[key]; ok {
			if nestedMap, ok := existing.(map[string]interface{}); ok {
				current = nestedMap
			} else {
				return fmt.Errorf("path %s: key %s is not a map", path, key)
			}
		} else {
			// Create new nested map
			newMap := make(map[string]interface{})
			current[key] = newMap
			current = newMap
		}
	}

	// Set the final value
	finalKey := keys[len(keys)-1]
	current[finalKey] = value

	return nil
}

// checkRequiredFields validates that all required fields are present
func (m *KratosFieldMapper) checkRequiredFields(result *MappingResult, flowType string) {
	requiredFieldsForFlow := m.getRequiredFieldsForFlow(flowType)
	
	for _, requiredField := range requiredFieldsForFlow {
		if !m.containsString(result.FieldsUsed, requiredField) {
			errorMsg := fmt.Sprintf("Required field missing: %s", requiredField)
			result.Errors = append(result.Errors, errorMsg)
			result.Success = false
		}
	}
}

// getRequiredFieldsForFlow returns required fields for a specific flow type
func (m *KratosFieldMapper) getRequiredFieldsForFlow(flowType string) []string {
	switch flowType {
	case "registration":
		return []string{"email", "password"}
	case "login":
		return []string{"email", "password"} // email is mapped to identifier
	default:
		return []string{}
	}
}

// containsString checks if a slice contains a string
func (m *KratosFieldMapper) containsString(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

// AddCustomMapping adds a custom field mapping
func (m *KratosFieldMapper) AddCustomMapping(frontendField string, mapping FieldMapping) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.mappingRules[frontendField] = mapping
	m.logger.Info("Custom mapping added",
		"frontend_field", frontendField,
		"kratos_path", mapping.KratosPath)
}

// GetMappingRules returns all current mapping rules
func (m *KratosFieldMapper) GetMappingRules() map[string]FieldMapping {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// Return a copy to prevent external modification
	rules := make(map[string]FieldMapping)
	for k, v := range m.mappingRules {
		rules[k] = v
	}
	
	return rules
}
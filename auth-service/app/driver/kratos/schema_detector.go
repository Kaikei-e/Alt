package kratos

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"auth-service/app/domain"
)

// KratosSchema represents the detected Kratos identity schema
type KratosSchema struct {
	RequiredFields    map[string]FieldSpec `json:"required_fields"`
	OptionalFields    map[string]FieldSpec `json:"optional_fields"`
	ContentType       string               `json:"content_type"`
	Method            string               `json:"method"`
	DetectedAt        time.Time            `json:"detected_at"`
	SchemaVersion     string               `json:"schema_version"`
}

// FieldSpec defines field specifications
type FieldSpec struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`         // "email", "text", "password"
	KratosPath   string      `json:"kratos_path"`  // "traits.email", "password"
	Required     bool        `json:"required"`
	DefaultValue interface{} `json:"default_value,omitempty"`
	Validation   []string    `json:"validation,omitempty"`
}

// KratosSchemaDetector detects and caches Kratos schema information
type KratosSchemaDetector struct {
	client      *Client
	logger      *slog.Logger
	
	// Schema cache with TTL
	schemaMutex     sync.RWMutex
	registrationSchema *KratosSchema
	loginSchema     *KratosSchema
	cacheExpiry     time.Time
	cacheTTL        time.Duration
}

// NewKratosSchemaDetector creates a new schema detector
func NewKratosSchemaDetector(client *Client, logger *slog.Logger) *KratosSchemaDetector {
	return &KratosSchemaDetector{
		client:   client,
		logger:   logger,
		cacheTTL: 15 * time.Minute, // Schema cache for 15 minutes
	}
}

// DetectRegistrationSchema detects and returns registration schema
func (d *KratosSchemaDetector) DetectRegistrationSchema(ctx context.Context) (*KratosSchema, error) {
	d.schemaMutex.RLock()
	if d.registrationSchema != nil && time.Now().Before(d.cacheExpiry) {
		d.schemaMutex.RUnlock()
		d.logger.Debug("Returning cached registration schema")
		return d.registrationSchema, nil
	}
	d.schemaMutex.RUnlock()

	d.logger.Info("🔍 Detecting Kratos registration schema...")
	
	schema, err := d.detectSchemaFromFlow(ctx, "registration")
	if err != nil {
		return nil, fmt.Errorf("failed to detect registration schema: %w", err)
	}

	d.schemaMutex.Lock()
	d.registrationSchema = schema
	d.cacheExpiry = time.Now().Add(d.cacheTTL)
	d.schemaMutex.Unlock()

	d.logger.Info("✅ Registration schema detected successfully",
		"required_fields", len(schema.RequiredFields),
		"optional_fields", len(schema.OptionalFields),
		"content_type", schema.ContentType)

	return schema, nil
}

// DetectLoginSchema detects and returns login schema
func (d *KratosSchemaDetector) DetectLoginSchema(ctx context.Context) (*KratosSchema, error) {
	d.schemaMutex.RLock()
	if d.loginSchema != nil && time.Now().Before(d.cacheExpiry) {
		d.schemaMutex.RUnlock()
		d.logger.Debug("Returning cached login schema")
		return d.loginSchema, nil
	}
	d.schemaMutex.RUnlock()

	d.logger.Info("🔍 Detecting Kratos login schema...")
	
	schema, err := d.detectSchemaFromFlow(ctx, "login")
	if err != nil {
		return nil, fmt.Errorf("failed to detect login schema: %w", err)
	}

	d.schemaMutex.Lock()
	d.loginSchema = schema
	d.cacheExpiry = time.Now().Add(d.cacheTTL)
	d.schemaMutex.Unlock()

	d.logger.Info("✅ Login schema detected successfully",
		"required_fields", len(schema.RequiredFields),
		"optional_fields", len(schema.OptionalFields),
		"content_type", schema.ContentType)

	return schema, nil
}

// detectSchemaFromFlow detects schema by analyzing Kratos flow
func (d *KratosSchemaDetector) detectSchemaFromFlow(ctx context.Context, flowType string) (*KratosSchema, error) {
	startTime := time.Now()
	d.logger.Debug("Starting schema detection", "flow_type", flowType)

	// Create a test flow to analyze its structure
	var flow interface{}
	var err error

	switch flowType {
	case "registration":
		flow, err = d.createTestRegistrationFlow(ctx)
	case "login":
		flow, err = d.createTestLoginFlow(ctx)
	default:
		return nil, fmt.Errorf("unsupported flow type: %s", flowType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create test %s flow: %w", flowType, err)
	}

	// Analyze flow structure
	schema, err := d.analyzeFlowStructure(flow, flowType)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze flow structure: %w", err)
	}

	duration := time.Since(startTime)
	d.logger.Info("Schema detection completed",
		"flow_type", flowType,
		"duration", duration.String(),
		"fields_found", len(schema.RequiredFields)+len(schema.OptionalFields))

	return schema, nil
}

// createTestRegistrationFlow creates a registration flow for analysis
func (d *KratosSchemaDetector) createTestRegistrationFlow(ctx context.Context) (*domain.RegistrationFlow, error) {
	// This should use the existing Kratos client to create a flow
	adapter := NewKratosClientAdapter(d.client, d.logger)
	// Use a dummy tenant ID and return URL for schema detection
	dummyTenantID := uuid.New()
	return adapter.CreateRegistrationFlow(ctx, dummyTenantID, "")
}

// createTestLoginFlow creates a login flow for analysis
func (d *KratosSchemaDetector) createTestLoginFlow(ctx context.Context) (*domain.LoginFlow, error) {
	// This should use the existing Kratos client to create a flow
	adapter := NewKratosClientAdapter(d.client, d.logger)
	// Use a dummy tenant ID, no refresh, and empty return URL for schema detection
	dummyTenantID := uuid.New()
	return adapter.CreateLoginFlow(ctx, dummyTenantID, false, "")
}

// analyzeFlowStructure analyzes the flow to determine schema
func (d *KratosSchemaDetector) analyzeFlowStructure(flow interface{}, flowType string) (*KratosSchema, error) {
	schema := &KratosSchema{
		RequiredFields: make(map[string]FieldSpec),
		OptionalFields: make(map[string]FieldSpec),
		DetectedAt:     time.Now(),
		SchemaVersion:  "1.0",
	}

	// Determine content type based on flow type and Kratos version
	// Registration flows typically prefer form-urlencoded
	if flowType == "registration" {
		schema.ContentType = "application/x-www-form-urlencoded"
		schema.Method = "password"
	} else {
		schema.ContentType = "application/x-www-form-urlencoded"
		schema.Method = "password"
	}

	var flowData map[string]interface{}
	
	// Convert flow to map for analysis
	flowJSON, err := json.Marshal(flow)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal flow: %w", err)
	}
	
	if err := json.Unmarshal(flowJSON, &flowData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flow: %w", err)
	}

	// Analyze UI nodes to determine field structure
	if ui, ok := flowData["ui"].(map[string]interface{}); ok {
		if nodes, ok := ui["nodes"].([]interface{}); ok {
			d.analyzeUINodes(nodes, schema)
		}
	}

	// Add common fields if not detected
	d.addCommonFields(schema, flowType)

	return schema, nil
}

// analyzeUINodes analyzes UI nodes to extract field information
func (d *KratosSchemaDetector) analyzeUINodes(nodes []interface{}, schema *KratosSchema) {
	d.logger.Debug("Analyzing UI nodes", "node_count", len(nodes))

	for _, node := range nodes {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract field attributes
		if attrs, ok := nodeMap["attributes"].(map[string]interface{}); ok {
			fieldSpec := d.extractFieldSpec(attrs, nodeMap)
			if fieldSpec.Name != "" {
				if fieldSpec.Required {
					schema.RequiredFields[fieldSpec.Name] = fieldSpec
				} else {
					schema.OptionalFields[fieldSpec.Name] = fieldSpec
				}
				
				d.logger.Debug("Field detected",
					"name", fieldSpec.Name,
					"type", fieldSpec.Type,
					"kratos_path", fieldSpec.KratosPath,
					"required", fieldSpec.Required)
			}
		}
	}
}

// extractFieldSpec extracts field specification from node attributes
func (d *KratosSchemaDetector) extractFieldSpec(attrs map[string]interface{}, node map[string]interface{}) FieldSpec {
	spec := FieldSpec{}

	// Extract name and type
	if name, ok := attrs["name"].(string); ok {
		spec.Name = d.extractFieldName(name)
		spec.KratosPath = name
	}

	if fieldType, ok := attrs["type"].(string); ok {
		spec.Type = fieldType
	}

	// Determine if required
	if required, ok := attrs["required"].(bool); ok {
		spec.Required = required
	}

	// Check for required attribute in node metadata
	if nodeType, ok := node["type"].(string); ok && nodeType == "input" {
		if group, ok := node["group"].(string); ok {
			// Password fields are typically required
			if strings.Contains(spec.KratosPath, "password") || group == "password" {
				spec.Required = true
			}
			// Email fields are typically required for registration
			if strings.Contains(spec.KratosPath, "email") || spec.Type == "email" {
				spec.Required = true
			}
		}
	}

	return spec
}

// extractFieldName extracts simplified field name from Kratos path
func (d *KratosSchemaDetector) extractFieldName(kratosPath string) string {
	// Convert "traits.email" -> "email"
	// Convert "password" -> "password"
	// Convert "traits.name.first" -> "name"
	
	if strings.HasPrefix(kratosPath, "traits.") {
		path := strings.TrimPrefix(kratosPath, "traits.")
		if strings.Contains(path, ".") {
			// Handle nested fields like "name.first"
			return strings.Split(path, ".")[0]
		}
		return path
	}
	
	return kratosPath
}

// addCommonFields adds commonly expected fields if not detected
func (d *KratosSchemaDetector) addCommonFields(schema *KratosSchema, flowType string) {
	if flowType == "registration" {
		// Ensure email field exists
		if _, exists := schema.RequiredFields["email"]; !exists {
			if _, exists := schema.OptionalFields["email"]; !exists {
				schema.RequiredFields["email"] = FieldSpec{
					Name:       "email",
					Type:       "email",
					KratosPath: "traits.email",
					Required:   true,
				}
			}
		}

		// Ensure password field exists
		if _, exists := schema.RequiredFields["password"]; !exists {
			schema.RequiredFields["password"] = FieldSpec{
				Name:       "password",
				Type:       "password",
				KratosPath: "password",
				Required:   true,
			}
		}

		// Add optional name field
		if _, exists := schema.RequiredFields["name"]; !exists {
			if _, exists := schema.OptionalFields["name"]; !exists {
				schema.OptionalFields["name"] = FieldSpec{
					Name:       "name",
					Type:       "text",
					KratosPath: "traits.name",
					Required:   false,
				}
			}
		}
	}

	if flowType == "login" {
		// Ensure identifier field exists
		if _, exists := schema.RequiredFields["identifier"]; !exists {
			schema.RequiredFields["identifier"] = FieldSpec{
				Name:       "identifier",
				Type:       "email",
				KratosPath: "identifier",
				Required:   true,
			}
		}

		// Ensure password field exists
		if _, exists := schema.RequiredFields["password"]; !exists {
			schema.RequiredFields["password"] = FieldSpec{
				Name:       "password",
				Type:       "password",
				KratosPath: "password",
				Required:   true,
			}
		}
	}
}

// GetRegistrationContentType returns the preferred content type for registration
func (d *KratosSchemaDetector) GetRegistrationContentType(ctx context.Context) (string, error) {
	schema, err := d.DetectRegistrationSchema(ctx)
	if err != nil {
		return "", err
	}
	return schema.ContentType, nil
}

// GetLoginContentType returns the preferred content type for login
func (d *KratosSchemaDetector) GetLoginContentType(ctx context.Context) (string, error) {
	schema, err := d.DetectLoginSchema(ctx)
	if err != nil {
		return "", err
	}
	return schema.ContentType, nil
}

// InvalidateCache clears the schema cache
func (d *KratosSchemaDetector) InvalidateCache() {
	d.schemaMutex.Lock()
	defer d.schemaMutex.Unlock()
	
	d.registrationSchema = nil
	d.loginSchema = nil
	d.cacheExpiry = time.Time{}
	
	d.logger.Info("Schema cache invalidated")
}
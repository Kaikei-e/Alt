// Phase R4: バリデーション設定 - 設定値バリデーション・型変換・必須項目チェック
package config

import (
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ValidationRule represents a single validation rule
type ValidationRule struct {
	Name        string
	Description string
	Validator   func(interface{}) error
}

// RequiredValidator validates that a value is not empty
type RequiredValidator struct {
	Message string
}

// Validate implements ConfigValidator interface
func (v *RequiredValidator) Validate(key string, value interface{}) error {
	if value == nil {
		return fmt.Errorf("%s is required", key)
	}

	// Check for empty strings
	if str, ok := value.(string); ok && str == "" {
		if v.Message != "" {
			return fmt.Errorf("%s", v.Message)
		}
		return fmt.Errorf("%s cannot be empty", key)
	}

	// Check for zero values
	reflectValue := reflect.ValueOf(value)
	if reflectValue.IsZero() {
		if v.Message != "" {
			return fmt.Errorf("%s", v.Message)
		}
		return fmt.Errorf("%s cannot be zero value", key)
	}

	return nil
}

// TypeValidator validates that a value is of expected type
type TypeValidator struct {
	ExpectedType reflect.Type
	Message      string
}

// Validate implements ConfigValidator interface
func (v *TypeValidator) Validate(key string, value interface{}) error {
	if value == nil {
		return nil // Allow nil values unless required validator is also used
	}

	actualType := reflect.TypeOf(value)
	if !actualType.AssignableTo(v.ExpectedType) {
		if v.Message != "" {
			return fmt.Errorf("%s", v.Message)
		}
		return fmt.Errorf("%s must be of type %s, got %s", key, v.ExpectedType, actualType)
	}

	return nil
}

// RangeValidator validates that numeric values are within range
type RangeValidator struct {
	Min     interface{}
	Max     interface{}
	Message string
}

// Validate implements ConfigValidator interface
func (v *RangeValidator) Validate(key string, value interface{}) error {
	if value == nil {
		return nil
	}

	// Convert to float64 for comparison
	floatValue, err := v.toFloat64(value)
	if err != nil {
		return fmt.Errorf("%s must be a numeric value: %w", key, err)
	}

	if v.Min != nil {
		minFloat, err := v.toFloat64(v.Min)
		if err != nil {
			return fmt.Errorf("invalid minimum value: %w", err)
		}
		if floatValue < minFloat {
			if v.Message != "" {
				return fmt.Errorf(v.Message)
			}
			return fmt.Errorf("%s must be at least %v", key, v.Min)
		}
	}

	if v.Max != nil {
		maxFloat, err := v.toFloat64(v.Max)
		if err != nil {
			return fmt.Errorf("invalid maximum value: %w", err)
		}
		if floatValue > maxFloat {
			if v.Message != "" {
				return fmt.Errorf(v.Message)
			}
			return fmt.Errorf("%s must be at most %v", key, v.Max)
		}
	}

	return nil
}

// toFloat64 converts various numeric types to float64
func (v *RangeValidator) toFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

// RegexValidator validates that string values match a regular expression
type RegexValidator struct {
	Pattern *regexp.Regexp
	Message string
}

// NewRegexValidator creates a new regex validator
func NewRegexValidator(pattern string, message string) (*RegexValidator, error) {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	return &RegexValidator{
		Pattern: regex,
		Message: message,
	}, nil
}

// Validate implements ConfigValidator interface
func (v *RegexValidator) Validate(key string, value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("%s must be a string for regex validation", key)
	}

	if !v.Pattern.MatchString(str) {
		if v.Message != "" {
			return fmt.Errorf("%s", v.Message)
		}
		return fmt.Errorf("%s does not match required pattern %s", key, v.Pattern.String())
	}

	return nil
}

// EnumValidator validates that values are from allowed set
type EnumValidator struct {
	AllowedValues []interface{}
	Message       string
}

// Validate implements ConfigValidator interface
func (v *EnumValidator) Validate(key string, value interface{}) error {
	if value == nil {
		return nil
	}

	for _, allowed := range v.AllowedValues {
		if reflect.DeepEqual(value, allowed) {
			return nil
		}
	}

	if v.Message != "" {
		return fmt.Errorf(v.Message)
	}

	allowedStrings := make([]string, len(v.AllowedValues))
	for i, v := range v.AllowedValues {
		allowedStrings[i] = fmt.Sprintf("%v", v)
	}

	return fmt.Errorf("%s must be one of: %s", key, strings.Join(allowedStrings, ", "))
}

// URLValidator validates that string values are valid URLs
type URLValidator struct {
	Schemes []string // Optional: restrict to specific schemes
	Message string
}

// Validate implements ConfigValidator interface
func (v *URLValidator) Validate(key string, value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("%s must be a string for URL validation", key)
	}

	if str == "" {
		return nil // Allow empty URLs unless required validator is used
	}

	parsedURL, err := url.Parse(str)
	if err != nil {
		if v.Message != "" {
			return fmt.Errorf("%s", v.Message)
		}
		return fmt.Errorf("%s is not a valid URL: %w", key, err)
	}

	if len(v.Schemes) > 0 {
		validScheme := false
		for _, scheme := range v.Schemes {
			if parsedURL.Scheme == scheme {
				validScheme = true
				break
			}
		}
		if !validScheme {
			return fmt.Errorf("%s must use one of these schemes: %s", key, strings.Join(v.Schemes, ", "))
		}
	}

	return nil
}

// DurationValidator validates that values are valid duration strings or duration objects
type DurationValidator struct {
	MinDuration time.Duration
	MaxDuration time.Duration
	Message     string
}

// Validate implements ConfigValidator interface
func (v *DurationValidator) Validate(key string, value interface{}) error {
	if value == nil {
		return nil
	}

	var duration time.Duration

	switch val := value.(type) {
	case time.Duration:
		duration = val
	case string:
		var err error
		duration, err = time.ParseDuration(val)
		if err != nil {
			if v.Message != "" {
				return fmt.Errorf(v.Message)
			}
			return fmt.Errorf("%s is not a valid duration: %w", key, err)
		}
	default:
		return fmt.Errorf("%s must be a duration string or duration object", key)
	}

	if v.MinDuration > 0 && duration < v.MinDuration {
		return fmt.Errorf("%s must be at least %s", key, v.MinDuration)
	}

	if v.MaxDuration > 0 && duration > v.MaxDuration {
		return fmt.Errorf("%s must be at most %s", key, v.MaxDuration)
	}

	return nil
}

// CompositeValidator combines multiple validators
type CompositeValidator struct {
	Validators []ConfigValidator
	StopOnFirstError bool
}

// Validate implements ConfigValidator interface
func (v *CompositeValidator) Validate(key string, value interface{}) error {
	var errors []string

	for _, validator := range v.Validators {
		if err := validator.Validate(key, value); err != nil {
			if v.StopOnFirstError {
				return err
			}
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed for %s: %s", key, strings.Join(errors, "; "))
	}

	return nil
}

// ValidationConfigBuilder helps build validation configurations
type ValidationConfigBuilder struct {
	manager *ConfigManager
}

// NewValidationConfigBuilder creates a new validation config builder
func NewValidationConfigBuilder(manager *ConfigManager) *ValidationConfigBuilder {
	return &ValidationConfigBuilder{
		manager: manager,
	}
}

// AddRequired adds a required validator for a key
func (b *ValidationConfigBuilder) AddRequired(key string, message ...string) *ValidationConfigBuilder {
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	b.manager.AddValidator(key, &RequiredValidator{Message: msg})
	return b
}

// AddType adds a type validator for a key
func (b *ValidationConfigBuilder) AddType(key string, expectedType reflect.Type, message ...string) *ValidationConfigBuilder {
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	b.manager.AddValidator(key, &TypeValidator{ExpectedType: expectedType, Message: msg})
	return b
}

// AddRange adds a range validator for a key
func (b *ValidationConfigBuilder) AddRange(key string, min, max interface{}, message ...string) *ValidationConfigBuilder {
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	b.manager.AddValidator(key, &RangeValidator{Min: min, Max: max, Message: msg})
	return b
}

// AddRegex adds a regex validator for a key
func (b *ValidationConfigBuilder) AddRegex(key string, pattern string, message ...string) *ValidationConfigBuilder {
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	
	validator, err := NewRegexValidator(pattern, msg)
	if err == nil {
		b.manager.AddValidator(key, validator)
	}
	return b
}

// AddEnum adds an enum validator for a key
func (b *ValidationConfigBuilder) AddEnum(key string, allowedValues []interface{}, message ...string) *ValidationConfigBuilder {
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	b.manager.AddValidator(key, &EnumValidator{AllowedValues: allowedValues, Message: msg})
	return b
}

// AddURL adds a URL validator for a key
func (b *ValidationConfigBuilder) AddURL(key string, schemes []string, message ...string) *ValidationConfigBuilder {
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	b.manager.AddValidator(key, &URLValidator{Schemes: schemes, Message: msg})
	return b
}

// AddDuration adds a duration validator for a key
func (b *ValidationConfigBuilder) AddDuration(key string, minDuration, maxDuration time.Duration, message ...string) *ValidationConfigBuilder {
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	b.manager.AddValidator(key, &DurationValidator{
		MinDuration: minDuration, 
		MaxDuration: maxDuration, 
		Message: msg,
	})
	return b
}

// SetupStandardValidations sets up standard validation rules for deploy-cli
func (b *ValidationConfigBuilder) SetupStandardValidations() *ValidationConfigBuilder {
	// Helm configuration validations
	b.AddDuration("helm.timeout", time.Minute, time.Hour, "Helm timeout must be between 1 minute and 1 hour")
	b.AddRange("helm.max_retries", 0, 10, "Helm max retries must be between 0 and 10")
	b.AddDuration("helm.retry_delay", time.Second, 5*time.Minute, "Helm retry delay must be between 1 second and 5 minutes")

	// Kubectl configuration validations
	b.AddDuration("kubectl.timeout", 30*time.Second, 30*time.Minute, "Kubectl timeout must be between 30 seconds and 30 minutes")
	b.AddRange("kubectl.max_retries", 0, 5, "Kubectl max retries must be between 0 and 5")

	// Deployment configuration validations
	b.AddRange("deployment.parallel.max_workers", 1, 10, "Max parallel workers must be between 1 and 10")
	b.AddDuration("deployment.layer_timeout", 5*time.Minute, 2*time.Hour, "Layer timeout must be between 5 minutes and 2 hours")
	b.AddDuration("health_check.timeout", 5*time.Second, 5*time.Minute, "Health check timeout must be between 5 seconds and 5 minutes")
	b.AddRange("health_check.retries", 1, 20, "Health check retries must be between 1 and 20")

	// Logging configuration validations
	b.AddEnum("logging.level", []interface{}{"debug", "info", "warn", "error"}, "Logging level must be one of: debug, info, warn, error")
	b.AddEnum("logging.format", []interface{}{"json", "text"}, "Logging format must be either json or text")

	// Security configuration validations
	b.AddRegex("ssl.certificate_path", `^(/[^/\x00]+)+/?$`, "SSL certificate path must be a valid absolute path")

	return b
}
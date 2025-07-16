package validator

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Validator wraps the go-playground validator with custom rules
type Validator struct {
	validator *validator.Validate
}

// New creates a new validator instance with custom rules
func New() *Validator {
	validate := validator.New()

	// Register custom validators
	registerCustomValidators(validate)

	// Use JSON field names for validation error messages
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	return &Validator{
		validator: validate,
	}
}

// Validate validates a struct and returns validation errors
func (v *Validator) Validate(i interface{}) error {
	if err := v.validator.Struct(i); err != nil {
		return NewValidationError(err.(validator.ValidationErrors))
	}
	return nil
}

// ValidateVar validates a single variable
func (v *Validator) ValidateVar(field interface{}, tag string) error {
	return v.validator.Var(field, tag)
}

// ValidationError represents a validation error with user-friendly messages
type ValidationError struct {
	Errors map[string]string `json:"errors"`
}

// Error implements the error interface
func (e ValidationError) Error() string {
	var messages []string
	for field, message := range e.Errors {
		messages = append(messages, fmt.Sprintf("%s: %s", field, message))
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(messages, ", "))
}

// NewValidationError creates a ValidationError from validator.ValidationErrors
func NewValidationError(errs validator.ValidationErrors) *ValidationError {
	errors := make(map[string]string)
	
	for _, err := range errs {
		field := err.Field()
		tag := err.Tag()
		
		switch tag {
		case "required":
			errors[field] = fmt.Sprintf("%s is required", field)
		case "email":
			errors[field] = fmt.Sprintf("%s must be a valid email address", field)
		case "min":
			errors[field] = fmt.Sprintf("%s must be at least %s characters long", field, err.Param())
		case "max":
			errors[field] = fmt.Sprintf("%s must be at most %s characters long", field, err.Param())
		case "uuid4":
			errors[field] = fmt.Sprintf("%s must be a valid UUID", field)
		case "password":
			errors[field] = "password must contain at least 8 characters with uppercase, lowercase, number and special character"
		case "username":
			errors[field] = "username must contain only letters, numbers, dots, hyphens and underscores"
		case "slug":
			errors[field] = "slug must contain only lowercase letters, numbers and hyphens"
		case "url":
			errors[field] = fmt.Sprintf("%s must be a valid URL", field)
		case "csrf_token":
			errors[field] = "CSRF token must be a valid base64 encoded string"
		default:
			errors[field] = fmt.Sprintf("%s is invalid", field)
		}
	}
	
	return &ValidationError{Errors: errors}
}

// registerCustomValidators registers custom validation rules
func registerCustomValidators(validate *validator.Validate) {
	// Password validation: at least 8 chars with upper, lower, number and special char
	validate.RegisterValidation("password", func(fl validator.FieldLevel) bool {
		password := fl.Field().String()
		if len(password) < 8 {
			return false
		}
		
		hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
		hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
		hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
		hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password)
		
		return hasUpper && hasLower && hasNumber && hasSpecial
	})
	
	// Username validation: letters, numbers, dots, hyphens, underscores
	validate.RegisterValidation("username", func(fl validator.FieldLevel) bool {
		username := fl.Field().String()
		matched, _ := regexp.MatchString(`^[a-zA-Z0-9._-]+$`, username)
		return matched && len(username) >= 3 && len(username) <= 30
	})
	
	// Slug validation: lowercase letters, numbers, hyphens
	validate.RegisterValidation("slug", func(fl validator.FieldLevel) bool {
		slug := fl.Field().String()
		matched, _ := regexp.MatchString(`^[a-z0-9-]+$`, slug)
		return matched && len(slug) >= 2 && len(slug) <= 50
	})
	
	// CSRF token validation: base64 encoded string
	validate.RegisterValidation("csrf_token", func(fl validator.FieldLevel) bool {
		token := fl.Field().String()
		matched, _ := regexp.MatchString(`^[A-Za-z0-9+/]*={0,2}$`, token)
		return matched && len(token) >= 32
	})
	
	// Role validation: valid user roles
	validate.RegisterValidation("user_role", func(fl validator.FieldLevel) bool {
		role := fl.Field().String()
		validRoles := []string{"admin", "user", "readonly"}
		for _, validRole := range validRoles {
			if role == validRole {
				return true
			}
		}
		return false
	})
	
	// Status validation: valid user status
	validate.RegisterValidation("user_status", func(fl validator.FieldLevel) bool {
		status := fl.Field().String()
		validStatuses := []string{"active", "inactive", "pending", "suspended"}
		for _, validStatus := range validStatuses {
			if status == validStatus {
				return true
			}
		}
		return false
	})
	
	// Tenant status validation
	validate.RegisterValidation("tenant_status", func(fl validator.FieldLevel) bool {
		status := fl.Field().String()
		validStatuses := []string{"active", "inactive", "trial", "suspended"}
		for _, validStatus := range validStatuses {
			if status == validStatus {
				return true
			}
		}
		return false
	})
}

// Helper validation functions

// IsValidEmail checks if an email is valid
func IsValidEmail(email string) bool {
	v := New()
	return v.ValidateVar(email, "required,email") == nil
}

// IsValidUUID checks if a string is a valid UUID
func IsValidUUID(uuid string) bool {
	v := New()
	return v.ValidateVar(uuid, "required,uuid4") == nil
}

// IsValidPassword checks if a password meets security requirements
func IsValidPassword(password string) bool {
	v := New()
	return v.ValidateVar(password, "required,password") == nil
}

// IsValidUsername checks if a username is valid
func IsValidUsername(username string) bool {
	v := New()
	return v.ValidateVar(username, "required,username") == nil
}

// IsValidSlug checks if a slug is valid
func IsValidSlug(slug string) bool {
	v := New()
	return v.ValidateVar(slug, "required,slug") == nil
}

// Common validation tags constants
const (
	TagRequired    = "required"
	TagEmail       = "email"
	TagUUID        = "uuid4"
	TagPassword    = "password"
	TagUsername    = "username"
	TagSlug        = "slug"
	TagUserRole    = "user_role"
	TagUserStatus  = "user_status"
	TagTenantStatus = "tenant_status"
	TagCSRFToken   = "csrf_token"
	TagMin         = "min"
	TagMax         = "max"
	TagURL         = "url"
)
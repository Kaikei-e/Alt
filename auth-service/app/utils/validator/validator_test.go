package validator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test struct for validation
type TestUser struct {
	Email    string `json:"email" validate:"required,email"`
	Username string `json:"username" validate:"required,username"`
	Password string `json:"password" validate:"required,password"`
	Role     string `json:"role" validate:"required,user_role"`
	Status   string `json:"status" validate:"required,user_status"`
}

type TestTenant struct {
	Slug   string `json:"slug" validate:"required,slug"`
	Status string `json:"status" validate:"required,tenant_status"`
}

func TestNew(t *testing.T) {
	v := New()
	assert.NotNil(t, v)
	assert.NotNil(t, v.validator)
}

func TestValidator_Validate(t *testing.T) {
	v := New()

	tests := []struct {
		name      string
		input     interface{}
		wantError bool
		checkErr  func(*testing.T, error)
	}{
		{
			name: "valid user",
			input: TestUser{
				Email:    "test@example.com",
				Username: "testuser",
				Password: "SecurePass123!",
				Role:     "user",
				Status:   "active",
			},
			wantError: false,
		},
		{
			name: "invalid email",
			input: TestUser{
				Email:    "invalid-email",
				Username: "testuser",
				Password: "SecurePass123!",
				Role:     "user",
				Status:   "active",
			},
			wantError: true,
			checkErr: func(t *testing.T, err error) {
				validationErr, ok := err.(*ValidationError)
				require.True(t, ok)
				assert.Contains(t, validationErr.Errors, "email")
			},
		},
		{
			name: "missing required fields",
			input: TestUser{
				Email: "test@example.com",
				// Missing other required fields
			},
			wantError: true,
			checkErr: func(t *testing.T, err error) {
				validationErr, ok := err.(*ValidationError)
				require.True(t, ok)
				assert.Contains(t, validationErr.Errors, "username")
				assert.Contains(t, validationErr.Errors, "password")
				assert.Contains(t, validationErr.Errors, "role")
				assert.Contains(t, validationErr.Errors, "status")
			},
		},
		{
			name: "invalid password",
			input: TestUser{
				Email:    "test@example.com",
				Username: "testuser",
				Password: "weak",
				Role:     "user",
				Status:   "active",
			},
			wantError: true,
			checkErr: func(t *testing.T, err error) {
				validationErr, ok := err.(*ValidationError)
				require.True(t, ok)
				assert.Contains(t, validationErr.Errors, "password")
			},
		},
		{
			name: "invalid role",
			input: TestUser{
				Email:    "test@example.com",
				Username: "testuser",
				Password: "SecurePass123!",
				Role:     "invalid_role",
				Status:   "active",
			},
			wantError: true,
			checkErr: func(t *testing.T, err error) {
				validationErr, ok := err.(*ValidationError)
				require.True(t, ok)
				assert.Contains(t, validationErr.Errors, "role")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			
			if tt.wantError {
				assert.Error(t, err)
				if tt.checkErr != nil {
					tt.checkErr(t, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateVar(t *testing.T) {
	v := New()

	tests := []struct {
		name      string
		field     interface{}
		tag       string
		wantError bool
	}{
		{
			name:      "valid email",
			field:     "test@example.com",
			tag:       "required,email",
			wantError: false,
		},
		{
			name:      "invalid email",
			field:     "invalid-email",
			tag:       "required,email",
			wantError: true,
		},
		{
			name:      "empty required field",
			field:     "",
			tag:       "required",
			wantError: true,
		},
		{
			name:      "valid UUID",
			field:     "550e8400-e29b-41d4-a716-446655440000",
			tag:       "required,uuid4",
			wantError: false,
		},
		{
			name:      "invalid UUID",
			field:     "not-a-uuid",
			tag:       "required,uuid4",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.field, tt.tag)
			
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		valid bool
	}{
		{"valid email", "test@example.com", true},
		{"valid email with subdomain", "user@mail.example.com", true},
		{"invalid email - no @", "testexample.com", false},
		{"invalid email - no domain", "test@", false},
		{"empty email", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidEmail(tt.email)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		name  string
		uuid  string
		valid bool
	}{
		{"valid UUID v4", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid UUID v4 uppercase", "550e8400-e29b-41d4-a716-446655440000", true},
		{"invalid UUID - wrong format", "550e8400-e29b-41d4-a716", false},
		{"invalid UUID - not hex", "550e8400-e29b-41d4-a716-44665544000g", false},
		{"empty UUID", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidUUID(tt.uuid)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestIsValidPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{"valid password", "SecurePass123!", true},
		{"valid password with symbols", "MyP@ssw0rd#123", true},
		{"too short", "Sec1!", false},
		{"no uppercase", "securepass123!", false},
		{"no lowercase", "SECUREPASS123!", false},
		{"no number", "SecurePass!", false},
		{"no special char", "SecurePass123", false},
		{"empty password", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidPassword(tt.password)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestIsValidUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		valid    bool
	}{
		{"valid username", "testuser", true},
		{"valid username with numbers", "testuser123", true},
		{"valid username with dots", "test.user", true},
		{"valid username with hyphens", "test-user", true},
		{"valid username with underscores", "test_user", true},
		{"too short", "ab", false},
		{"too long", "this_username_is_way_too_long_for_our_system", false},
		{"invalid characters", "test@user", false},
		{"empty username", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidUsername(tt.username)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestIsValidSlug(t *testing.T) {
	tests := []struct {
		name  string
		slug  string
		valid bool
	}{
		{"valid slug", "test-slug", true},
		{"valid slug with numbers", "test123", true},
		{"valid slug lowercase", "testslug", true},
		{"invalid uppercase", "Test-Slug", false},
		{"invalid underscore", "test_slug", false},
		{"invalid space", "test slug", false},
		{"too short", "a", false},
		{"too long", "this-slug-is-way-too-long-for-our-system-requirements", false},
		{"empty slug", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidSlug(tt.slug)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestCustomValidators(t *testing.T) {
	v := New()

	// Test user role validation
	t.Run("user_role validation", func(t *testing.T) {
		validRoles := []string{"admin", "user", "readonly"}
		invalidRoles := []string{"superuser", "guest", "invalid"}

		for _, role := range validRoles {
			err := v.ValidateVar(role, "user_role")
			assert.NoError(t, err, "Role %s should be valid", role)
		}

		for _, role := range invalidRoles {
			err := v.ValidateVar(role, "user_role")
			assert.Error(t, err, "Role %s should be invalid", role)
		}
	})

	// Test user status validation
	t.Run("user_status validation", func(t *testing.T) {
		validStatuses := []string{"active", "inactive", "pending", "suspended"}
		invalidStatuses := []string{"deleted", "unknown", "invalid"}

		for _, status := range validStatuses {
			err := v.ValidateVar(status, "user_status")
			assert.NoError(t, err, "Status %s should be valid", status)
		}

		for _, status := range invalidStatuses {
			err := v.ValidateVar(status, "user_status")
			assert.Error(t, err, "Status %s should be invalid", status)
		}
	})

	// Test tenant status validation
	t.Run("tenant_status validation", func(t *testing.T) {
		validStatuses := []string{"active", "inactive", "trial", "suspended"}
		invalidStatuses := []string{"deleted", "unknown", "invalid"}

		for _, status := range validStatuses {
			err := v.ValidateVar(status, "tenant_status")
			assert.NoError(t, err, "Status %s should be valid", status)
		}

		for _, status := range invalidStatuses {
			err := v.ValidateVar(status, "tenant_status")
			assert.Error(t, err, "Status %s should be invalid", status)
		}
	})
}

func TestValidationError(t *testing.T) {
	v := New()

	// Create a validation error
	user := TestUser{
		Email: "invalid-email",
		// Missing other required fields
	}

	err := v.Validate(user)
	require.Error(t, err)

	validationErr, ok := err.(*ValidationError)
	require.True(t, ok)

	// Test Error() method
	errorMsg := validationErr.Error()
	assert.Contains(t, errorMsg, "validation failed")
	assert.Contains(t, errorMsg, "email")

	// Test error structure
	assert.Contains(t, validationErr.Errors, "email")
	assert.Contains(t, validationErr.Errors, "username")
	assert.Contains(t, validationErr.Errors, "password")
	assert.Contains(t, validationErr.Errors, "role")
	assert.Contains(t, validationErr.Errors, "status")
}

func TestTenantValidation(t *testing.T) {
	v := New()

	tests := []struct {
		name      string
		tenant    TestTenant
		wantError bool
		checkErr  func(*testing.T, error)
	}{
		{
			name: "valid tenant",
			tenant: TestTenant{
				Slug:   "valid-tenant",
				Status: "active",
			},
			wantError: false,
		},
		{
			name: "invalid slug",
			tenant: TestTenant{
				Slug:   "Invalid_Slug",
				Status: "active",
			},
			wantError: true,
			checkErr: func(t *testing.T, err error) {
				validationErr, ok := err.(*ValidationError)
				require.True(t, ok)
				assert.Contains(t, validationErr.Errors, "slug")
			},
		},
		{
			name: "invalid status",
			tenant: TestTenant{
				Slug:   "valid-tenant",
				Status: "invalid",
			},
			wantError: true,
			checkErr: func(t *testing.T, err error) {
				validationErr, ok := err.(*ValidationError)
				require.True(t, ok)
				assert.Contains(t, validationErr.Errors, "status")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.tenant)
			
			if tt.wantError {
				assert.Error(t, err)
				if tt.checkErr != nil {
					tt.checkErr(t, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
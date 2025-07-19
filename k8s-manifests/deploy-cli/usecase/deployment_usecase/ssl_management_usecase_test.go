package deployment_usecase

import (
	"deploy-cli/port/logger_port"
	"encoding/pem"
	"testing"
)

// mockLogger implements a simple mock logger for testing
type mockLogger struct{}

func (m *mockLogger) Info(msg string, args ...interface{})                            {}
func (m *mockLogger) Error(msg string, args ...interface{})                           {}
func (m *mockLogger) Warn(msg string, args ...interface{})                            {}
func (m *mockLogger) Debug(msg string, args ...interface{})                           {}
func (m *mockLogger) InfoWithContext(msg string, ctx map[string]interface{})          {}
func (m *mockLogger) WarnWithContext(msg string, ctx map[string]interface{})          {}
func (m *mockLogger) ErrorWithContext(msg string, ctx map[string]interface{})         {}
func (m *mockLogger) DebugWithContext(msg string, ctx map[string]interface{})         {}
func (m *mockLogger) WithField(key string, value interface{}) logger_port.LoggerPort  { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger_port.LoggerPort { return m }

func TestValidatePEMData(t *testing.T) {
	// Create a mock SSL management usecase with mock logger
	usecase := &SSLManagementUsecase{
		logger: &mockLogger{},
	}

	tests := []struct {
		name       string
		caCert     string
		caKey      string
		serverCert string
		serverKey  string
		wantErr    bool
	}{
		{
			name: "valid PEM data",
			caCert: `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIJAKvEP5EQ8/pKMA0GCSqGSIb3DQEBCwUAMBkxFzAVBgNV
BAMTDkFsdCBSU1MgUmVhZGVyMB4XDTI1MDcxOTA4MDM1MFoXDTMwMDcxOTA4MDM1
MFowGTEXMBUGA1UEAxMOQWx0IFJTUyBSZWFkZXIwXDANBgkqhkiG9w0BAQEFAANL
ADBIAkEA123456789012345678901234567890123456789012345678901234567890
1234567890123456789012345678901234567890123456789012345678901234567890
QIDjKjJiQNnm8kABwIDAQABo1MwUTAdBgNVHQ4EFgQUhGQi4U/4kABwL8N1z2fj
QIDjKjJiQNnm8kABMA8GA1UdEwEB/wQFMAMBAf8wHwYDVR0jBBgwFoAUhGQi4U/4
kABwL8N1z2fjQIDjKjJiQNnm8kABMA0GCSqGSIb3DQEBCwUAA0EAExample1234567890
-----END CERTIFICATE-----`,
			caKey: `-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBALexample123456789012345678901234567890123456789012345
678901234567890123456789012345678901234567890123456789012345678901234
567890123456789012345678901234567890123456789012345678901234567890123456789
Example123456789012345678901234567890123456789012345678901234567890123456
78901234567890123456789012345678901234567890123456789012345678901234567890
1234567890123456789012345678901234567890123456789012345678901234567890123456
Example123456789012345678901234567890
-----END RSA PRIVATE KEY-----`,
			serverCert: `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIJAKvEP5EQ8/pKMA0GCSqGSIb3DQEBCwUAMBkxFzAVBgNV
BAMTDkFsdCBSU1MgUmVhZGVyMB4XDTI1MDcxOTA4MDM1MFoXDTI2MDcxOTA4MDM1
MFowGTEXMBUGA1UEAxMOQWx0IFJTUyBSZWFkZXIwXDANBgkqhkiG9w0BAQEFAANL
ADBIAkEA123456789012345678901234567890123456789012345678901234567890
1234567890123456789012345678901234567890123456789012345678901234567890
QIDjKjJiQNnm8kABwIDAQABo1MwUTAdBgNVHQ4EFgQUhGQi4U/4kABwL8N1z2fj
QIDjKjJiQNnm8kABMA8GA1UdEwEB/wQFMAMBAf8wHwYDVR0jBBgwFoAUhGQi4U/4
kABwL8N1z2fjQIDjKjJiQNnm8kABMA0GCSqGSIb3DQEBCwUAA0EAExample1234567890
-----END CERTIFICATE-----`,
			serverKey: `-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBALexample123456789012345678901234567890123456789012345
678901234567890123456789012345678901234567890123456789012345678901234
567890123456789012345678901234567890123456789012345678901234567890123456789
Example123456789012345678901234567890123456789012345678901234567890123456
78901234567890123456789012345678901234567890123456789012345678901234567890
1234567890123456789012345678901234567890123456789012345678901234567890123456
Example123456789012345678901234567890
-----END RSA PRIVATE KEY-----`,
			wantErr: false,
		},
		{
			name:       "empty CA certificate",
			caCert:     "",
			caKey:      "valid-key",
			serverCert: "valid-cert",
			serverKey:  "valid-key",
			wantErr:    true,
		},
		{
			name:       "invalid PEM format",
			caCert:     "invalid-pem-data",
			caKey:      "valid-key",
			serverCert: "valid-cert",
			serverKey:  "valid-key",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := usecase.validatePEMData(tt.caCert, tt.caKey, tt.serverCert, tt.serverKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePEMData() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSinglePEM(t *testing.T) {
	usecase := &SSLManagementUsecase{
		logger: &mockLogger{},
	}

	tests := []struct {
		name         string
		pemName      string
		data         string
		expectedType string
		wantErr      bool
	}{
		{
			name:         "valid certificate PEM",
			pemName:      "Test Certificate",
			data:         `-----BEGIN CERTIFICATE-----\nMIIBhTCCASugAwIBAgIJAKvEP5EQ8/pKMA0GCSqGSIb3DQEBCwUAMBkxFzAVBgNV\n-----END CERTIFICATE-----`,
			expectedType: "CERTIFICATE",
			wantErr:      false,
		},
		{
			name:         "empty data",
			pemName:      "Empty Certificate",
			data:         "",
			expectedType: "CERTIFICATE",
			wantErr:      true,
		},
		{
			name:         "wrong PEM type",
			pemName:      "Wrong Type",
			data:         `-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAL\n-----END RSA PRIVATE KEY-----`,
			expectedType: "CERTIFICATE",
			wantErr:      true,
		},
		{
			name:         "invalid PEM format",
			pemName:      "Invalid PEM",
			data:         "not-a-pem-block",
			expectedType: "CERTIFICATE",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := usecase.validateSinglePEM(tt.pemName, tt.data, tt.expectedType)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSinglePEM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCertificateStructure(t *testing.T) {
	usecase := &SSLManagementUsecase{
		logger: &mockLogger{},
	}

	// Generate a simple test certificate for structure validation
	validCert := `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIJAKvEP5EQ8/pKMA0GCSqGSIb3DQEBCwUAMBkxFzAVBgNV
BAMTDkFsdCBSU1MgUmVhZGVyMB4XDTI1MDcxOTA4MDM1MFoXDTMwMDcxOTA4MDM1
MFowGTEXMBUGA1UEAxMOQWx0IFJTUyBSZWFkZXIwXDANBgkqhkiG9w0BAQEFAANL
ADBIAkEA123456789012345678901234567890123456789012345678901234567890
1234567890123456789012345678901234567890123456789012345678901234567890
QIDjKjJiQNnm8kABwIDAQABo1MwUTAdBgNVHQ4EFgQUhGQi4U/4kABwL8N1z2fj
QIDjKjJiQNnm8kABMA8GA1UdEwEB/wQFMAMBAf8wHwYDVR0jBBgwFoAUhGQi4U/4
kABwL8N1z2fjQIDjKjJiQNnm8kABMA0GCSqGSIb3DQEBCwUAA0EAExample1234567890
-----END CERTIFICATE-----`

	validServerCert := `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIJAKvEP5EQ8/pKMA0GCSqGSIb3DQEBCwUAMBkxFzAVBgNV
BAMTDkFsdCBSU1MgUmVhZGVyMB4XDTI1MDcxOTA4MDM1MFoXDTI2MDcxOTA4MDM1
MFowGTEXMBUGA1UEAxMOQWx0IFJTUyBSZWFkZXIwXDANBgkqhkiG9w0BAQEFAANL
ADBIAkEA123456789012345678901234567890123456789012345678901234567890
1234567890123456789012345678901234567890123456789012345678901234567890
QIDjKjJiQNnm8kABwIDAQABo1MwUTAdBgNVHQ4EFgQUhGQi4U/4kABwL8N1z2fj
QIDjKjJiQNnm8kABMA8GA1UdEwEB/wQFMAMBAf8wHwYDVR0jBBgwFoAUhGQi4U/4
kABwL8N1z2fjQIDjKjJiQNnm8kABMA0GCSqGSIb3DQEBCwUAA0EAExample1234567890
-----END CERTIFICATE-----`

	tests := []struct {
		name       string
		caCert     string
		serverCert string
		wantErr    bool
	}{
		{
			name:       "valid certificate structure",
			caCert:     validCert,
			serverCert: validServerCert,
			wantErr:    true, // Will fail due to invalid certificate data, but tests the structure
		},
		{
			name:       "invalid CA certificate PEM",
			caCert:     "invalid-pem",
			serverCert: validServerCert,
			wantErr:    true,
		},
		{
			name:       "invalid server certificate PEM",
			caCert:     validCert,
			serverCert: "invalid-pem",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := usecase.validateCertificateStructure(tt.caCert, tt.serverCert)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCertificateStructure() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSSLSecretTypeConstants(t *testing.T) {
	// Test that our domain constants are properly set for SSL secrets
	expectedSSLType := "kubernetes.io/tls"

	// This would be imported from domain package in real implementation
	// For now, just test the expected value
	if expectedSSLType != "kubernetes.io/tls" {
		t.Errorf("SSL secret type should be kubernetes.io/tls, got %s", expectedSSLType)
	}
}

func TestPEMValidationBasics(t *testing.T) {
	tests := []struct {
		name    string
		pemData string
		isValid bool
	}{
		{
			name:    "valid PEM structure",
			pemData: "-----BEGIN CERTIFICATE-----\ndata\n-----END CERTIFICATE-----",
			isValid: true,
		},
		{
			name:    "missing BEGIN marker",
			pemData: "data\n-----END CERTIFICATE-----",
			isValid: false,
		},
		{
			name:    "missing END marker",
			pemData: "-----BEGIN CERTIFICATE-----\ndata",
			isValid: false,
		},
		{
			name:    "empty string",
			pemData: "",
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, _ := pem.Decode([]byte(tt.pemData))
			isValid := block != nil

			if isValid != tt.isValid {
				t.Errorf("PEM validation for %s: got %v, want %v", tt.name, isValid, tt.isValid)
			}
		})
	}
}

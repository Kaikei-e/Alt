package utils

import (
	"net/url"
	"testing"
)

func TestValidateURL_BlockedPorts(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{"blocked_port_22", "http://example.com:22", true},
		{"allowed_port_8080", "http://example.com:8080", false},
		{"no_port", "http://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatalf("failed to parse URL: %v", err)
			}
			err = ValidateURL(u)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

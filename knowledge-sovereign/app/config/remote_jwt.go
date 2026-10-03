package config

import (
	"os"

	"knowledge-sovereign/driver/authhub"
	"knowledge-sovereign/gateway/authgw"
	"knowledge-sovereign/port/authport"
)

// RemoteUserJWTVerifier aliases authhub.RemoteUserJWTVerifier for backwards compatibility.
type RemoteUserJWTVerifier = authhub.RemoteUserJWTVerifier

// RemoteVerifierConfig aliases authhub.Config for backwards compatibility.
type RemoteVerifierConfig = authhub.Config

// NewRemoteUserJWTVerifier aliases authhub.NewRemoteUserJWTVerifier for backwards compatibility.
var NewRemoteUserJWTVerifier = authhub.NewRemoteUserJWTVerifier

// LoadRemoteUserJWTVerifier parses environment variables and constructs a driver RemoteUserJWTVerifier.
func LoadRemoteUserJWTVerifier(overrideURL string) (authport.TokenVerifier, error) {
	endpoint := overrideURL
	if endpoint == "" {
		endpoint = os.Getenv("USER_JWT_INTROSPECTION_URL")
	}
	if endpoint == "" {
		endpoint = "https://auth-hub:9443/internal/token/introspect"
	}

	certPath := os.Getenv("MTLS_CERT_FILE")
	if certPath == "" {
		certPath = os.Getenv("CERT_PATH")
	}
	if certPath == "" {
		certPath = os.Getenv("USER_JWT_CERT_FILE")
	}
	if certPath == "" {
		certPath = "/certs/svc-cert.pem"
	}

	keyPath := os.Getenv("MTLS_KEY_FILE")
	if keyPath == "" {
		keyPath = os.Getenv("KEY_PATH")
	}
	if keyPath == "" {
		keyPath = os.Getenv("USER_JWT_KEY_FILE")
	}
	if keyPath == "" {
		keyPath = "/certs/svc-key.pem"
	}

	caPath := os.Getenv("MTLS_CA_FILE")
	if caPath == "" {
		caPath = os.Getenv("CA_PATH")
	}
	if caPath == "" {
		caPath = os.Getenv("USER_JWT_CA_FILE")
	}
	if caPath == "" {
		caPath = "/trust/ca-bundle.pem"
	}

	driver, err := authhub.NewRemoteUserJWTVerifier(authhub.Config{
		URL:      endpoint,
		CertPath: certPath,
		KeyPath:  keyPath,
		CAPath:   caPath,
	})
	if err != nil {
		return nil, err
	}
	return authgw.NewGateway(driver), nil
}

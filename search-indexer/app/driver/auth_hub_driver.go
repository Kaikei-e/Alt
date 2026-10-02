package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"search-indexer/tlsutil"
)

type TokenIntrospectionResponse struct {
	Active   bool   `json:"active"`
	Sub      string `json:"sub"`
	TenantID string `json:"tenant_id"`
	Exp      int64  `json:"exp"`
}

type AuthHubDriver interface {
	IntrospectToken(ctx context.Context, token string) (*TokenIntrospectionResponse, error)
}

type authHubDriver struct {
	client *http.Client
	url    string
}

// NewAuthHubDriver creates a new AuthHubDriver with rotating verified mTLS,
// enforced HTTPS, and disabled redirects.
func NewAuthHubDriver(endpointURL string) (AuthHubDriver, error) {
	if endpointURL == "" {
		return nil, fmt.Errorf("authhub url is required")
	}

	parsed, err := url.Parse(endpointURL)
	if err != nil {
		return nil, fmt.Errorf("invalid authhub url: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return nil, fmt.Errorf("authhub url must use HTTPS scheme, got: %s", parsed.Scheme)
	}

	certFile := os.Getenv("MTLS_CERT_FILE")
	if certFile == "" {
		certFile = "/certs/svc-cert.pem"
	}
	keyFile := os.Getenv("MTLS_KEY_FILE")
	if keyFile == "" {
		keyFile = "/certs/svc-key.pem"
	}
	caFile := os.Getenv("MTLS_CA_FILE")
	if caFile == "" {
		caFile = "/trust/ca-bundle.pem"
	}

	tlsConfig, err := tlsutil.LoadClientConfig(certFile, keyFile, caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load rotating client tls config: %w", err)
	}

	serverName := os.Getenv("AUTH_HUB_MTLS_SERVER_NAME")
	if serverName == "" {
		serverName = "auth-hub"
	}
	tlsConfig.ServerName = serverName

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Disable redirect to prevent secret/JWT leak
			return http.ErrUseLastResponse
		},
	}

	return &authHubDriver{
		client: client,
		url:    endpointURL,
	}, nil
}

// NewAuthHubDriverWithClient creates an AuthHubDriver with an explicit *http.Client (for tests/custom client).
func NewAuthHubDriverWithClient(endpointURL string, client *http.Client) (AuthHubDriver, error) {
	if endpointURL == "" {
		return nil, fmt.Errorf("authhub url is required")
	}
	parsed, err := url.Parse(endpointURL)
	if err != nil {
		return nil, fmt.Errorf("invalid authhub url: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return nil, fmt.Errorf("authhub url must use HTTPS scheme, got: %s", parsed.Scheme)
	}

	return &authHubDriver{
		client: client,
		url:    endpointURL,
	}, nil
}

func (d *authHubDriver) IntrospectToken(ctx context.Context, token string) (*TokenIntrospectionResponse, error) {
	reqBody, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspect request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authhub returned status %d", resp.StatusCode)
	}

	const maxBytes = 1024 * 1024 // 1MB bound
	limitedBody := io.LimitReader(resp.Body, maxBytes+1)
	bodyBytes, err := io.ReadAll(limitedBody)
	if err != nil {
		return nil, fmt.Errorf("read authhub response: %w", err)
	}
	if len(bodyBytes) > maxBytes {
		return nil, fmt.Errorf("authhub response exceeds 1MB limit")
	}

	dec := json.NewDecoder(bytes.NewReader(bodyBytes))
	var result TokenIntrospectionResponse
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode authhub response: %w", err)
	}

	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("authhub response contains trailing garbage or multiple JSON values")
	}

	if result.Active {
		if _, err := uuid.Parse(result.TenantID); err != nil {
			return nil, fmt.Errorf("invalid tenant_id uuid: %w", err)
		}
		if _, err := uuid.Parse(result.Sub); err != nil {
			return nil, fmt.Errorf("invalid sub uuid: %w", err)
		}
		if result.Exp <= 0 || time.Now().Unix() > result.Exp {
			return nil, fmt.Errorf("token expired or invalid exp")
		}
	}

	return &result, nil
}

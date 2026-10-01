package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alt-butterfly-facade/internal/tlsutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createConnectEnvelope builds a 5-byte prefixed Connect streaming envelope:
// 1 byte flags (0x00 data, 0x02 end-of-stream) + 4 bytes big-endian length + payload.
func createConnectEnvelope(flag byte, payload []byte) []byte {
	buf := make([]byte, 5+len(payload))
	buf[0] = flag
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(payload)))
	copy(buf[5:], payload)
	return buf
}

// TestTTSProxy_StreamingPassThrough pins requirement 4(a):
// A fake upstream writes 3 enveloped frames with pauses; the test reads the BFF
// response incrementally and observes the first frame before the upstream finished (flush),
// and the bytes arrive unmodified.
func TestTTSProxy_StreamingPassThrough(t *testing.T) {
	upstreamDone := make(chan struct{})

	frame1Payload := []byte(`{"chunk":1,"audio_wav":"Y2h1bmsx"}`)
	frame2Payload := []byte(`{"chunk":2,"audio_wav":"Y2h1bmsy"}`)
	frame3Payload := []byte(`{}`) // end-of-stream trailer frame

	frame1 := createConnectEnvelope(0x00, frame1Payload)
	frame2 := createConnectEnvelope(0x00, frame2Payload)
	frame3 := createConnectEnvelope(0x02, frame3Payload)

	allExpectedBytes := append(append(append([]byte{}, frame1...), frame2...), frame3...)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.tts.v1.TTSService/SynthesizeStream", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.NotEmpty(t, r.Header.Get("X-Alt-Backend-Token"), "token must be forwarded to upstream")

		w.Header().Set("Content-Type", "application/connect+json")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "upstream must support flushing")

		// Write frame 1 and flush immediately
		_, err := w.Write(frame1)
		require.NoError(t, err)
		flusher.Flush()

		// Pause between frame 1 and frame 2 to allow client to read incrementally
		select {
		case <-time.After(150 * time.Millisecond):
		case <-r.Context().Done():
			return
		}

		// Write frame 2 and flush
		_, err = w.Write(frame2)
		require.NoError(t, err)
		flusher.Flush()

		select {
		case <-time.After(100 * time.Millisecond):
		case <-r.Context().Done():
			return
		}

		// Write frame 3 (end-of-stream) and flush
		_, err = w.Write(frame3)
		require.NoError(t, err)
		flusher.Flush()
		close(upstreamDone)
	}))
	defer upstream.Close()

	secret := []byte("test-secret-at-least-32-chars-long!")
	cfg := Config{
		BackendURL:       "http://127.0.0.1:1",
		TTSProxy:         "enabled",
		TTSConnectURL:    upstream.URL,
		TTSTransport:     http.DefaultTransport,
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   50 * time.Millisecond,
		StreamingTimeout: 5 * time.Minute,
	}

	bffSrv := httptest.NewServer(NewServerWithTransport(cfg, nil, http.DefaultTransport))
	defer bffSrv.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
		strings.NewReader(`{"text":"こんにちは"}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, secret))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/connect+json")

	// Read frame 1 header (5 bytes)
	f1Header := make([]byte, 5)
	_, err = io.ReadFull(resp.Body, f1Header)
	require.NoError(t, err)
	assert.Equal(t, byte(0x00), f1Header[0], "frame 1 flag must be 0x00 (data)")
	f1Len := binary.BigEndian.Uint32(f1Header[1:5])
	f1Body := make([]byte, f1Len)
	_, err = io.ReadFull(resp.Body, f1Body)
	require.NoError(t, err)
	assert.Equal(t, frame1Payload, f1Body)

	// Flush verification: frame 1 must be received BEFORE upstream finishes sending remaining frames
	select {
	case <-upstreamDone:
		t.Fatal("frame 1 should have been received before upstream finished (response was buffered, not flushed)")
	default:
		// Upstream still paused/sending: incremental flush verified
	}

	// Read remaining bytes and verify full stream arrives unmodified
	remainingBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	fullReceived := append(append(f1Header, f1Body...), remainingBytes...)
	assert.Equal(t, allExpectedBytes, fullReceived, "streaming bytes must arrive completely unmodified")
}

// TestTTSProxy_AuthRequired pins requirement 4(b):
// Missing or invalid token yields 401 Unauthorized, and the upstream is not called.
func TestTTSProxy_AuthRequired(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		t.Fatalf("upstream must not be called when authentication fails")
	}))
	defer upstream.Close()

	secret := []byte("test-secret-at-least-32-chars-long!")
	cfg := Config{
		BackendURL:       "http://127.0.0.1:1",
		TTSProxy:         "enabled",
		TTSConnectURL:    upstream.URL,
		TTSTransport:     http.DefaultTransport,
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	bffSrv := httptest.NewServer(NewServerWithTransport(cfg, nil, http.DefaultTransport))
	defer bffSrv.Close()

	t.Run("missing token", func(t *testing.T) {
		upstreamCalled = false
		req, err := http.NewRequest(
			http.MethodPost,
			bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
			strings.NewReader(`{"text":"test"}`),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/connect+json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.False(t, upstreamCalled, "upstream must not be called for missing token")
	})

	t.Run("invalid token", func(t *testing.T) {
		upstreamCalled = false
		req, err := http.NewRequest(
			http.MethodPost,
			bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
			strings.NewReader(`{"text":"test"}`),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/connect+json")
		req.Header.Set("X-Alt-Backend-Token", "invalid-token-string")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.False(t, upstreamCalled, "upstream must not be called for invalid token")
	})
}

// TestTTSProxy_DisabledMode pins requirement 4(c):
// In disabled mode, every request answers a Connect error failed_precondition with message
// "tts is disabled" (HTTP 200 with application/connect+json and an end-of-stream frame carrying the error),
// and never dials upstream.
func TestTTSProxy_DisabledMode(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		t.Fatalf("upstream must never be called in disabled mode")
	}))
	defer upstream.Close()

	secret := []byte("test-secret-at-least-32-chars-long!")
	cfg := Config{
		BackendURL:       "http://127.0.0.1:1",
		TTSProxy:         "disabled",
		TTSConnectURL:    upstream.URL, // even if URL is configured, disabled mode must not dial it
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	bffSrv := httptest.NewServer(NewServerWithTransport(cfg, nil, http.DefaultTransport))
	defer bffSrv.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
		strings.NewReader(`{"text":"test"}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, secret))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Connect streaming errors return HTTP 200 with application/connect+json
	// and the error is carried inside the end-of-stream frame.
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/connect+json")
	assert.False(t, upstreamCalled, "upstream must never be called in disabled mode")

	// Read the end-of-stream frame: 5-byte prefix + JSON payload
	eosHeader := make([]byte, 5)
	_, err = io.ReadFull(resp.Body, eosHeader)
	require.NoError(t, err)
	assert.Equal(t, byte(0x02), eosHeader[0], "flag must be 0x02 (end-of-stream)")

	eosLen := binary.BigEndian.Uint32(eosHeader[1:5])
	eosPayload := make([]byte, eosLen)
	_, err = io.ReadFull(resp.Body, eosPayload)
	require.NoError(t, err)

	var eosMessage struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(eosPayload, &eosMessage), "payload must be valid JSON: %s", string(eosPayload))
	assert.Equal(t, "failed_precondition", eosMessage.Error.Code)
	assert.Equal(t, "tts is disabled", eosMessage.Error.Message)

	// Compare exact expected bytes (N4)
	expectedPayload := []byte(`{"error":{"code":"failed_precondition","message":"tts is disabled"}}`)
	expectedEnvelope := make([]byte, 5+len(expectedPayload))
	expectedEnvelope[0] = 0x02
	binary.BigEndian.PutUint32(expectedEnvelope[1:5], uint32(len(expectedPayload)))
	copy(expectedEnvelope[5:], expectedPayload)
	fullReceived := append(append([]byte{}, eosHeader...), eosPayload...)
	assert.Equal(t, expectedEnvelope, fullReceived, "disabled response must match exact expected frame bytes")
}

// TestTTSProxy_UnreachableUpstream pins requirement 4(f) and S5:
// When enabled and upstream is unreachable, the client receives exactly HTTP 502 Bad Gateway
// with Content-Type not application/connect+*.
func TestTTSProxy_UnreachableUpstream(t *testing.T) {
	// Create and immediately close a listener to get an unreachable port
	closedUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := closedUpstream.URL
	closedUpstream.Close()

	secret := []byte("test-secret-at-least-32-chars-long!")
	cfg := Config{
		BackendURL:       "http://127.0.0.1:1",
		TTSProxy:         "enabled",
		TTSConnectURL:    unreachableURL,
		TTSTransport:     http.DefaultTransport,
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	bffSrv := httptest.NewServer(NewServerWithTransport(cfg, nil, http.DefaultTransport))
	defer bffSrv.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
		strings.NewReader(`{"text":"test"}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, secret))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// S5: assert exactly StatusBadGateway, assert Content-Type is not application/connect+*
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	assert.False(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/connect+"),
		"content type must not be application/connect+*")
}

// TestTTSProxy_EnabledNilTransportPanics proves that when TTS_PROXY=enabled
// but TTSTransport is nil, NewServer panics fail-fast (rule 8).
func TestTTSProxy_EnabledNilTransportPanics(t *testing.T) {
	cfg := Config{
		BackendURL:       "http://127.0.0.1:1",
		TTSProxy:         "enabled",
		TTSConnectURL:    "https://127.0.0.1:9443",
		TTSTransport:     nil,
		Secret:           []byte("test-secret-at-least-32-chars-long!"),
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}
	assert.PanicsWithValue(t, "server: TTSTransport is required when TTS_PROXY=enabled", func() {
		NewServer(cfg, nil)
	})
}

func generateThrowawayMTLSCerts(t *testing.T) (caPEM []byte, serverCert tls.Certificate, clientCertPEM, clientKeyPEM []byte) {
	t.Helper()

	// 1. Generate CA
	caPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Throwaway Test CA"},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caDer, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDer})
	require.NotEmpty(t, caPEM)

	// 2. Generate Server Cert
	serverPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-1 * time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	serverDer, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDer})
	serverKeyDer, err := x509.MarshalECPrivateKey(serverPrivKey)
	require.NoError(t, err)
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDer})

	serverCert, err = tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	require.NoError(t, err)

	// 3. Generate Client Cert
	clientPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "alt-butterfly-facade"},
		NotBefore:    now.Add(-1 * time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientDer, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	clientCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDer})
	clientKeyDer, err := x509.MarshalECPrivateKey(clientPrivKey)
	require.NoError(t, err)
	clientKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDer})

	return caPEM, serverCert, clientCertPEM, clientKeyPEM
}

// TestTTSProxy_MTLSTransport_StreamsThroughUpstreamRequiringClientCert pins B1/B2:
// Proves that when TTS_PROXY=enabled with dedicated HTTP/1.1 mTLS transport,
// the BFF successfully streams through a TLS upstream that requires a client certificate.
func TestTTSProxy_MTLSTransport_StreamsThroughUpstreamRequiringClientCert(t *testing.T) {
	caPEM, serverCert, clientCertPEM, clientKeyPEM := generateThrowawayMTLSCerts(t)

	// Save certs to temp files for tlsutil.LoadClientConfig
	tmpDir := t.TempDir()
	caFile := filepath.Join(tmpDir, "ca.pem")
	certFile := filepath.Join(tmpDir, "client.pem")
	keyFile := filepath.Join(tmpDir, "client.key")

	require.NoError(t, os.WriteFile(caFile, caPEM, 0600))
	require.NoError(t, os.WriteFile(certFile, clientCertPEM, 0600))
	require.NoError(t, os.WriteFile(keyFile, clientKeyPEM, 0600))

	caPool := x509.NewCertPool()
	require.True(t, caPool.AppendCertsFromPEM(caPEM))

	frame1Payload := []byte(`{"chunk":1,"audio_wav":"Y2h1bmsx"}`)
	frame2Payload := []byte(`{}`) // end-of-stream
	frame1 := createConnectEnvelope(0x00, frame1Payload)
	frame2 := createConnectEnvelope(0x02, frame2Payload)
	expectedBytes := append(append([]byte{}, frame1...), frame2...)

	upstreamCalled := false
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		require.NotNil(t, r.TLS, "must be TLS request")
		require.NotEmpty(t, r.TLS.PeerCertificates, "upstream requires client cert")
		assert.Equal(t, "alt-butterfly-facade", r.TLS.PeerCertificates[0].Subject.CommonName)
		assert.Equal(t, "/alt.tts.v1.TTSService/SynthesizeStream", r.URL.Path)

		w.Header().Set("Content-Type", "application/connect+json")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		_, err := w.Write(frame1)
		require.NoError(t, err)
		flusher.Flush()

		_, err = w.Write(frame2)
		require.NoError(t, err)
		flusher.Flush()
	}))

	upstream.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	upstream.StartTLS()
	defer upstream.Close()

	// 1. Verify that a client without a client certificate is rejected by the upstream TLS handshake
	noCertClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caPool,
			},
		},
	}
	_, err := noCertClient.Get(upstream.URL)
	require.Error(t, err, "direct dial to upstream without client cert must fail TLS handshake")

	// 2. Build dedicated HTTP/1.1 TTS transport using tlsutil.LoadClientConfig (same as main.go)
	tlsClientCfg, err := tlsutil.LoadClientConfig(certFile, keyFile, caFile)
	require.NoError(t, err)

	ttsTransport := &http.Transport{
		TLSClientConfig: tlsClientCfg,
	}

	secret := []byte("test-secret-at-least-32-chars-long!")
	cfg := Config{
		BackendURL:       "http://127.0.0.1:1",
		TTSProxy:         "enabled",
		TTSConnectURL:    upstream.URL,
		TTSTransport:     ttsTransport,
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	bffSrv := httptest.NewServer(NewServer(cfg, nil))
	defer bffSrv.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
		strings.NewReader(`{"text":"hello"}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, secret))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/connect+json")
	assert.True(t, upstreamCalled, "upstream must have been called")

	receivedBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, expectedBytes, receivedBytes, "streamed bytes through mTLS upstream must match exactly")
}

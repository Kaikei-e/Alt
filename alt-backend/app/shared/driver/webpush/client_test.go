package webpush

import (
	"context"
	"crypto/ecdh"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, httpClient *http.Client) *Client {
	t.Helper()
	client, err := NewClient(newTestSigner(t, "you@example.com"), httpClient)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func testHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func TestNewClient_Validation(t *testing.T) {
	signer := newTestSigner(t, "you@example.com")

	tests := []struct {
		name       string
		signer     *VAPIDSigner
		httpClient *http.Client
		wantErr    bool
	}{
		{name: "nil http client gets a default with a timeout", signer: signer},
		{name: "explicit timeout accepted", signer: signer, httpClient: testHTTPClient()},
		// A zero-value http.Client waits forever, which turns one wedged push
		// service into a stuck worker.
		{name: "http client without a timeout rejected", signer: signer, httpClient: &http.Client{}, wantErr: true},
		{name: "nil signer rejected", signer: nil, httpClient: testHTTPClient(), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.signer, tt.httpClient)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if client.httpClient.Timeout <= 0 {
				t.Error("client must always carry an explicit timeout")
			}
		})
	}
}

func TestClient_Send_RequestShape(t *testing.T) {
	var (
		gotMethod  string
		gotPath    string
		gotHeaders http.Header
		gotBody    []byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotHeaders = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := newTestClient(t, testHTTPClient())
	sub := rfcSubscription()
	sub.Endpoint = server.URL + "/push/JzLQ3raZJfFBR0aqvOMsLrt54w4rJUsV"

	payload := []byte(`{"title":"Alt","body":"3 new articles"}`)
	result, err := client.Send(context.Background(), sub, Message{Payload: payload, TTL: 60 * time.Second})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.StatusCode != http.StatusCreated || result.Gone || result.Retryable {
		t.Errorf("result = %+v, want a plain 201 success", result)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if want := "/push/JzLQ3raZJfFBR0aqvOMsLrt54w4rJUsV"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	headerTests := []struct {
		header string
		want   string
	}{
		{"Content-Encoding", "aes128gcm"},
		{"Content-Type", "application/octet-stream"},
		{"TTL", "60"},
	}
	for _, ht := range headerTests {
		if got := gotHeaders.Get(ht.header); got != ht.want {
			t.Errorf("%s = %q, want %q", ht.header, got, ht.want)
		}
	}
	if auth := gotHeaders.Get("Authorization"); !strings.HasPrefix(auth, "vapid t=") || !strings.Contains(auth, ", k=") {
		t.Errorf("Authorization = %q, want the vapid t=/k= form", auth)
	}
	// Normal is the protocol default, so the header is left off entirely.
	if got := gotHeaders.Get("Urgency"); got != "" {
		t.Errorf("Urgency = %q, want it omitted for normal urgency", got)
	}
	if got := gotHeaders.Get("Topic"); got != "" {
		t.Errorf("Topic = %q, want it omitted when unset", got)
	}

	uaPriv, err := ecdh.P256().NewPrivateKey(mustDecodeB64(t, rfcUAPrivateB64))
	if err != nil {
		t.Fatalf("parse user agent private key: %v", err)
	}
	decrypted, err := decryptForTest(t, gotBody, uaPriv, mustDecodeB64(t, rfcAuthSecretB64))
	if err != nil {
		t.Fatalf("decrypt request body: %v", err)
	}
	if string(decrypted) != string(payload) {
		t.Errorf("delivered payload = %q, want %q", decrypted, payload)
	}
}

func TestClient_Send_TTLHeaderAlwaysPresent(t *testing.T) {
	tests := []struct {
		name string
		ttl  time.Duration
		want string
	}{
		{"zero TTL is still sent", 0, "0"},
		{"seconds", 60 * time.Second, "60"},
		{"hours", 24 * time.Hour, "86400"},
		{"sub-second truncates down", 1500 * time.Millisecond, "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			var present bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, present = r.Header["Ttl"]
				got = r.Header.Get("TTL")
				w.WriteHeader(http.StatusCreated)
			}))
			defer server.Close()

			sub := rfcSubscription()
			sub.Endpoint = server.URL + "/push/abc"
			client := newTestClient(t, testHTTPClient())

			if _, err := client.Send(context.Background(), sub, Message{Payload: []byte("x"), TTL: tt.ttl}); err != nil {
				t.Fatalf("Send: %v", err)
			}
			if !present {
				t.Fatal("TTL header must always be sent; omitting it is a 400 from every push service")
			}
			if got != tt.want {
				t.Errorf("TTL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClient_Send_UrgencyAndTopic(t *testing.T) {
	tests := []struct {
		name        string
		urgency     Urgency
		topic       string
		wantUrgency string
		wantTopic   string
	}{
		{name: "unset urgency omits the header", wantUrgency: ""},
		{name: "explicit normal omits the header", urgency: UrgencyNormal, wantUrgency: ""},
		{name: "very low", urgency: UrgencyVeryLow, wantUrgency: "very-low"},
		{name: "low", urgency: UrgencyLow, wantUrgency: "low"},
		{name: "high", urgency: UrgencyHigh, wantUrgency: "high"},
		{name: "topic is forwarded", topic: "recap-2026-08-08", wantTopic: "recap-2026-08-08"},
		{name: "topic at the length limit", topic: strings.Repeat("a", maxTopicLength), wantTopic: strings.Repeat("a", maxTopicLength)},
		{name: "topic using the full url-safe alphabet", topic: "aZ0-_", wantTopic: "aZ0-_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotUrgency, gotTopic string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotUrgency = r.Header.Get("Urgency")
				gotTopic = r.Header.Get("Topic")
				w.WriteHeader(http.StatusCreated)
			}))
			defer server.Close()

			sub := rfcSubscription()
			sub.Endpoint = server.URL + "/push/abc"
			client := newTestClient(t, testHTTPClient())

			msg := Message{Payload: []byte("x"), TTL: time.Minute, Urgency: tt.urgency, Topic: tt.topic}
			if _, err := client.Send(context.Background(), sub, msg); err != nil {
				t.Fatalf("Send: %v", err)
			}
			if gotUrgency != tt.wantUrgency {
				t.Errorf("Urgency = %q, want %q", gotUrgency, tt.wantUrgency)
			}
			if gotTopic != tt.wantTopic {
				t.Errorf("Topic = %q, want %q", gotTopic, tt.wantTopic)
			}
		})
	}
}

func TestClient_Send_InvalidMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
	}{
		{name: "negative TTL", msg: Message{TTL: -time.Second}},
		{name: "unknown urgency", msg: Message{TTL: time.Minute, Urgency: "urgent"}},
		{name: "topic too long", msg: Message{TTL: time.Minute, Topic: strings.Repeat("a", maxTopicLength+1)}},
		{name: "topic with a slash", msg: Message{TTL: time.Minute, Topic: "recap/today"}},
		{name: "topic with a plus", msg: Message{TTL: time.Minute, Topic: "recap+today"}},
		{name: "topic with padding", msg: Message{TTL: time.Minute, Topic: "recap="}},
		{name: "topic with a space", msg: Message{TTL: time.Minute, Topic: "recap today"}},
		{name: "payload over the size limit", msg: Message{TTL: time.Minute, Payload: make([]byte, MaxPayloadLength+1)}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("an invalid message must be rejected before any request is made")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	sub := rfcSubscription()
	sub.Endpoint = server.URL + "/push/abc"
	client := newTestClient(t, testHTTPClient())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.Send(context.Background(), sub, tt.msg)
			if err == nil {
				t.Fatal("expected an error")
			}
			if result.Retryable {
				t.Error("a malformed message is a sender bug, never retryable")
			}
		})
	}
}

func TestClient_Send_StatusClassification(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		wantGone      bool
		wantRetryable bool
		wantErr       bool
	}{
		{name: "200 OK", status: http.StatusOK},
		{name: "201 Created", status: http.StatusCreated},
		{name: "202 Accepted", status: http.StatusAccepted},

		// The caller MUST delete the subscription on these.
		{name: "404 Not Found", status: http.StatusNotFound, wantGone: true, wantErr: true},
		{name: "410 Gone", status: http.StatusGone, wantGone: true, wantErr: true},

		{name: "429 Too Many Requests", status: http.StatusTooManyRequests, wantRetryable: true, wantErr: true},
		{name: "500 Internal Server Error", status: http.StatusInternalServerError, wantRetryable: true, wantErr: true},
		{name: "502 Bad Gateway", status: http.StatusBadGateway, wantRetryable: true, wantErr: true},
		{name: "503 Service Unavailable", status: http.StatusServiceUnavailable, wantRetryable: true, wantErr: true},
		{name: "504 Gateway Timeout", status: http.StatusGatewayTimeout, wantRetryable: true, wantErr: true},

		// Sender bugs: retrying reproduces them forever.
		{name: "400 Bad Request", status: http.StatusBadRequest, wantErr: true},
		{name: "401 Unauthorized", status: http.StatusUnauthorized, wantErr: true},
		{name: "403 Forbidden", status: http.StatusForbidden, wantErr: true},
		{name: "413 Payload Too Large", status: http.StatusRequestEntityTooLarge, wantErr: true},
		{name: "406 Not Acceptable", status: http.StatusNotAcceptable, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			sub := rfcSubscription()
			sub.Endpoint = server.URL + "/push/abc"
			client := newTestClient(t, testHTTPClient())

			result, err := client.Send(context.Background(), sub, Message{Payload: []byte("x"), TTL: time.Minute})
			if tt.wantErr != (err != nil) {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if result.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", result.StatusCode, tt.status)
			}
			if result.Gone != tt.wantGone {
				t.Errorf("Gone = %v, want %v", result.Gone, tt.wantGone)
			}
			if result.Retryable != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", result.Retryable, tt.wantRetryable)
			}
			if result.Gone && result.Retryable {
				t.Error("Gone and Retryable are mutually exclusive")
			}
		})
	}
}

func TestClient_Send_RetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		status     int
		retryAfter string
		want       time.Duration
	}{
		{name: "absent", status: http.StatusTooManyRequests, retryAfter: "", want: 0},
		{name: "delta seconds", status: http.StatusTooManyRequests, retryAfter: "120", want: 2 * time.Minute},
		{name: "zero delta", status: http.StatusTooManyRequests, retryAfter: "0", want: 0},
		{name: "http date in the future", status: http.StatusServiceUnavailable, retryAfter: "Sat, 08 Aug 2026 12:05:00 GMT", want: 5 * time.Minute},
		{name: "http date in the past clamps to zero", status: http.StatusServiceUnavailable, retryAfter: "Sat, 08 Aug 2026 11:55:00 GMT", want: 0},
		{name: "unparseable value ignored", status: http.StatusTooManyRequests, retryAfter: "soon", want: 0},
		{name: "negative delta ignored", status: http.StatusTooManyRequests, retryAfter: "-30", want: 0},
		{name: "ignored on success", status: http.StatusCreated, retryAfter: "120", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			sub := rfcSubscription()
			sub.Endpoint = server.URL + "/push/abc"
			client := newTestClient(t, testHTTPClient())
			client.nowFn = func() time.Time { return now }

			result, _ := client.Send(context.Background(), sub, Message{Payload: []byte("x"), TTL: time.Minute})
			if result.RetryAfter != tt.want {
				t.Errorf("RetryAfter = %v, want %v", result.RetryAfter, tt.want)
			}
		})
	}
}

func TestClient_Send_TransportFailureIsRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	endpoint := server.URL + "/push/abc"
	server.Close() // nothing is listening now

	sub := rfcSubscription()
	sub.Endpoint = endpoint
	client := newTestClient(t, testHTTPClient())

	result, err := client.Send(context.Background(), sub, Message{Payload: []byte("x"), TTL: time.Minute})
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if !result.Retryable {
		t.Error("a transport failure must be retryable")
	}
	if result.Gone {
		t.Error("a transport failure must not be reported as Gone")
	}
	if result.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0 when no response arrived", result.StatusCode)
	}
}

func TestClient_Send_ContextCancellation(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	defer close(release)

	sub := rfcSubscription()
	sub.Endpoint = server.URL + "/push/abc"
	client := newTestClient(t, testHTTPClient())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := client.Send(ctx, sub, Message{Payload: []byte("x"), TTL: time.Minute})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want it to wrap context.Canceled", err)
	}
	if !result.Retryable {
		t.Error("a cancelled send is retryable")
	}
}

// trackingBody records whether the response body was closed.
type trackingBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *trackingBody) Close() error {
	b.closed.Store(true)
	return nil
}

type stubTransport struct {
	body   *trackingBody
	status int
}

func (s *stubTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.status,
		Body:       s.body,
		Header:     make(http.Header),
	}, nil
}

// TestClient_Send_ClosesResponseBody guards the leak that silently exhausts the
// connection pool after a few thousand sends.
func TestClient_Send_ClosesResponseBody(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusGone, http.StatusTooManyRequests, http.StatusInternalServerError} {
		body := &trackingBody{Reader: strings.NewReader("response text")}
		client := newTestClient(t, &http.Client{
			Timeout:   5 * time.Second,
			Transport: &stubTransport{body: body, status: status},
		})

		sub := rfcSubscription()
		sub.Endpoint = "https://push.example.net/push/abc"

		if _, err := client.Send(context.Background(), sub, Message{Payload: []byte("x"), TTL: time.Minute}); err != nil && status < 300 {
			t.Fatalf("Send: %v", err)
		}
		if !body.closed.Load() {
			t.Errorf("status %d: response body was not closed", status)
		}
	}
}

// TestClient_Send_DoesNotMutateCallerPayload covers the webpush-go v1.4.0 data
// race, where a payload slice shared across concurrent sends is written in place.
func TestClient_Send_DoesNotMutateCallerPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	sub := rfcSubscription()
	sub.Endpoint = server.URL + "/push/abc"
	client := newTestClient(t, testHTTPClient())

	payload := []byte(`{"title":"Alt","body":"shared across sends"}`)
	original := string(payload)

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.Send(context.Background(), sub, Message{Payload: payload, TTL: time.Minute}); err != nil {
				t.Errorf("Send: %v", err)
			}
		}()
	}
	wg.Wait()

	if string(payload) != original {
		t.Errorf("caller payload mutated to %q, want %q", payload, original)
	}
}

func TestClient_Send_InvalidEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{"empty", ""},
		{"relative", "/push/abc"},
		{"no host", "https:///push/abc"},
	}

	client := newTestClient(t, testHTTPClient())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := rfcSubscription()
			sub.Endpoint = tt.endpoint
			result, err := client.Send(context.Background(), sub, Message{Payload: []byte("x"), TTL: time.Minute})
			if err == nil {
				t.Fatal("expected an error")
			}
			if result.Retryable {
				t.Error("a malformed endpoint is not retryable")
			}
		})
	}
}

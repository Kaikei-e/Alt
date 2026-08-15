package webpush

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Urgency is the RFC 8030 section 5.3 delivery urgency.
type Urgency string

const (
	UrgencyVeryLow Urgency = "very-low"
	UrgencyLow     Urgency = "low"
	UrgencyNormal  Urgency = "normal"
	UrgencyHigh    Urgency = "high"
)

const (
	maxTopicLength = 32

	// defaultTimeout bounds a single push delivery. Push services are third
	// parties, so an unbounded wait is a wedged worker.
	defaultTimeout = 10 * time.Second

	// errorBodyLimit caps how much of an error response is read off the wire.
	errorBodyLimit = 1024

	// maxBodyExcerptLength caps what survives sanitising. The excerpt is stored
	// on a delivery row and repeated in a log line, and a push service that
	// answers with an HTML error page would otherwise put a kilobyte of markup
	// in both. Every push service's actual reason is a short JSON object.
	maxBodyExcerptLength = 256
	excerptEllipsis      = "..."

	redactionMarker = "[redacted]"
)

// Message is one push message. TTL is always transmitted: RFC 8030 makes the
// header mandatory and every push service answers 400 without it. TTL zero is a
// meaningful value ("deliver now or discard"), not an unset marker.
type Message struct {
	Payload []byte
	TTL     time.Duration
	// Urgency defaults to UrgencyNormal, which is sent by omitting the header.
	Urgency Urgency
	// Topic, when set, replaces any undelivered message with the same topic.
	Topic string
}

// SendResult classifies a delivery attempt. It is returned even alongside an
// error so the caller cannot miss the Gone branch by only checking err.
type SendResult struct {
	StatusCode int
	// Gone reports 404 or 410: the subscription is dead and MUST be deleted.
	Gone bool
	// Retryable reports 429, 5xx, or a transport failure.
	Retryable bool
	// RetryAfter is the server's requested delay, when it sent one.
	RetryAfter time.Duration
	// BodyExcerpt is the push service's own account of why it refused, made
	// safe to store and to log: single line, printable, bounded, and with the
	// capability URL and the VAPID credentials scrubbed out. Empty on success.
	//
	// It exists because the status code alone cannot distinguish a rotated
	// VAPID key from a malformed subject from a topic the service dislikes —
	// all three are a flat 400, and the body is the only thing that says which.
	BodyExcerpt string
}

// Client delivers Web Push messages. It is safe for concurrent use.
type Client struct {
	httpClient *http.Client
	signer     *VAPIDSigner
	encrypter  *Encrypter
	nowFn      func() time.Time
}

// NewClient builds a client. A nil httpClient gets a default carrying an explicit
// timeout; a supplied one must set Timeout itself, since a zero-value http.Client
// waits forever.
func NewClient(signer *VAPIDSigner, httpClient *http.Client) (*Client, error) {
	if signer == nil {
		return nil, fmt.Errorf("webpush client requires a VAPID signer")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	} else if httpClient.Timeout <= 0 {
		return nil, fmt.Errorf("webpush client requires an http.Client with an explicit timeout")
	}

	return &Client{
		httpClient: httpClient,
		signer:     signer,
		encrypter:  NewEncrypter(),
		nowFn:      time.Now,
	}, nil
}

// PublicKey returns the application server key the browser must subscribe with.
func (c *Client) PublicKey() string { return c.signer.PublicKey() }

// Send delivers one message to one subscription. The returned SendResult is
// always populated, including when err is non-nil.
func (c *Client) Send(ctx context.Context, sub Subscription, msg Message) (SendResult, error) {
	if err := msg.validate(); err != nil {
		return SendResult{}, err
	}

	authorization, err := c.signer.AuthorizationHeader(sub.Endpoint)
	if err != nil {
		return SendResult{}, err
	}

	body, err := c.encrypter.Encrypt(sub, msg.Payload)
	if err != nil {
		return SendResult{}, err
	}

	// bytes.NewReader over a body this call owns: nothing is shared with the
	// caller's payload slice or with a concurrent send.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return SendResult{}, fmt.Errorf("webpush build request: %w", err)
	}

	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", strconv.FormatInt(int64(msg.TTL.Seconds()), 10))
	req.ContentLength = int64(len(body))
	if msg.Urgency != "" && msg.Urgency != UrgencyNormal {
		req.Header.Set("Urgency", string(msg.Urgency))
	}
	if msg.Topic != "" {
		req.Header.Set("Topic", msg.Topic)
	}

	// The host is the only part of the endpoint that may be named anywhere an
	// operator can read: the path is a bearer capability for that device.
	host := hostOf(sub.Endpoint)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// No response reached us, so the message may or may not have been
		// delivered; retrying is the safe choice for an at-least-once sender.
		//
		// net/http puts the full request URL in its own message, so the message
		// is scrubbed while the error itself stays wrapped — callers still match
		// context.Canceled and friends with errors.Is.
		return SendResult{Retryable: true}, fmt.Errorf("webpush post to %s: %w", host, &redactedError{
			message: sanitizeExcerpt(err.Error(), sub.Endpoint, authorization),
			cause:   err,
		})
	}
	// Closing a response body cannot fail in a way the caller can act on: the
	// status has already been read, and the only cost of a failed close is a
	// connection that will not be reused.
	defer func() { _ = resp.Body.Close() }()

	result := c.classify(resp)
	if result.Gone || result.Retryable || resp.StatusCode >= 300 {
		result.BodyExcerpt = sanitizeExcerpt(readErrorBody(resp.Body), sub.Endpoint, authorization)

		// Logged here rather than left to the caller because this is the only
		// place the response body exists. A dispatcher that dead-letters every
		// delivery with nothing but a status code is undiagnosable, and the
		// host is what says whether it is one push service or all of them.
		slog.WarnContext(ctx, "webpush_send_rejected",
			"push_host", host,
			"status", resp.StatusCode,
			"gone", result.Gone,
			"retryable", result.Retryable,
			"body_excerpt", result.BodyExcerpt,
		)

		return result, fmt.Errorf("webpush %s returned %s: %s", host, resp.Status, result.BodyExcerpt)
	}

	// Drain so the connection can be reused for the next send.
	_, _ = io.Copy(io.Discard, resp.Body)
	return result, nil
}

func (c *Client) classify(resp *http.Response) SendResult {
	result := SendResult{StatusCode: resp.StatusCode}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return result
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		// The subscription no longer exists; retrying can never succeed.
		result.Gone = true
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		result.Retryable = true
		result.RetryAfter = c.parseRetryAfter(resp.Header.Get("Retry-After"))
	}

	// Everything else (400, 401, 403, 413, ...) is a sender bug: the VAPID
	// config or the payload is wrong and every retry reproduces it.
	return result
}

func (c *Client) parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if delay := at.Sub(c.nowFn()); delay > 0 {
			return delay
		}
	}
	return 0
}

func (m Message) validate() error {
	if m.TTL < 0 {
		return fmt.Errorf("webpush TTL must not be negative, got %s", m.TTL)
	}
	switch m.Urgency {
	case "", UrgencyVeryLow, UrgencyLow, UrgencyNormal, UrgencyHigh:
	default:
		return fmt.Errorf("webpush urgency %q must be one of very-low, low, normal, high", m.Urgency)
	}
	if len(m.Payload) > MaxPayloadLength {
		return fmt.Errorf("webpush payload of %d bytes exceeds the maximum of %d", len(m.Payload), MaxPayloadLength)
	}
	return validateTopic(m.Topic)
}

// validateTopic enforces RFC 8030 section 5.4: at most 32 characters from the
// URL-safe base64 alphabet, without padding.
func validateTopic(topic string) error {
	if topic == "" {
		return nil
	}
	if len(topic) > maxTopicLength {
		return fmt.Errorf("webpush topic must be at most %d characters, got %d", maxTopicLength, len(topic))
	}
	for _, r := range topic {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return fmt.Errorf("webpush topic %q must use only the URL-safe base64 alphabet", topic)
		}
	}
	return nil
}

func readErrorBody(body io.Reader) string {
	snippet, err := io.ReadAll(io.LimitReader(body, errorBodyLimit))
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(snippet))
}

// redactedError replaces an error's message while keeping its identity, so
// errors.Is still reaches context.Canceled through a scrubbed transport error.
type redactedError struct {
	message string
	cause   error
}

func (e *redactedError) Error() string { return e.message }
func (e *redactedError) Unwrap() error { return e.cause }

// hostOf reduces an endpoint to its host, which is the only part of a
// subscription URL that is safe to write down: the path is a bearer capability
// and anyone holding it can push to that device. Endpoints reaching here have
// already been parsed by the VAPID signer, so a failure means an empty host
// rather than a leak.
func hostOf(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "unknown"
	}
	if parsed.Host == "" {
		return "unknown"
	}
	return parsed.Host
}

// bearerRun matches a long run of the base64url alphabet — the shape of both a
// VAPID JWT segment and a subscription's capability token.
var bearerRun = regexp.MustCompile(`[A-Za-z0-9_-]{32,}`)

// sanitizeExcerpt turns whatever a push service said into something that can be
// stored on a delivery row and printed in a log: one line, printable, bounded,
// and carrying no credential.
//
// The scrubbing is two-layered on purpose. Exact removal of the secrets we know
// we sent handles a service that quotes the request back verbatim; the
// base64url sweep handles one that quotes back only a fragment, which exact
// matching would miss while still handing out a usable prefix.
func sanitizeExcerpt(raw string, secrets ...string) string {
	cleaned := strings.ToValidUTF8(raw, "")
	for _, secret := range secrets {
		if len(secret) < 8 {
			continue
		}
		cleaned = strings.ReplaceAll(cleaned, secret, redactionMarker)
	}
	cleaned = bearerRun.ReplaceAllString(cleaned, redactionMarker)
	return truncateExcerpt(collapseWhitespace(cleaned))
}

// collapseWhitespace folds every run of whitespace or control characters into a
// single space, so a multi-line HTML error page cannot break one log line into
// many or smuggle terminal escapes into an operator's console.
func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pendingSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) || !unicode.IsPrint(r) {
			pendingSpace = b.Len() > 0
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func truncateExcerpt(s string) string {
	if len(s) <= maxBodyExcerptLength {
		return s
	}
	cut := maxBodyExcerptLength - len(excerptEllipsis)
	// Never split a multi-byte rune: the excerpt is quoted into a log line and
	// stored in a text column, both of which expect valid UTF-8.
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + excerptEllipsis
}

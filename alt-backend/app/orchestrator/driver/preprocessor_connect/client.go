// Package preprocessor_connect provides Connect-RPC client for pre-processor service.
package preprocessor_connect

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"

	ppv2 "alt/gen/proto/services/preprocessor/v2"
	"alt/gen/proto/services/preprocessor/v2/preprocessorv2connect"
	"alt/shared/domain/authcontext"
	"alt/tlsutil"
)

// SummarizeStatus represents the status of a summarization job.
type SummarizeStatus struct {
	JobID        string
	Status       string
	Summary      string
	ErrorMessage string
	ArticleID    string
}

// ConnectPreProcessorClient provides Connect-RPC client for pre-processor.
type ConnectPreProcessorClient struct {
	client preprocessorv2connect.PreProcessorServiceClient
}

type authInterceptor struct{}

func (i *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if jwtToken, ok := authcontext.JWTFromContext(ctx); ok && jwtToken != "" {
			req.Header().Set("X-Alt-Backend-Token", jwtToken)
		}
		return next(ctx, req)
	}
}

func (i *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		if jwtToken, ok := authcontext.JWTFromContext(ctx); ok && jwtToken != "" {
			conn.RequestHeader().Set("X-Alt-Backend-Token", jwtToken)
		}
		return conn
	}
}

func (i *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func defaultPreProcessorHTTPClient(baseURL string) (connect.HTTPClient, error) {
	if !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("pre-processor baseURL must use https:// scheme, got: %s", baseURL)
	}
	certFile := os.Getenv("MTLS_CERT_FILE")
	keyFile := os.Getenv("MTLS_KEY_FILE")
	caFile := os.Getenv("MTLS_CA_FILE")

	if certFile == "" || keyFile == "" || caFile == "" {
		return nil, fmt.Errorf("missing MTLS cert/key/ca environment variables")
	}

	tlsCfg, err := tlsutil.LoadClientConfig(certFile, keyFile, caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS client config: %w", err)
	}

	serverName := os.Getenv("PRE_PROCESSOR_MTLS_SERVER_NAME")
	if serverName == "" {
		serverName = "pre-processor"
	}
	tlsCfg.ServerName = serverName
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:     tlsCfg,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

// NewConnectPreProcessorClient creates a new Connect-RPC client for pre-processor.
func NewConnectPreProcessorClient(baseURL, _ string) (*ConnectPreProcessorClient, error) {
	httpClient, err := defaultPreProcessorHTTPClient(baseURL)
	if err != nil {
		return nil, err
	}
	client := preprocessorv2connect.NewPreProcessorServiceClient(
		httpClient,
		baseURL,
		connect.WithProtoJSON(),
		connect.WithInterceptors(&authInterceptor{}),
	)
	return &ConnectPreProcessorClient{client: client}, nil
}

// NewConnectPreProcessorClientWithHTTPClient creates a client with an injected HTTP client, for tests only.
func NewConnectPreProcessorClientWithHTTPClient(baseURL string, httpClient connect.HTTPClient) *ConnectPreProcessorClient {
	client := preprocessorv2connect.NewPreProcessorServiceClient(
		httpClient,
		baseURL,
		connect.WithProtoJSON(),
		connect.WithInterceptors(&authInterceptor{}),
	)
	return &ConnectPreProcessorClient{client: client}
}

// Summarize performs synchronous article summarization via Connect-RPC.
func (c *ConnectPreProcessorClient) Summarize(ctx context.Context, content, articleID, title string) (string, error) {
	resp, err := c.client.Summarize(ctx, connect.NewRequest(&ppv2.SummarizeRequest{
		ArticleId: articleID,
		Title:     title,
		Content:   content,
	}))
	if err != nil {
		return "", err
	}
	return resp.Msg.Summary, nil
}

// StreamSummarize performs streaming article summarization via Connect-RPC.
// Returns an io.ReadCloser that can be used to read the streaming response.
func (c *ConnectPreProcessorClient) StreamSummarize(ctx context.Context, content, articleID, title string) (io.ReadCloser, error) {
	stream, err := c.client.StreamSummarize(ctx, connect.NewRequest(&ppv2.StreamSummarizeRequest{
		ArticleId: articleID,
		Title:     title,
		Content:   content,
	}))
	if err != nil {
		return nil, err
	}
	return &streamAdapter{stream: stream}, nil
}

// QueueSummarize submits an article for async summarization via Connect-RPC.
func (c *ConnectPreProcessorClient) QueueSummarize(ctx context.Context, articleID, title string) (string, error) {
	resp, err := c.client.QueueSummarize(ctx, connect.NewRequest(&ppv2.QueueSummarizeRequest{
		ArticleId: articleID,
		Title:     title,
	}))
	if err != nil {
		return "", err
	}
	return resp.Msg.JobId, nil
}

// GetSummarizeStatus checks the status of a summarization job via Connect-RPC.
func (c *ConnectPreProcessorClient) GetSummarizeStatus(ctx context.Context, jobID string) (*SummarizeStatus, error) {
	resp, err := c.client.GetSummarizeStatus(ctx, connect.NewRequest(&ppv2.GetSummarizeStatusRequest{
		JobId: jobID,
	}))
	if err != nil {
		return nil, err
	}
	return &SummarizeStatus{
		JobID:        resp.Msg.JobId,
		Status:       resp.Msg.Status,
		Summary:      resp.Msg.Summary,
		ErrorMessage: resp.Msg.ErrorMessage,
		ArticleID:    resp.Msg.ArticleId,
	}, nil
}

// streamAdapter adapts the Connect-RPC server stream to io.ReadCloser.
type streamAdapter struct {
	stream *connect.ServerStreamForClient[ppv2.StreamSummarizeResponse]
	buf    []byte
}

func (a *streamAdapter) Read(p []byte) (n int, err error) {
	// If we have buffered data, return it first
	if len(a.buf) > 0 {
		n = copy(p, a.buf)
		a.buf = a.buf[n:]
		return n, nil
	}

	// Get next message from stream
	if !a.stream.Receive() {
		if err := a.stream.Err(); err != nil {
			return 0, err
		}
		return 0, io.EOF
	}

	msg := a.stream.Msg()
	if msg.IsFinal {
		return 0, io.EOF
	}

	// Copy chunk to output buffer
	chunk := []byte(msg.Chunk)
	n = copy(p, chunk)
	if n < len(chunk) {
		a.buf = chunk[n:]
	}
	return n, nil
}

func (a *streamAdapter) Close() error {
	return a.stream.Close()
}

// Package preprocessor_connect provides Connect-RPC client for pre-processor service.
package preprocessor_connect

import (
	"context"
	"io"
	"net/http"

	"connectrpc.com/connect"

	ppv2 "alt/gen/proto/clients/preprocessor/v2"
	"alt/gen/proto/clients/preprocessor/v2/preprocessorv2connect"
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

// NewConnectPreProcessorClient creates a new Connect-RPC client for pre-processor.
func NewConnectPreProcessorClient(baseURL string) *ConnectPreProcessorClient {
	client := preprocessorv2connect.NewPreProcessorServiceClient(
		http.DefaultClient,
		baseURL,
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

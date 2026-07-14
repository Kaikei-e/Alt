// Package preprocessor_summarize_gateway adapts
// driver/preprocessor_client's HTTP client to the
// port/preprocessor_summarize_port.PreProcessorSummarizePort contract.
package preprocessor_summarize_gateway

import (
	"alt/orchestrator/driver/preprocessor_client"
	"alt/orchestrator/port/preprocessor_summarize_port"
	"context"
	"io"
)

// Gateway implements preprocessor_summarize_port.PreProcessorSummarizePort
// on top of the driver-layer pre-processor HTTP client.
type Gateway struct {
	client *preprocessor_client.Client
}

// NewGateway creates a Gateway wrapping client.
func NewGateway(client *preprocessor_client.Client) *Gateway {
	return &Gateway{client: client}
}

func (g *Gateway) Summarize(ctx context.Context, content, articleID, title string) (string, error) {
	return g.client.Summarize(ctx, content, articleID, title)
}

func (g *Gateway) StreamSummarize(ctx context.Context, content, articleID, title string) (io.ReadCloser, error) {
	return g.client.StreamSummarize(ctx, content, articleID, title)
}

func (g *Gateway) QueueSummarize(ctx context.Context, articleID, title string) (string, error) {
	return g.client.QueueSummarize(ctx, articleID, title)
}

func (g *Gateway) GetSummarizeStatus(ctx context.Context, jobID string) (*preprocessor_summarize_port.SummarizeStatus, error) {
	status, err := g.client.GetSummarizeStatus(ctx, jobID)
	if err != nil || status == nil {
		return nil, err
	}
	return &preprocessor_summarize_port.SummarizeStatus{
		JobID:        status.JobID,
		Status:       status.Status,
		Summary:      status.Summary,
		ErrorMessage: status.ErrorMessage,
		ArticleID:    status.ArticleID,
	}, nil
}

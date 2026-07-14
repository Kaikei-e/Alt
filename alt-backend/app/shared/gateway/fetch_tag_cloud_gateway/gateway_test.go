package fetch_tag_cloud_gateway

import (
	"alt/utils/logger"
	"context"
	"testing"
)

func TestFetchTagCloudGateway_NilDB(t *testing.T) {
	logger.InitLogger()

	gateway := NewFetchTagCloudGateway(nil)
	ctx := context.Background()

	_, err := gateway.FetchTagCloud(ctx, 100)
	if err == nil {
		t.Error("expected error for nil database, got nil")
	}

	expectedError := "database connection not available"
	if err.Error() != expectedError {
		t.Errorf("expected error %q, got %q", expectedError, err.Error())
	}
}

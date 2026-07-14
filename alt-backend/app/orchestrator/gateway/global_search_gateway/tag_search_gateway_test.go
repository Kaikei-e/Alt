package global_search_gateway

import (
	"alt/utils/logger"
	"context"
	"testing"
)

func TestTagSearchGateway_NilRepo(t *testing.T) {
	logger.InitLogger()

	gw := NewTagSearchGateway(nil)
	_, err := gw.SearchTagsByPrefix(context.Background(), "ai", 10)
	if err == nil {
		t.Fatal("expected error for nil repo, got nil")
	}
	if err.Error() != "tag repository not available" {
		t.Errorf("expected 'tag repository not available', got %q", err.Error())
	}
}

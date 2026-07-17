package driver

import (
	"errors"
	"net"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

func TestIsIndexNotFoundErr(t *testing.T) {
	t.Parallel()

	indexNotFound := &meilisearch.Error{StatusCode: 400}
	indexNotFound.MeilisearchApiError.Code = "index_not_found"

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("connection refused"), want: false},
		{name: "communication error", err: &meilisearch.Error{
			StatusCode:  0,
			ErrCode:     meilisearch.MeilisearchCommunicationError,
			OriginError: &net.OpError{Op: "dial", Err: errors.New("connection refused")},
		}, want: false},
		{name: "404 status", err: &meilisearch.Error{StatusCode: 404}, want: true},
		{name: "index_not_found code", err: indexNotFound, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isIndexNotFoundErr(tt.err); got != tt.want {
				t.Fatalf("isIndexNotFoundErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

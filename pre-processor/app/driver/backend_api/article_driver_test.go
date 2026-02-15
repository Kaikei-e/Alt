package backend_api

import (
	"context"
	"testing"
	"time"
)

func TestFetchInoreaderArticles_NilDBPool(t *testing.T) {
	client := &Client{} // dummy client, not used for this method
	repo := NewArticleRepository(client, nil)

	_, err := repo.FetchInoreaderArticles(context.Background(), time.Now().Add(-1*time.Hour))
	if err == nil {
		t.Fatal("expected error when dbPool is nil, got nil")
	}

	want := "database connection is nil"
	if err.Error() != want {
		t.Errorf("got error %q, want %q", err.Error(), want)
	}
}

func TestFetchInoreaderArticles_DBPoolFieldIsSet(t *testing.T) {
	client := &Client{}
	repo := NewArticleRepository(client, nil)

	// Verify the struct stores the dbPool (nil in this case)
	if repo.dbPool != nil {
		t.Error("expected dbPool to be nil")
	}
}

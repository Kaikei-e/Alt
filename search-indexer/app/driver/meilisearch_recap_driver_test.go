package driver

import (
	"context"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

type recapTestServiceManager struct {
	meilisearch.ServiceManager
	idx            meilisearch.IndexManager
	createCalled   bool
	createdConfig  *meilisearch.IndexConfig
	createIndexErr error
	createTaskUID  int64
}

func (s *recapTestServiceManager) Index(_ string) meilisearch.IndexManager {
	return s.idx
}

func (s *recapTestServiceManager) CreateIndexWithContext(_ context.Context, cfg *meilisearch.IndexConfig) (*meilisearch.TaskInfo, error) {
	s.createCalled = true
	s.createdConfig = cfg
	if s.createIndexErr != nil {
		return nil, s.createIndexErr
	}
	uid := s.createTaskUID
	if uid == 0 {
		uid = 10
	}
	return &meilisearch.TaskInfo{TaskUID: uid}, nil
}

func TestMeilisearchRecapDriver_EnsureIndex_NonNotFoundFetchInfoError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "500 internal server error",
			err:  &meilisearch.Error{StatusCode: 500},
		},
		{
			name: "timeout error",
			err: &meilisearch.Error{
				ErrCode:     meilisearch.MeilisearchTimeoutError,
				OriginError: context.DeadlineExceeded,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeIdx := newFakeRecapIndexManager(t)
			fakeIdx.fetchInfoErr = tt.err
			sm := &recapTestServiceManager{idx: fakeIdx}
			d := NewMeilisearchRecapDriver(sm)

			err := d.EnsureIndex(context.Background())
			if err == nil {
				t.Fatalf("EnsureIndex() error = nil, want error for FetchInfo failure %v", tt.err)
			}
			if fakeIdx.called("AddDocuments") || fakeIdx.called("AddDocumentsNoContext") {
				t.Fatal("EnsureIndex() must not add documents when FetchInfo fails with a non-404 error")
			}
			if sm.createCalled {
				t.Fatal("EnsureIndex() must not call CreateIndexWithContext when FetchInfo fails with a non-404 error")
			}
		})
	}
}

func TestMeilisearchRecapDriver_EnsureIndex_CreatesIndexExplicitlyWhenNotFound(t *testing.T) {
	fakeIdx := newFakeRecapIndexManager(t)
	notFoundErr := &meilisearch.Error{StatusCode: 404}
	notFoundErr.MeilisearchApiError.Code = "index_not_found"
	fakeIdx.fetchInfoErr = notFoundErr

	sm := &recapTestServiceManager{idx: fakeIdx}
	d := NewMeilisearchRecapDriver(sm)

	if err := d.EnsureIndex(context.Background()); err != nil {
		t.Fatalf("EnsureIndex() unexpected error = %v", err)
	}
	if !sm.createCalled {
		t.Fatal("EnsureIndex() did not call CreateIndexWithContext when index was not found")
	}
	if sm.createdConfig == nil || sm.createdConfig.Uid != "recaps" || sm.createdConfig.PrimaryKey != "id" {
		t.Fatalf("CreateIndexWithContext config = %+v, want Uid=\"recaps\" PrimaryKey=\"id\"", sm.createdConfig)
	}
	if fakeIdx.called("AddDocuments") || fakeIdx.called("AddDocumentsNoContext") {
		t.Fatal("EnsureIndex() must not insert a dummy document to create the index")
	}
}

package recap_gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alt/orchestrator/driver/recap_job_driver"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecapJobGateway_GetRecapJobs(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/dashboard/recap_jobs", r.URL.Path)
		assert.Equal(t, "86400", r.URL.Query().Get("window"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{
			"job_id": "job-123",
			"status": "completed",
			"last_stage": "clustering",
			"kicked_at": "` + now.Format(time.RFC3339) + `",
			"updated_at": "` + now.Format(time.RFC3339) + `"
		}]`))
	}))
	defer ts.Close()

	driver := recap_job_driver.NewDriver(ts.URL)
	gw := NewRecapJobGateway(driver)

	jobs, err := gw.GetRecapJobs(context.Background(), 86400, 10)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	assert.Equal(t, "job-123", jobs[0].JobID)
	assert.Equal(t, "completed", jobs[0].Status)
	require.NotNil(t, jobs[0].LastStage)
	assert.Equal(t, "clustering", *jobs[0].LastStage)
}

func TestMapRecapJobDTOsToDomain(t *testing.T) {
	now := time.Now()
	stage := "summarization"
	dtos := []recap_job_driver.JobDTO{
		{
			JobID:     "j1",
			Status:    "running",
			LastStage: &stage,
			KickedAt:  now,
			UpdatedAt: now,
		},
	}

	jobs := mapRecapJobDTOsToDomain(dtos)
	require.Len(t, jobs, 1)
	assert.Equal(t, "j1", jobs[0].JobID)
	assert.Equal(t, "running", jobs[0].Status)
	assert.Equal(t, &stage, jobs[0].LastStage)
	assert.Equal(t, now, jobs[0].KickedAt)
}

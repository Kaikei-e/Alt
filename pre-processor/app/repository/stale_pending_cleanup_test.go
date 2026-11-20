package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStalePendingCleanupQueries(t *testing.T) {
	t.Run("count query targets pending rows that already have summaries", func(t *testing.T) {
		assert.Contains(t, countStalePendingJobsQuery, "summarize_job_queue q")
		assert.Contains(t, countStalePendingJobsQuery, "q.status = 'pending'")
		assert.Contains(t, countStalePendingJobsQuery, "EXISTS (")
		assert.Contains(t, countStalePendingJobsQuery, "article_summaries s")
	})

	t.Run("delete query only deletes pending rows that already have summaries", func(t *testing.T) {
		assert.Contains(t, deleteStalePendingJobsQuery, "DELETE FROM summarize_job_queue")
		assert.Contains(t, deleteStalePendingJobsQuery, "status = 'pending'")
		assert.Contains(t, deleteStalePendingJobsQuery, "article_summaries s")
		assert.Contains(t, deleteStalePendingJobsQuery, "RETURNING")
	})
}

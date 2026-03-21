package repository

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStalePendingCleanupQueries(t *testing.T) {
	t.Run("count query targets pending rows that already have summaries", func(t *testing.T) {
		assert.True(t, strings.Contains(countStalePendingJobsQuery, "summarize_job_queue q"))
		assert.True(t, strings.Contains(countStalePendingJobsQuery, "q.status = 'pending'"))
		assert.True(t, strings.Contains(countStalePendingJobsQuery, "EXISTS ("))
		assert.True(t, strings.Contains(countStalePendingJobsQuery, "article_summaries s"))
	})

	t.Run("delete query only deletes pending rows that already have summaries", func(t *testing.T) {
		assert.True(t, strings.Contains(deleteStalePendingJobsQuery, "DELETE FROM summarize_job_queue"))
		assert.True(t, strings.Contains(deleteStalePendingJobsQuery, "status = 'pending'"))
		assert.True(t, strings.Contains(deleteStalePendingJobsQuery, "article_summaries s"))
		assert.True(t, strings.Contains(deleteStalePendingJobsQuery, "RETURNING"))
	})
}

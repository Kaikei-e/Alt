package preprocessor_summarize_gateway

import (
	"alt/orchestrator/driver/preprocessor_client"
	"testing"
)

func TestMapClientStatusToPortStatus(t *testing.T) {
	if mapClientStatusToPortStatus(nil) != nil {
		t.Fatal("expected nil for nil input")
	}

	src := &preprocessor_client.SummarizeStatus{
		JobID:        "job-1",
		Status:       "completed",
		Summary:      "A summary",
		ErrorMessage: "",
		ArticleID:    "article-1",
	}

	res := mapClientStatusToPortStatus(src)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.JobID != "job-1" || res.Status != "completed" || res.Summary != "A summary" || res.ArticleID != "article-1" {
		t.Errorf("unexpected mapped result: %+v", res)
	}
}

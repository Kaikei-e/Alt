package summarization

import (
	"alt/config"
	"alt/di"
	"alt/utils/logger"
	"net/http"

	"github.com/labstack/echo/v4"
)

// RestHandleSummarizeFeedStatus returns the current state of an async summarization job.
func RestHandleSummarizeFeedStatus(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		jobID := c.Param("job_id")
		if jobID == "" {
			logger.Logger.Warn("Empty job_id provided")
			return echo.NewHTTPError(http.StatusBadRequest, "job_id is required")
		}

		logger.Logger.Debug("Checking summarization job status", "job_id", jobID)

		status, err := callPreProcessorSummarizeStatus(c.Request().Context(), jobID, cfg.PreProcessor.URL)
		if err != nil {
			logger.Logger.Error("Failed to get summarization job status", "error", err, "job_id", jobID)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get job status")
		}

		if status == nil {
			return echo.NewHTTPError(http.StatusNotFound, "Job not found")
		}

		response := map[string]interface{}{
			"job_id":     status.JobID,
			"status":     status.Status,
			"article_id": status.ArticleID,
		}

		if status.Status == "completed" && status.Summary != "" {
			response["summary"] = status.Summary
		}

		if status.Status == "failed" && status.ErrorMessage != "" {
			response["error_message"] = status.ErrorMessage
		}

		return c.JSON(http.StatusOK, response)
	}
}

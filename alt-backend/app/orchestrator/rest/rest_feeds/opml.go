package rest_feeds

import (
	"alt/di"
	"alt/utils/logger"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

const maxOPMLFileSize = 1 * 1024 * 1024 // 1MB

// RestHandleExportOPML generates OPML 2.0 XML for all registered feeds.
// GET /v1/rss-feed-link/export/opml
func RestHandleExportOPML(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		xmlData, err := container.ExportOPMLUsecase.Execute(ctx)
		if err != nil {
			return HandleError(c, err, "export_opml")
		}

		c.Response().Header().Set("Content-Disposition", `attachment; filename="alt-feeds.opml"`)
		return c.Blob(http.StatusOK, "application/xml", xmlData)
	}
}

// RestHandleImportOPML parses an uploaded OPML file and registers feeds.
// POST /v1/rss-feed-link/import/opml
func RestHandleImportOPML(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		// Read multipart file
		file, err := c.FormFile("file")
		if err != nil {
			return HandleValidationError(c, "OPML file is required (field: 'file')", "file", "missing")
		}

		// Validate file size
		if file.Size > maxOPMLFileSize {
			return HandleValidationError(c, "OPML file too large (max 1MB)", "file", file.Size)
		}

		src, err := file.Open()
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to open uploaded file", "error", err)
			return HandleValidationError(c, "Failed to read uploaded file", "file", "unreadable")
		}
		defer src.Close()

		// Read file content with size limit
		data, err := io.ReadAll(io.LimitReader(src, maxOPMLFileSize+1))
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to read OPML file", "error", err)
			return HandleValidationError(c, "Failed to read OPML file", "file", "read_error")
		}

		if int64(len(data)) > maxOPMLFileSize {
			return HandleValidationError(c, "OPML file too large (max 1MB)", "file", len(data))
		}

		result, err := container.ImportOPMLUsecase.Execute(ctx, data)
		if err != nil {
			return HandleError(c, err, "import_opml")
		}

		return c.JSON(http.StatusOK, result)
	}
}

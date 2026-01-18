package job

import (
	"alt/utils/logger"
	"context"
	"encoding/csv"
	"net/url"
	"os"
	"path/filepath"
)

const (
	CSVPath = "datastore/list.csv"
)

func PathCleaner(csvPath string) string {
	ctx := context.Background()
	wd, err := os.Getwd()
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Error getting working directory", "error", err)
		return ""
	}
	cleanedPath := filepath.Join(wd, csvPath)
	logger.Logger.InfoContext(ctx, "Cleaned path", "path", cleanedPath)
	return cleanedPath
}

func CSVToURLList(csvPath string) ([]url.URL, error) {
	ctx := context.Background()
	csvFile, err := os.Open(csvPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := csvFile.Close(); closeErr != nil {
			// Log error but don't fail - data has been read
			_ = closeErr
		}
	}()

	csvReader := csv.NewReader(csvFile)
	csvReader.FieldsPerRecord = -1

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	var urls []url.URL
	for _, record := range records {
		if len(record) == 0 || record[0] == "" {
			continue // Skip empty records
		}

		urlStr := record[0]
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error parsing URL", "url", urlStr, "error", err)
			continue // Skip invalid URLs
		}

		// If no scheme is provided, assume https
		if parsedURL.Scheme == "" {
			parsedURL.Scheme = "https"
		}

		urls = append(urls, *parsedURL)
	}
	return urls, nil
}

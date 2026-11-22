package job

import (
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"alt/internal/fsperm"

	rssFeed "github.com/mmcdole/gofeed"
)

const (
	FeedsFilePath = "datastore/output.json"
)

func WriteFeedsToFile(feeds []*rssFeed.Feed) error {
	ctx := context.Background()
	jsonData, err := json.Marshal(feeds)
	if err != nil {
		return fmt.Errorf("marshal feeds: %w", err)
	}

	cleanedPath, err := PathCleaner(FeedsFilePath)
	if err != nil {
		return fmt.Errorf("clean feeds file path: %w", err)
	}
	err = os.WriteFile(cleanedPath, jsonData, fsperm.SecretFile)
	if err != nil {
		return fmt.Errorf("write feeds file: %w", err)
	}
	logger.Logger.InfoContext(ctx, "Feeds written to file", "file", cleanedPath)

	return nil
}

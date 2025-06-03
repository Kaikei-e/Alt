package job

import (
	"alt/utils/logger"
	"encoding/json"
	"os"

	rssFeed "github.com/mmcdole/gofeed"
)

const (
	FeedsFilePath = "datastore/output.json"
)

func WriteFeedsToFile(feeds []*rssFeed.Feed) error {
	jsonData, err := json.Marshal(feeds)
	if err != nil {
		return err
	}

	cleanedPath := PathCleaner(FeedsFilePath)
	err = os.WriteFile(cleanedPath, jsonData, 0644)
	if err != nil {
		return err
	}
	logger.Logger.Info("Feeds written to file", "file", cleanedPath)

	return nil
}

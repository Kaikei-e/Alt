package search_indexer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"alt/driver/models"
)

func SearchArticles(query string) ([]models.SearchArticlesHit, error) {
	host := "search-indexer"
	port := "9300"
	url := fmt.Sprintf("http://%s:%s/v1/search?q=%s", host, port, query)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response models.SearchArticlesAPIResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	results := make([]models.SearchArticlesHit, 0, len(response.Hits))

	for _, hit := range response.Hits {
		results = append(results, models.SearchArticlesHit{
			ID:      hit.ID,
			Title:   hit.Title,
			Content: hit.Content,
		})
	}

	return results, nil
}

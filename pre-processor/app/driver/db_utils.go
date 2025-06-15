package driver

import (
	"fmt"
	"net/url"
)

// convertToURL converts a string to a url.URL
func convertToURL(u string) (url.URL, error) {
	ul, err := url.Parse(u)
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to parse URL: %w", err)
	}

	return *ul, nil
}

package job

import (
	"net/url"
	"strings"

	"github.com/mmcdole/gofeed"
)

// ExtractImageURL extracts the best image URL from a gofeed Item.
// Priority: Item.Image > media:thumbnail > media:content (medium=image) > Enclosure (image/*).
// Only http/https URLs are accepted.
func ExtractImageURL(item *gofeed.Item) string {
	// Priority 1: Item.Image
	if item.Image != nil && item.Image.URL != "" {
		if isValidImageScheme(item.Image.URL) {
			return item.Image.URL
		}
	}

	// Priority 2: media:thumbnail from Extensions
	if mediaExt, ok := item.Extensions["media"]; ok {
		if thumbnails, ok := mediaExt["thumbnail"]; ok {
			for _, thumb := range thumbnails {
				if u := thumb.Attrs["url"]; u != "" && isValidImageScheme(u) {
					return u
				}
			}
		}

		// Priority 3: media:content with medium=image
		if contents, ok := mediaExt["content"]; ok {
			for _, content := range contents {
				if content.Attrs["medium"] == "image" {
					if u := content.Attrs["url"]; u != "" && isValidImageScheme(u) {
						return u
					}
				}
			}
		}
	}

	// Priority 4: Enclosures with image/* type
	for _, enc := range item.Enclosures {
		if strings.HasPrefix(enc.Type, "image/") && enc.URL != "" && isValidImageScheme(enc.URL) {
			return enc.URL
		}
	}

	return ""
}

// isValidImageScheme returns true if the URL has http or https scheme.
func isValidImageScheme(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

package job

import (
	"testing"

	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
)

func TestExtractImageURL_ItemImage(t *testing.T) {
	item := &gofeed.Item{
		Image: &gofeed.Image{URL: "https://example.com/image.jpg"},
	}
	got := ExtractImageURL(item)
	if got != "https://example.com/image.jpg" {
		t.Errorf("got %q, want %q", got, "https://example.com/image.jpg")
	}
}

func TestExtractImageURL_ItemImageHTTP(t *testing.T) {
	item := &gofeed.Item{
		Image: &gofeed.Image{URL: "http://example.com/image.jpg"},
	}
	got := ExtractImageURL(item)
	if got != "http://example.com/image.jpg" {
		t.Errorf("got %q, want %q", got, "http://example.com/image.jpg")
	}
}

func TestExtractImageURL_ItemImageEmpty(t *testing.T) {
	item := &gofeed.Item{
		Image: &gofeed.Image{URL: ""},
	}
	got := ExtractImageURL(item)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractImageURL_NilImage(t *testing.T) {
	item := &gofeed.Item{}
	got := ExtractImageURL(item)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractImageURL_MediaThumbnail(t *testing.T) {
	item := &gofeed.Item{
		Extensions: ext.Extensions{
			"media": {
				"thumbnail": []ext.Extension{
					{Attrs: map[string]string{"url": "https://cdn.example.com/thumb.jpg"}},
				},
			},
		},
	}
	got := ExtractImageURL(item)
	if got != "https://cdn.example.com/thumb.jpg" {
		t.Errorf("got %q, want %q", got, "https://cdn.example.com/thumb.jpg")
	}
}

func TestExtractImageURL_MediaContent(t *testing.T) {
	item := &gofeed.Item{
		Extensions: ext.Extensions{
			"media": {
				"content": []ext.Extension{
					{Attrs: map[string]string{"medium": "image", "url": "https://cdn.example.com/media.png"}},
				},
			},
		},
	}
	got := ExtractImageURL(item)
	if got != "https://cdn.example.com/media.png" {
		t.Errorf("got %q, want %q", got, "https://cdn.example.com/media.png")
	}
}

func TestExtractImageURL_MediaContentNonImage(t *testing.T) {
	item := &gofeed.Item{
		Extensions: ext.Extensions{
			"media": {
				"content": []ext.Extension{
					{Attrs: map[string]string{"medium": "video", "url": "https://cdn.example.com/video.mp4"}},
				},
			},
		},
	}
	got := ExtractImageURL(item)
	if got != "" {
		t.Errorf("got %q, want empty (non-image medium)", got)
	}
}

func TestExtractImageURL_Enclosure(t *testing.T) {
	item := &gofeed.Item{
		Enclosures: []*gofeed.Enclosure{
			{URL: "https://example.com/photo.jpg", Type: "image/jpeg"},
		},
	}
	got := ExtractImageURL(item)
	if got != "https://example.com/photo.jpg" {
		t.Errorf("got %q, want %q", got, "https://example.com/photo.jpg")
	}
}

func TestExtractImageURL_EnclosureNonImage(t *testing.T) {
	item := &gofeed.Item{
		Enclosures: []*gofeed.Enclosure{
			{URL: "https://example.com/audio.mp3", Type: "audio/mpeg"},
		},
	}
	got := ExtractImageURL(item)
	if got != "" {
		t.Errorf("got %q, want empty (non-image enclosure)", got)
	}
}

func TestExtractImageURL_InvalidScheme(t *testing.T) {
	item := &gofeed.Item{
		Image: &gofeed.Image{URL: "data:image/png;base64,abc123"},
	}
	got := ExtractImageURL(item)
	if got != "" {
		t.Errorf("got %q, want empty (data: scheme rejected)", got)
	}
}

func TestExtractImageURL_Priority_ImageOverMedia(t *testing.T) {
	item := &gofeed.Item{
		Image: &gofeed.Image{URL: "https://example.com/primary.jpg"},
		Extensions: ext.Extensions{
			"media": {
				"thumbnail": []ext.Extension{
					{Attrs: map[string]string{"url": "https://example.com/fallback.jpg"}},
				},
			},
		},
	}
	got := ExtractImageURL(item)
	if got != "https://example.com/primary.jpg" {
		t.Errorf("got %q, want %q (Image should take priority over media)", got, "https://example.com/primary.jpg")
	}
}

func TestExtractImageURL_Priority_MediaOverEnclosure(t *testing.T) {
	item := &gofeed.Item{
		Extensions: ext.Extensions{
			"media": {
				"thumbnail": []ext.Extension{
					{Attrs: map[string]string{"url": "https://cdn.example.com/media.jpg"}},
				},
			},
		},
		Enclosures: []*gofeed.Enclosure{
			{URL: "https://example.com/enclosure.jpg", Type: "image/jpeg"},
		},
	}
	got := ExtractImageURL(item)
	if got != "https://cdn.example.com/media.jpg" {
		t.Errorf("got %q, want %q (media should take priority over enclosure)", got, "https://cdn.example.com/media.jpg")
	}
}

package opml_usecase

import (
	"alt/domain"
	"alt/port/opml_port"
	"alt/utils"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"time"
)

type ExportOPMLUsecase struct {
	exportPort opml_port.ExportOPMLPort
}

func NewExportOPMLUsecase(exportPort opml_port.ExportOPMLPort) *ExportOPMLUsecase {
	return &ExportOPMLUsecase{exportPort: exportPort}
}

// Execute fetches all feed links and generates OPML 2.0 XML.
func (u *ExportOPMLUsecase) Execute(ctx context.Context) ([]byte, error) {
	links, err := u.exportPort.FetchFeedLinksForExport(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch feed links for export: %w", err)
	}

	links = deduplicateLinks(links)
	doc := buildOPMLDocument(links)
	return marshalOPML(doc)
}

// deduplicateLinks strips tracking parameters and removes duplicates that
// differ only by tracking params. The first occurrence wins.
func deduplicateLinks(links []*domain.FeedLinkForExport) []*domain.FeedLinkForExport {
	seen := make(map[string]struct{})
	result := make([]*domain.FeedLinkForExport, 0, len(links))
	for _, link := range links {
		cleaned, err := utils.StripTrackingParams(link.URL)
		if err != nil {
			cleaned = link.URL
		}
		if _, exists := seen[cleaned]; !exists {
			seen[cleaned] = struct{}{}
			result = append(result, &domain.FeedLinkForExport{
				URL:   cleaned,
				Title: link.Title,
			})
		}
	}
	return result
}

func buildOPMLDocument(links []*domain.FeedLinkForExport) *opmlXML {
	outlines := make([]outlineXML, 0, len(links))
	for _, link := range links {
		outlines = append(outlines, outlineXML{
			Text:   html.UnescapeString(link.Title),
			Type:   "rss",
			XMLURL: link.URL,
		})
	}

	return &opmlXML{
		Version: "2.0",
		Head: headXML{
			Title:       "Alt RSS Feeds",
			DateCreated: time.Now().UTC().Format(time.RFC1123Z),
		},
		Body: bodyXML{
			Outlines: outlines,
		},
	}
}

func marshalOPML(doc *opmlXML) ([]byte, error) {
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal OPML XML: %w", err)
	}
	return append([]byte(xml.Header), data...), nil
}

// XML structures for OPML 2.0 serialization.

type opmlXML struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    headXML  `xml:"head"`
	Body    bodyXML  `xml:"body"`
}

type headXML struct {
	Title       string `xml:"title"`
	DateCreated string `xml:"dateCreated,omitempty"`
}

type bodyXML struct {
	Outlines []outlineXML `xml:"outline"`
}

type outlineXML struct {
	Text     string       `xml:"text,attr"`
	Type     string       `xml:"type,attr,omitempty"`
	XMLURL   string       `xml:"xmlUrl,attr,omitempty"`
	HTMLURL  string       `xml:"htmlUrl,attr,omitempty"`
	Children []outlineXML `xml:"outline,omitempty"`
}

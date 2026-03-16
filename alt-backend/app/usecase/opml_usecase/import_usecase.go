package opml_usecase

import (
	"alt/domain"
	"alt/port/opml_port"
	"context"
	"encoding/xml"
	"fmt"
)

type ImportOPMLUsecase struct {
	importPort opml_port.ImportOPMLPort
}

func NewImportOPMLUsecase(importPort opml_port.ImportOPMLPort) *ImportOPMLUsecase {
	return &ImportOPMLUsecase{importPort: importPort}
}

// Execute parses OPML XML and registers all feed URLs.
func (u *ImportOPMLUsecase) Execute(ctx context.Context, xmlData []byte) (*domain.OPMLImportResult, error) {
	doc, err := parseOPML(xmlData)
	if err != nil {
		return nil, fmt.Errorf("parse OPML: %w", err)
	}

	urls := extractFeedURLs(doc)
	if len(urls) == 0 {
		return &domain.OPMLImportResult{}, nil
	}

	return u.importPort.RegisterFeedLinkBulk(ctx, urls)
}

func parseOPML(data []byte) (*opmlXML, error) {
	var doc opmlXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal OPML XML: %w", err)
	}
	return &doc, nil
}

// extractFeedURLs flattens the OPML outline tree and extracts all RSS feed URLs.
func extractFeedURLs(doc *opmlXML) []string {
	var urls []string
	seen := make(map[string]struct{})

	var walk func(outlines []outlineXML)
	walk = func(outlines []outlineXML) {
		for _, o := range outlines {
			if o.XMLURL != "" {
				if _, exists := seen[o.XMLURL]; !exists {
					seen[o.XMLURL] = struct{}{}
					urls = append(urls, o.XMLURL)
				}
			}
			if len(o.Children) > 0 {
				walk(o.Children)
			}
		}
	}

	walk(doc.Body.Outlines)
	return urls
}

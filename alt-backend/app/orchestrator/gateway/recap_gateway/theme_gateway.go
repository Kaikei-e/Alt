package recap_gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"alt/domain"
)

type themeSentenceWire struct {
	Text             string   `json:"text"`
	SourceArticleIDs []string `json:"source_article_ids"`
}

type themeTimelineEntryWire struct {
	Date             string   `json:"date"`
	Text             string   `json:"text"`
	SourceArticleIDs []string `json:"source_article_ids"`
}

type themeSourceWire struct {
	ArticleID string `json:"article_id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
}

type themeVersionRefWire struct {
	Version      int32  `json:"version"`
	CreatedAt    string `json:"created_at"`
	ArticleCount int32  `json:"article_count"`
}

type themePageWire struct {
	ThemeID       string                   `json:"theme_id"`
	Title         string                   `json:"title"`
	Version       int32                    `json:"version"`
	LatestVersion int32                    `json:"latest_version"`
	CreatedAt     string                   `json:"created_at"`
	KeyPoints     []themeSentenceWire      `json:"key_points"`
	Timeline      []themeTimelineEntryWire `json:"timeline"`
	WhatChanged   []themeSentenceWire      `json:"what_changed"`
	OpenQuestions []themeSentenceWire      `json:"open_questions"`
	Sources       []themeSourceWire        `json:"sources"`
	Versions      []themeVersionRefWire    `json:"versions"`
}

type themeErrorWire struct {
	Error string `json:"error"`
}

func (g *RecapGateway) GetTheme(
	ctx context.Context,
	userID uuid.UUID,
	themeID uuid.UUID,
	version *int32,
) (*domain.ReadingThemePage, error) {
	reqURL, err := buildThemeURL(g.recapWorkerURL, userID, themeID, version)
	if err != nil {
		return nil, fmt.Errorf("build theme url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch theme from recap-worker: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		msg := readThemeErrorMessage(resp.Body)
		if msg == "" {
			return nil, domain.ErrThemeNotFound
		}
		return nil, fmt.Errorf("%w: %s", domain.ErrThemeNotFound, msg)
	}

	if resp.StatusCode == http.StatusBadRequest {
		msg := readThemeErrorMessage(resp.Body)
		if msg == "" {
			msg = "invalid theme request"
		}
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidThemeInput, msg)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(body))
	}

	var wire themePageWire
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return nil, fmt.Errorf("failed to decode theme response: %w", err)
	}

	return mapThemePageWire(wire)
}

func buildThemeURL(baseURL string, userID uuid.UUID, themeID uuid.UUID, version *int32) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v1/themes/" + url.PathEscape(themeID.String())
	q := parsed.Query()
	q.Set("user_id", userID.String())
	if version != nil {
		q.Set("version", strconv.FormatInt(int64(*version), 10))
	}
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

func readThemeErrorMessage(body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, 4096))
	if err != nil || len(raw) == 0 {
		return ""
	}
	var errWire themeErrorWire
	if err := json.Unmarshal(raw, &errWire); err != nil {
		return ""
	}
	return strings.TrimSpace(errWire.Error)
}

func mapThemePageWire(w themePageWire) (*domain.ReadingThemePage, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, w.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse theme created_at %q: %w", w.CreatedAt, err)
	}

	versions := make([]domain.ThemeVersionRef, 0, len(w.Versions))
	for _, v := range w.Versions {
		vCreatedAt, err := time.Parse(time.RFC3339Nano, v.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse theme version %d created_at %q: %w", v.Version, v.CreatedAt, err)
		}
		versions = append(versions, domain.ThemeVersionRef{
			Version:      v.Version,
			CreatedAt:    vCreatedAt,
			ArticleCount: v.ArticleCount,
		})
	}

	timeline := make([]domain.ThemeTimelineEntry, 0, len(w.Timeline))
	for _, t := range w.Timeline {
		timeline = append(timeline, domain.ThemeTimelineEntry{
			Date:             t.Date,
			Text:             t.Text,
			SourceArticleIDs: t.SourceArticleIDs,
		})
	}

	sources := make([]domain.ThemeSource, 0, len(w.Sources))
	for _, s := range w.Sources {
		sources = append(sources, domain.ThemeSource{
			ArticleID: s.ArticleID,
			Title:     s.Title,
			URL:       s.URL,
		})
	}

	return &domain.ReadingThemePage{
		ThemeID:       w.ThemeID,
		Title:         w.Title,
		Version:       w.Version,
		LatestVersion: w.LatestVersion,
		CreatedAt:     createdAt,
		KeyPoints:     mapSentencesWire(w.KeyPoints),
		Timeline:      timeline,
		WhatChanged:   mapSentencesWire(w.WhatChanged),
		OpenQuestions: mapSentencesWire(w.OpenQuestions),
		Sources:       sources,
		Versions:      versions,
	}, nil
}

func mapSentencesWire(in []themeSentenceWire) []domain.ThemeSentence {
	out := make([]domain.ThemeSentence, 0, len(in))
	for _, s := range in {
		out = append(out, domain.ThemeSentence{
			Text:             s.Text,
			SourceArticleIDs: s.SourceArticleIDs,
		})
	}
	return out
}

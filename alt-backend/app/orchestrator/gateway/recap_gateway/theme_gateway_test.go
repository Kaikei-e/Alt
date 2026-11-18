package recap_gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

func TestRecapGateway_GetTheme(t *testing.T) {
	userID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	themeID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	t.Run("success without version", func(t *testing.T) {
		payload := map[string]any{
			"theme_id":       themeID.String(),
			"title":          "Systems Architecture",
			"version":        2,
			"latest_version": 2,
			"created_at":     "2026-11-18T08:00:00Z",
			"key_points": []map[string]any{
				{"text": "Point one.", "source_article_ids": []string{"33333333-3333-3333-3333-333333333333"}},
			},
			"timeline": []map[string]any{
				{"date": "2026-11-15", "text": "Started.", "source_article_ids": []string{"33333333-3333-3333-3333-333333333333"}},
			},
			"what_changed": []map[string]any{
				{"text": "Updated.", "source_article_ids": []string{"33333333-3333-3333-3333-333333333333"}},
			},
			"open_questions": []map[string]any{
				{"text": "What next?", "source_article_ids": []string{"33333333-3333-3333-3333-333333333333"}},
			},
			"sources": []map[string]any{
				{"article_id": "33333333-3333-3333-3333-333333333333", "title": "Src 1", "url": "https://example.com/1"},
			},
			"versions": []map[string]any{
				{"version": 2, "created_at": "2026-11-18T08:00:00Z", "article_count": 5},
				{"version": 1, "created_at": "2026-11-17T08:00:00Z", "article_count": 3},
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "/v1/themes/"+themeID.String(), r.URL.Path)
			assert.Equal(t, userID.String(), r.URL.Query().Get("user_id"))
			assert.False(t, r.URL.Query().Has("version"))
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(payload))
		}))
		defer srv.Close()

		gw := NewRecapGatewayWithConfig(nil, srv.URL)
		got, err := gw.GetTheme(context.Background(), userID, themeID, nil)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, themeID.String(), got.ThemeID)
		assert.Equal(t, "Systems Architecture", got.Title)
		assert.Equal(t, int32(2), got.Version)
		assert.Equal(t, int32(2), got.LatestVersion)
		assert.Equal(t, time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC), got.CreatedAt)
		require.Len(t, got.KeyPoints, 1)
		assert.Equal(t, "Point one.", got.KeyPoints[0].Text)
		require.Len(t, got.Timeline, 1)
		assert.Equal(t, "2026-11-15", got.Timeline[0].Date)
		require.Len(t, got.WhatChanged, 1)
		require.Len(t, got.OpenQuestions, 1)
		require.Len(t, got.Sources, 1)
		assert.Equal(t, "https://example.com/1", got.Sources[0].URL)
		require.Len(t, got.Versions, 2)
		assert.Equal(t, int32(2), got.Versions[0].Version)
		assert.Equal(t, int32(5), got.Versions[0].ArticleCount)
	})

	t.Run("success with explicit version", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "1", r.URL.Query().Get("version"))
			assert.Equal(t, userID.String(), r.URL.Query().Get("user_id"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"theme_id":"44444444-4444-4444-4444-444444444444","title":"Systems","version":1,"latest_version":2,"created_at":"2026-11-17T08:00:00Z","key_points":[],"timeline":[],"what_changed":[],"open_questions":[],"sources":[],"versions":[]}`))
		}))
		defer srv.Close()

		gw := NewRecapGatewayWithConfig(nil, srv.URL)
		v := int32(1)
		got, err := gw.GetTheme(context.Background(), userID, themeID, &v)
		require.NoError(t, err)
		assert.Equal(t, int32(1), got.Version)
		assert.Equal(t, int32(2), got.LatestVersion)
	})

	t.Run("404 maps to ErrThemeNotFound", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"theme not found"}`))
		}))
		defer srv.Close()

		gw := NewRecapGatewayWithConfig(nil, srv.URL)
		_, err := gw.GetTheme(context.Background(), userID, themeID, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrThemeNotFound)
	})

	t.Run("400 maps to ErrInvalidThemeInput", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid version"}`))
		}))
		defer srv.Close()

		gw := NewRecapGatewayWithConfig(nil, srv.URL)
		_, err := gw.GetTheme(context.Background(), userID, themeID, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrInvalidThemeInput)
	})

	t.Run("500 returns upstream status error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		}))
		defer srv.Close()

		gw := NewRecapGatewayWithConfig(nil, srv.URL)
		_, err := gw.GetTheme(context.Background(), userID, themeID, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recap-worker returned status 500")
	})
}

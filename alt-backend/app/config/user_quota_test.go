package config

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadUserQuotaConfig_Defaults(t *testing.T) {
	cfg, err := LoadUserQuotaConfig()
	require.NoError(t, err)

	assert.Equal(t, DefaultUserQuotaMaxTrackedUsers, cfg.MaxTrackedUsers)

	assert.Equal(t, DefaultUserQuotaSummarizeRate, cfg.Summarize.Rate)
	assert.Equal(t, DefaultUserQuotaSummarizeBurst, cfg.Summarize.Burst)
	assert.Equal(t, DefaultUserQuotaSummarizeWindow, cfg.Summarize.Window)
	assert.Equal(t, 2, cfg.Summarize.MaxInFlight)

	assert.Equal(t, DefaultUserQuotaAugurChatRate, cfg.AugurChat.Rate)
	assert.Equal(t, DefaultUserQuotaAugurChatBurst, cfg.AugurChat.Burst)
	assert.Equal(t, DefaultUserQuotaAugurChatWindow, cfg.AugurChat.Window)
	assert.Equal(t, 1, cfg.AugurChat.MaxInFlight)

	assert.Equal(t, DefaultUserQuotaMorningChatRate, cfg.MorningChat.Rate)
	assert.Equal(t, DefaultUserQuotaMorningChatBurst, cfg.MorningChat.Burst)
	assert.Equal(t, DefaultUserQuotaMorningChatWindow, cfg.MorningChat.Window)
	assert.Equal(t, 1, cfg.MorningChat.MaxInFlight)

	assert.Equal(t, OperationQuotaConfig{Rate: 30, Burst: 15, Window: time.Minute, MaxInFlight: 2}, cfg.AugurRetrieve)
	assert.Equal(t, OperationQuotaConfig{Rate: 120, Burst: 60, Window: time.Minute, MaxInFlight: 40}, cfg.ArticleTags)
}

func TestLoadUserQuotaConfig_OverridesAndRejectsInvalid(t *testing.T) {
	t.Run("overrides from environment", func(t *testing.T) {
		t.Setenv(UserQuotaMaxTrackedUsersEnv, "512")
		t.Setenv(UserQuotaSummarizeRateEnv, "5")
		t.Setenv(UserQuotaSummarizeBurstEnv, "3")
		t.Setenv(UserQuotaSummarizeWindowEnv, "30s")
		t.Setenv(UserQuotaSummarizeMaxInFlightEnv, "4")

		cfg, err := LoadUserQuotaConfig()
		require.NoError(t, err)
		assert.Equal(t, 512, cfg.MaxTrackedUsers)
		assert.Equal(t, OperationQuotaConfig{
			Rate:        5,
			Burst:       3,
			Window:      30 * time.Second,
			MaxInFlight: 4,
		}, cfg.Summarize)
	})

	t.Run("overrides retrieve and tag classes", func(t *testing.T) {
		t.Setenv("USER_QUOTA_AUGUR_RETRIEVE_RATE", "7")
		t.Setenv("USER_QUOTA_AUGUR_RETRIEVE_BURST", "3")
		t.Setenv("USER_QUOTA_AUGUR_RETRIEVE_WINDOW", "2m")
		t.Setenv("USER_QUOTA_AUGUR_RETRIEVE_MAX_IN_FLIGHT", "5")
		t.Setenv("USER_QUOTA_ARTICLE_TAGS_RATE", "9")
		t.Setenv("USER_QUOTA_ARTICLE_TAGS_BURST", "8")
		t.Setenv("USER_QUOTA_ARTICLE_TAGS_WINDOW", "30s")
		t.Setenv("USER_QUOTA_ARTICLE_TAGS_MAX_IN_FLIGHT", "6")

		cfg, err := LoadUserQuotaConfig()
		require.NoError(t, err)
		assert.Equal(t, OperationQuotaConfig{Rate: 7, Burst: 3, Window: 2 * time.Minute, MaxInFlight: 5}, cfg.AugurRetrieve)
		assert.Equal(t, OperationQuotaConfig{Rate: 9, Burst: 8, Window: 30 * time.Second, MaxInFlight: 6}, cfg.ArticleTags)
	})

	t.Run("rejects zero or negative values", func(t *testing.T) {
		t.Setenv(UserQuotaAugurChatBurstEnv, "0")
		_, err := LoadUserQuotaConfig()
		require.Error(t, err)
	})

	t.Run("rejects malformed duration", func(t *testing.T) {
		t.Setenv(UserQuotaMorningChatWindowEnv, "not-a-duration")
		_, err := LoadUserQuotaConfig()
		require.Error(t, err)
	})

	t.Run("rejects malformed integer", func(t *testing.T) {
		t.Setenv(UserQuotaMorningChatMaxInFlightEnv, "abc")
		_, err := LoadUserQuotaConfig()
		require.Error(t, err)
	})
}

func TestLogUserQuotaConfig_EmitsStructuredValues(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	LogUserQuotaConfig(logger, DefaultUserQuotaConfig())

	out := buf.String()
	assert.Contains(t, out, `"msg":"user_quota_configured"`)
	assert.Contains(t, out, `"enabled":true`)
	assert.Contains(t, out, `"max_tracked_users":10000`)
	assert.Contains(t, out, `"summarize_max_in_flight":2`)
	assert.Contains(t, out, `"augur_chat_max_in_flight":1`)
	assert.Contains(t, out, `"morning_chat_max_in_flight":1`)
	assert.Contains(t, out, `"augur_retrieve_max_in_flight":2`)
	assert.Contains(t, out, `"article_tags_max_in_flight":40`)
	assert.NotContains(t, out, `morning_regenerate`)
	assert.NotContains(t, out, `max_concurrent`)
}

func TestLoadUserQuotaConfig_RejectsSetButEmptyValues(t *testing.T) {
	for _, name := range []string{
		UserQuotaMaxTrackedUsersEnv,
		UserQuotaSummarizeRateEnv,
		UserQuotaSummarizeBurstEnv,
		UserQuotaSummarizeWindowEnv,
		UserQuotaSummarizeMaxInFlightEnv,
		UserQuotaAugurChatRateEnv,
		UserQuotaMorningChatWindowEnv,
		"USER_QUOTA_AUGUR_RETRIEVE_BURST",
		"USER_QUOTA_ARTICLE_TAGS_MAX_IN_FLIGHT",
	} {
		for _, value := range []string{"", "   "} {
			t.Run(name+"="+value, func(t *testing.T) {
				t.Setenv(name, value)
				_, err := LoadUserQuotaConfig()
				require.Error(t, err)
				assert.Contains(t, err.Error(), name)
			})
		}
	}
}

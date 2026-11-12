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
	assert.Equal(t, DefaultUserQuotaSummarizeMaxConcurrent, cfg.Summarize.MaxConcurrent)

	assert.Equal(t, DefaultUserQuotaAugurChatRate, cfg.AugurChat.Rate)
	assert.Equal(t, DefaultUserQuotaAugurChatBurst, cfg.AugurChat.Burst)
	assert.Equal(t, DefaultUserQuotaAugurChatWindow, cfg.AugurChat.Window)
	assert.Equal(t, DefaultUserQuotaAugurChatMaxConcurrent, cfg.AugurChat.MaxConcurrent)

	assert.Equal(t, DefaultUserQuotaMorningChatRate, cfg.MorningChat.Rate)
	assert.Equal(t, DefaultUserQuotaMorningChatBurst, cfg.MorningChat.Burst)
	assert.Equal(t, DefaultUserQuotaMorningChatWindow, cfg.MorningChat.Window)
	assert.Equal(t, DefaultUserQuotaMorningChatMaxConcurrent, cfg.MorningChat.MaxConcurrent)

	assert.Equal(t, DefaultUserQuotaMorningRegenerateRate, cfg.MorningRegenerate.Rate)
	assert.Equal(t, DefaultUserQuotaMorningRegenerateBurst, cfg.MorningRegenerate.Burst)
	assert.Equal(t, DefaultUserQuotaMorningRegenerateWindow, cfg.MorningRegenerate.Window)
	assert.Equal(t, DefaultUserQuotaMorningRegenerateMaxConcurrent, cfg.MorningRegenerate.MaxConcurrent)
}

func TestLoadUserQuotaConfig_OverridesAndRejectsInvalid(t *testing.T) {
	t.Run("overrides from environment", func(t *testing.T) {
		t.Setenv(UserQuotaMaxTrackedUsersEnv, "512")
		t.Setenv(UserQuotaSummarizeRateEnv, "5")
		t.Setenv(UserQuotaSummarizeBurstEnv, "3")
		t.Setenv(UserQuotaSummarizeWindowEnv, "30s")
		t.Setenv(UserQuotaSummarizeMaxConcurrentEnv, "2")

		cfg, err := LoadUserQuotaConfig()
		require.NoError(t, err)
		assert.Equal(t, 512, cfg.MaxTrackedUsers)
		assert.Equal(t, OperationQuotaConfig{
			Rate:          5,
			Burst:         3,
			Window:        30 * time.Second,
			MaxConcurrent: 2,
		}, cfg.Summarize)
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
		t.Setenv(UserQuotaMorningRegenerateMaxConcurrentEnv, "abc")
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
	assert.Contains(t, out, `"summarize_max_concurrent":4`)
	assert.Contains(t, out, `"augur_chat_max_concurrent":2`)
	assert.Contains(t, out, `"morning_chat_max_concurrent":2`)
	assert.Contains(t, out, `"morning_regenerate_max_concurrent":1`)
}

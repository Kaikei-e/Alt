package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	UserQuotaMaxTrackedUsersEnv = "USER_QUOTA_MAX_TRACKED_USERS"

	UserQuotaSummarizeRateEnv          = "USER_QUOTA_SUMMARIZE_RATE"
	UserQuotaSummarizeBurstEnv         = "USER_QUOTA_SUMMARIZE_BURST"
	UserQuotaSummarizeWindowEnv        = "USER_QUOTA_SUMMARIZE_WINDOW"
	UserQuotaSummarizeMaxConcurrentEnv = "USER_QUOTA_SUMMARIZE_MAX_CONCURRENT"

	UserQuotaAugurChatRateEnv          = "USER_QUOTA_AUGUR_CHAT_RATE"
	UserQuotaAugurChatBurstEnv         = "USER_QUOTA_AUGUR_CHAT_BURST"
	UserQuotaAugurChatWindowEnv        = "USER_QUOTA_AUGUR_CHAT_WINDOW"
	UserQuotaAugurChatMaxConcurrentEnv = "USER_QUOTA_AUGUR_CHAT_MAX_CONCURRENT"

	UserQuotaMorningChatRateEnv          = "USER_QUOTA_MORNING_CHAT_RATE"
	UserQuotaMorningChatBurstEnv         = "USER_QUOTA_MORNING_CHAT_BURST"
	UserQuotaMorningChatWindowEnv        = "USER_QUOTA_MORNING_CHAT_WINDOW"
	UserQuotaMorningChatMaxConcurrentEnv = "USER_QUOTA_MORNING_CHAT_MAX_CONCURRENT"

	UserQuotaMorningRegenerateRateEnv          = "USER_QUOTA_MORNING_REGENERATE_RATE"
	UserQuotaMorningRegenerateBurstEnv         = "USER_QUOTA_MORNING_REGENERATE_BURST"
	UserQuotaMorningRegenerateWindowEnv        = "USER_QUOTA_MORNING_REGENERATE_WINDOW"
	UserQuotaMorningRegenerateMaxConcurrentEnv = "USER_QUOTA_MORNING_REGENERATE_MAX_CONCURRENT"

	DefaultUserQuotaMaxTrackedUsers = 10000

	DefaultUserQuotaSummarizeRate          = 20
	DefaultUserQuotaSummarizeBurst         = 10
	DefaultUserQuotaSummarizeWindow        = time.Minute
	DefaultUserQuotaSummarizeMaxConcurrent = 4

	DefaultUserQuotaAugurChatRate          = 12
	DefaultUserQuotaAugurChatBurst         = 6
	DefaultUserQuotaAugurChatWindow        = time.Minute
	DefaultUserQuotaAugurChatMaxConcurrent = 2

	DefaultUserQuotaMorningChatRate          = 12
	DefaultUserQuotaMorningChatBurst         = 6
	DefaultUserQuotaMorningChatWindow        = time.Minute
	DefaultUserQuotaMorningChatMaxConcurrent = 2

	DefaultUserQuotaMorningRegenerateRate          = 2
	DefaultUserQuotaMorningRegenerateBurst         = 2
	DefaultUserQuotaMorningRegenerateWindow        = time.Hour
	DefaultUserQuotaMorningRegenerateMaxConcurrent = 1
)

type OperationQuotaConfig struct {
	Rate          int
	Burst         int
	Window        time.Duration
	MaxConcurrent int
}

type UserQuotaConfig struct {
	MaxTrackedUsers   int
	Summarize         OperationQuotaConfig
	AugurChat         OperationQuotaConfig
	MorningChat       OperationQuotaConfig
	MorningRegenerate OperationQuotaConfig
}

func DefaultUserQuotaConfig() UserQuotaConfig {
	return UserQuotaConfig{
		MaxTrackedUsers: DefaultUserQuotaMaxTrackedUsers,
		Summarize: OperationQuotaConfig{
			Rate:          DefaultUserQuotaSummarizeRate,
			Burst:         DefaultUserQuotaSummarizeBurst,
			Window:        DefaultUserQuotaSummarizeWindow,
			MaxConcurrent: DefaultUserQuotaSummarizeMaxConcurrent,
		},
		AugurChat: OperationQuotaConfig{
			Rate:          DefaultUserQuotaAugurChatRate,
			Burst:         DefaultUserQuotaAugurChatBurst,
			Window:        DefaultUserQuotaAugurChatWindow,
			MaxConcurrent: DefaultUserQuotaAugurChatMaxConcurrent,
		},
		MorningChat: OperationQuotaConfig{
			Rate:          DefaultUserQuotaMorningChatRate,
			Burst:         DefaultUserQuotaMorningChatBurst,
			Window:        DefaultUserQuotaMorningChatWindow,
			MaxConcurrent: DefaultUserQuotaMorningChatMaxConcurrent,
		},
		MorningRegenerate: OperationQuotaConfig{
			Rate:          DefaultUserQuotaMorningRegenerateRate,
			Burst:         DefaultUserQuotaMorningRegenerateBurst,
			Window:        DefaultUserQuotaMorningRegenerateWindow,
			MaxConcurrent: DefaultUserQuotaMorningRegenerateMaxConcurrent,
		},
	}
}

func LoadUserQuotaConfig() (UserQuotaConfig, error) {
	cfg := DefaultUserQuotaConfig()

	var err error
	if cfg.MaxTrackedUsers, err = loadPositiveIntEnv(UserQuotaMaxTrackedUsersEnv, cfg.MaxTrackedUsers); err != nil {
		return UserQuotaConfig{}, err
	}
	if cfg.Summarize, err = loadOperationQuotaEnv(
		cfg.Summarize,
		UserQuotaSummarizeRateEnv,
		UserQuotaSummarizeBurstEnv,
		UserQuotaSummarizeWindowEnv,
		UserQuotaSummarizeMaxConcurrentEnv,
	); err != nil {
		return UserQuotaConfig{}, err
	}
	if cfg.AugurChat, err = loadOperationQuotaEnv(
		cfg.AugurChat,
		UserQuotaAugurChatRateEnv,
		UserQuotaAugurChatBurstEnv,
		UserQuotaAugurChatWindowEnv,
		UserQuotaAugurChatMaxConcurrentEnv,
	); err != nil {
		return UserQuotaConfig{}, err
	}
	if cfg.MorningChat, err = loadOperationQuotaEnv(
		cfg.MorningChat,
		UserQuotaMorningChatRateEnv,
		UserQuotaMorningChatBurstEnv,
		UserQuotaMorningChatWindowEnv,
		UserQuotaMorningChatMaxConcurrentEnv,
	); err != nil {
		return UserQuotaConfig{}, err
	}
	if cfg.MorningRegenerate, err = loadOperationQuotaEnv(
		cfg.MorningRegenerate,
		UserQuotaMorningRegenerateRateEnv,
		UserQuotaMorningRegenerateBurstEnv,
		UserQuotaMorningRegenerateWindowEnv,
		UserQuotaMorningRegenerateMaxConcurrentEnv,
	); err != nil {
		return UserQuotaConfig{}, err
	}

	if err := ValidateUserQuotaConfig(cfg); err != nil {
		return UserQuotaConfig{}, err
	}
	return cfg, nil
}

func ValidateUserQuotaConfig(cfg UserQuotaConfig) error {
	if cfg.MaxTrackedUsers <= 0 {
		return fmt.Errorf("%s must be > 0, got %d", UserQuotaMaxTrackedUsersEnv, cfg.MaxTrackedUsers)
	}
	checks := []struct {
		name string
		op   OperationQuotaConfig
	}{
		{"summarize", cfg.Summarize},
		{"augur_chat", cfg.AugurChat},
		{"morning_chat", cfg.MorningChat},
		{"morning_regenerate", cfg.MorningRegenerate},
	}
	for _, c := range checks {
		if c.op.Rate <= 0 {
			return fmt.Errorf("user quota %s rate must be > 0, got %d", c.name, c.op.Rate)
		}
		if c.op.Burst <= 0 {
			return fmt.Errorf("user quota %s burst must be > 0, got %d", c.name, c.op.Burst)
		}
		if c.op.Window <= 0 {
			return fmt.Errorf("user quota %s window must be > 0, got %s", c.name, c.op.Window)
		}
		if c.op.MaxConcurrent <= 0 {
			return fmt.Errorf("user quota %s max_concurrent must be > 0, got %d", c.name, c.op.MaxConcurrent)
		}
	}
	return nil
}

func LogUserQuotaConfig(logger *slog.Logger, cfg UserQuotaConfig) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("user_quota_configured",
		"enabled", true,
		"max_tracked_users", cfg.MaxTrackedUsers,
		"summarize_rate", cfg.Summarize.Rate,
		"summarize_burst", cfg.Summarize.Burst,
		"summarize_window", cfg.Summarize.Window.String(),
		"summarize_max_concurrent", cfg.Summarize.MaxConcurrent,
		"augur_chat_rate", cfg.AugurChat.Rate,
		"augur_chat_burst", cfg.AugurChat.Burst,
		"augur_chat_window", cfg.AugurChat.Window.String(),
		"augur_chat_max_concurrent", cfg.AugurChat.MaxConcurrent,
		"morning_chat_rate", cfg.MorningChat.Rate,
		"morning_chat_burst", cfg.MorningChat.Burst,
		"morning_chat_window", cfg.MorningChat.Window.String(),
		"morning_chat_max_concurrent", cfg.MorningChat.MaxConcurrent,
		"morning_regenerate_rate", cfg.MorningRegenerate.Rate,
		"morning_regenerate_burst", cfg.MorningRegenerate.Burst,
		"morning_regenerate_window", cfg.MorningRegenerate.Window.String(),
		"morning_regenerate_max_concurrent", cfg.MorningRegenerate.MaxConcurrent,
	)
}

func loadOperationQuotaEnv(
	defaults OperationQuotaConfig,
	rateEnv, burstEnv, windowEnv, maxConcEnv string,
) (OperationQuotaConfig, error) {
	out := defaults
	var err error
	if out.Rate, err = loadPositiveIntEnv(rateEnv, out.Rate); err != nil {
		return OperationQuotaConfig{}, err
	}
	if out.Burst, err = loadPositiveIntEnv(burstEnv, out.Burst); err != nil {
		return OperationQuotaConfig{}, err
	}
	if out.Window, err = loadPositiveDurationEnv(windowEnv, out.Window); err != nil {
		return OperationQuotaConfig{}, err
	}
	if out.MaxConcurrent, err = loadPositiveIntEnv(maxConcEnv, out.MaxConcurrent); err != nil {
		return OperationQuotaConfig{}, err
	}
	return out, nil
}

func loadPositiveIntEnv(name string, defaultVal int) (int, error) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return defaultVal, nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("%s must not be empty when set", name)
	}
	v, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s=%q: %w", name, raw, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("%s must be > 0, got %d", name, v)
	}
	return v, nil
}

func loadPositiveDurationEnv(name string, defaultVal time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return defaultVal, nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("%s must not be empty when set", name)
	}
	d, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s=%q: %w", name, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be > 0, got %s", name, d)
	}
	return d, nil
}

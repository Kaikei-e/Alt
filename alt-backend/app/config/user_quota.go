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

	UserQuotaSummarizeRateEnv        = "USER_QUOTA_SUMMARIZE_RATE"
	UserQuotaSummarizeBurstEnv       = "USER_QUOTA_SUMMARIZE_BURST"
	UserQuotaSummarizeWindowEnv      = "USER_QUOTA_SUMMARIZE_WINDOW"
	UserQuotaSummarizeMaxInFlightEnv = "USER_QUOTA_SUMMARIZE_MAX_IN_FLIGHT"

	UserQuotaAugurChatRateEnv        = "USER_QUOTA_AUGUR_CHAT_RATE"
	UserQuotaAugurChatBurstEnv       = "USER_QUOTA_AUGUR_CHAT_BURST"
	UserQuotaAugurChatWindowEnv      = "USER_QUOTA_AUGUR_CHAT_WINDOW"
	UserQuotaAugurChatMaxInFlightEnv = "USER_QUOTA_AUGUR_CHAT_MAX_IN_FLIGHT"

	UserQuotaMorningChatRateEnv        = "USER_QUOTA_MORNING_CHAT_RATE"
	UserQuotaMorningChatBurstEnv       = "USER_QUOTA_MORNING_CHAT_BURST"
	UserQuotaMorningChatWindowEnv      = "USER_QUOTA_MORNING_CHAT_WINDOW"
	UserQuotaMorningChatMaxInFlightEnv = "USER_QUOTA_MORNING_CHAT_MAX_IN_FLIGHT"

	UserQuotaAugurRetrieveRateEnv        = "USER_QUOTA_AUGUR_RETRIEVE_RATE"
	UserQuotaAugurRetrieveBurstEnv       = "USER_QUOTA_AUGUR_RETRIEVE_BURST"
	UserQuotaAugurRetrieveWindowEnv      = "USER_QUOTA_AUGUR_RETRIEVE_WINDOW"
	UserQuotaAugurRetrieveMaxInFlightEnv = "USER_QUOTA_AUGUR_RETRIEVE_MAX_IN_FLIGHT"

	UserQuotaArticleTagsRateEnv        = "USER_QUOTA_ARTICLE_TAGS_RATE"
	UserQuotaArticleTagsBurstEnv       = "USER_QUOTA_ARTICLE_TAGS_BURST"
	UserQuotaArticleTagsWindowEnv      = "USER_QUOTA_ARTICLE_TAGS_WINDOW"
	UserQuotaArticleTagsMaxInFlightEnv = "USER_QUOTA_ARTICLE_TAGS_MAX_IN_FLIGHT"

	DefaultUserQuotaMaxTrackedUsers = 10000

	DefaultUserQuotaSummarizeRate        = 20
	DefaultUserQuotaSummarizeBurst       = 10
	DefaultUserQuotaSummarizeWindow      = time.Minute
	DefaultUserQuotaSummarizeMaxInFlight = 2

	DefaultUserQuotaAugurChatRate        = 12
	DefaultUserQuotaAugurChatBurst       = 6
	DefaultUserQuotaAugurChatWindow      = time.Minute
	DefaultUserQuotaAugurChatMaxInFlight = 1

	DefaultUserQuotaMorningChatRate        = 12
	DefaultUserQuotaMorningChatBurst       = 6
	DefaultUserQuotaMorningChatWindow      = time.Minute
	DefaultUserQuotaMorningChatMaxInFlight = 1

	DefaultUserQuotaAugurRetrieveRate        = 30
	DefaultUserQuotaAugurRetrieveBurst       = 15
	DefaultUserQuotaAugurRetrieveWindow      = time.Minute
	DefaultUserQuotaAugurRetrieveMaxInFlight = 2

	// One tag-trail page view opens a tag stream for each of its 20 articles at
	// once, and loading a second page before the first settles doubles that.
	DefaultUserQuotaArticleTagsRate        = 120
	DefaultUserQuotaArticleTagsBurst       = 60
	DefaultUserQuotaArticleTagsWindow      = time.Minute
	DefaultUserQuotaArticleTagsMaxInFlight = 40
)

type OperationQuotaConfig struct {
	Rate        int
	Burst       int
	Window      time.Duration
	MaxInFlight int
}

type UserQuotaConfig struct {
	MaxTrackedUsers int
	Summarize       OperationQuotaConfig
	AugurChat       OperationQuotaConfig
	MorningChat     OperationQuotaConfig
	AugurRetrieve   OperationQuotaConfig
	ArticleTags     OperationQuotaConfig
}

func DefaultUserQuotaConfig() UserQuotaConfig {
	return UserQuotaConfig{
		MaxTrackedUsers: DefaultUserQuotaMaxTrackedUsers,
		Summarize: OperationQuotaConfig{
			Rate:        DefaultUserQuotaSummarizeRate,
			Burst:       DefaultUserQuotaSummarizeBurst,
			Window:      DefaultUserQuotaSummarizeWindow,
			MaxInFlight: DefaultUserQuotaSummarizeMaxInFlight,
		},
		AugurChat: OperationQuotaConfig{
			Rate:        DefaultUserQuotaAugurChatRate,
			Burst:       DefaultUserQuotaAugurChatBurst,
			Window:      DefaultUserQuotaAugurChatWindow,
			MaxInFlight: DefaultUserQuotaAugurChatMaxInFlight,
		},
		MorningChat: OperationQuotaConfig{
			Rate:        DefaultUserQuotaMorningChatRate,
			Burst:       DefaultUserQuotaMorningChatBurst,
			Window:      DefaultUserQuotaMorningChatWindow,
			MaxInFlight: DefaultUserQuotaMorningChatMaxInFlight,
		},
		AugurRetrieve: OperationQuotaConfig{
			Rate:        DefaultUserQuotaAugurRetrieveRate,
			Burst:       DefaultUserQuotaAugurRetrieveBurst,
			Window:      DefaultUserQuotaAugurRetrieveWindow,
			MaxInFlight: DefaultUserQuotaAugurRetrieveMaxInFlight,
		},
		ArticleTags: OperationQuotaConfig{
			Rate:        DefaultUserQuotaArticleTagsRate,
			Burst:       DefaultUserQuotaArticleTagsBurst,
			Window:      DefaultUserQuotaArticleTagsWindow,
			MaxInFlight: DefaultUserQuotaArticleTagsMaxInFlight,
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
		UserQuotaSummarizeMaxInFlightEnv,
	); err != nil {
		return UserQuotaConfig{}, err
	}
	if cfg.AugurChat, err = loadOperationQuotaEnv(
		cfg.AugurChat,
		UserQuotaAugurChatRateEnv,
		UserQuotaAugurChatBurstEnv,
		UserQuotaAugurChatWindowEnv,
		UserQuotaAugurChatMaxInFlightEnv,
	); err != nil {
		return UserQuotaConfig{}, err
	}
	if cfg.MorningChat, err = loadOperationQuotaEnv(
		cfg.MorningChat,
		UserQuotaMorningChatRateEnv,
		UserQuotaMorningChatBurstEnv,
		UserQuotaMorningChatWindowEnv,
		UserQuotaMorningChatMaxInFlightEnv,
	); err != nil {
		return UserQuotaConfig{}, err
	}
	if cfg.AugurRetrieve, err = loadOperationQuotaEnv(
		cfg.AugurRetrieve,
		UserQuotaAugurRetrieveRateEnv,
		UserQuotaAugurRetrieveBurstEnv,
		UserQuotaAugurRetrieveWindowEnv,
		UserQuotaAugurRetrieveMaxInFlightEnv,
	); err != nil {
		return UserQuotaConfig{}, err
	}
	if cfg.ArticleTags, err = loadOperationQuotaEnv(
		cfg.ArticleTags,
		UserQuotaArticleTagsRateEnv,
		UserQuotaArticleTagsBurstEnv,
		UserQuotaArticleTagsWindowEnv,
		UserQuotaArticleTagsMaxInFlightEnv,
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
		{"augur_retrieve", cfg.AugurRetrieve},
		{"article_tags", cfg.ArticleTags},
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
		if c.op.MaxInFlight <= 0 {
			return fmt.Errorf("user quota %s max_in_flight must be > 0, got %d", c.name, c.op.MaxInFlight)
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
		"summarize_max_in_flight", cfg.Summarize.MaxInFlight,
		"augur_chat_rate", cfg.AugurChat.Rate,
		"augur_chat_burst", cfg.AugurChat.Burst,
		"augur_chat_window", cfg.AugurChat.Window.String(),
		"augur_chat_max_in_flight", cfg.AugurChat.MaxInFlight,
		"morning_chat_rate", cfg.MorningChat.Rate,
		"morning_chat_burst", cfg.MorningChat.Burst,
		"morning_chat_window", cfg.MorningChat.Window.String(),
		"morning_chat_max_in_flight", cfg.MorningChat.MaxInFlight,
		"augur_retrieve_rate", cfg.AugurRetrieve.Rate,
		"augur_retrieve_burst", cfg.AugurRetrieve.Burst,
		"augur_retrieve_window", cfg.AugurRetrieve.Window.String(),
		"augur_retrieve_max_in_flight", cfg.AugurRetrieve.MaxInFlight,
		"article_tags_rate", cfg.ArticleTags.Rate,
		"article_tags_burst", cfg.ArticleTags.Burst,
		"article_tags_window", cfg.ArticleTags.Window.String(),
		"article_tags_max_in_flight", cfg.ArticleTags.MaxInFlight,
	)
}

func loadOperationQuotaEnv(
	defaults OperationQuotaConfig,
	rateEnv, burstEnv, windowEnv, maxInFlightEnv string,
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
	if out.MaxInFlight, err = loadPositiveIntEnv(maxInFlightEnv, out.MaxInFlight); err != nil {
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

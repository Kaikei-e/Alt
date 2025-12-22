package config

import (
	"math/rand"
	"sync"
)

type UserAgentRotator struct {
	config *HTTPConfig
	index  int
	mu     sync.Mutex
}

func NewUserAgentRotator(httpConfig *HTTPConfig) *UserAgentRotator {
	return &UserAgentRotator{
		config: httpConfig,
		index:  0,
	}
}

func (uar *UserAgentRotator) GetUserAgent() string {
	if !uar.config.UserAgentRotation || len(uar.config.UserAgents) == 0 {
		return uar.config.UserAgent
	}

	uar.mu.Lock()
	defer uar.mu.Unlock()

	userAgent := uar.config.UserAgents[uar.index]
	uar.index = (uar.index + 1) % len(uar.config.UserAgents)

	return userAgent
}

func (uar *UserAgentRotator) GetRandomUserAgent() string {
	if !uar.config.UserAgentRotation || len(uar.config.UserAgents) == 0 {
		return uar.config.UserAgent
	}

	uar.mu.Lock()
	defer uar.mu.Unlock()

	// Go 1.20+ seeds math/rand automatically.
	index := rand.Intn(len(uar.config.UserAgents))
	return uar.config.UserAgents[index]
}

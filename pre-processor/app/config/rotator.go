package config

import (
	"crypto/rand"
	"math/big"
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

	// Use crypto/rand for better randomness (security best practice)
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(uar.config.UserAgents))))
	if err != nil {
		// Fallback to first user agent if random generation fails
		return uar.config.UserAgents[0]
	}
	index := int(n.Int64())
	return uar.config.UserAgents[index]
}

package autolearn

import (
	"sync"
	"time"
)

// RateLimiter implements learning rate limiting to prevent abuse
type RateLimiter struct {
	// ドメイン別学習履歴
	domainLearning    map[string]time.Time  // ドメイン別最終学習時刻
	globalLearnings   []time.Time           // 全体学習履歴（時系列）
	
	// 制限設定
	globalLimit       int           // 全体学習数制限（1時間あたり）
	domainCooldown    time.Duration // 同一ドメイン学習間隔
	
	// 並行安全性
	mutex             sync.RWMutex
}

// NewRateLimiter creates a new rate limiter with specified limits
func NewRateLimiter(globalLimitPerHour int, domainCooldown time.Duration) *RateLimiter {
	return &RateLimiter{
		domainLearning:  make(map[string]time.Time),
		globalLearnings: make([]time.Time, 0),
		globalLimit:     globalLimitPerHour,
		domainCooldown:  domainCooldown,
	}
}

// AllowLearning checks if learning is allowed for the given domain
func (rl *RateLimiter) AllowLearning(domain string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()

	// 1. 同一ドメインのクールダウンチェック
	if lastLearning, exists := rl.domainLearning[domain]; exists {
		if now.Sub(lastLearning) < rl.domainCooldown {
			return false // クールダウン期間中
		}
	}

	// 2. 全体レート制限チェック
	if !rl.checkGlobalRateLimit(now) {
		return false // 全体制限超過
	}

	// 3. 学習許可 - 記録更新
	rl.domainLearning[domain] = now
	rl.globalLearnings = append(rl.globalLearnings, now)

	// 4. 古い記録をクリーンアップ（メモリ使用量制御）
	go rl.cleanupOldRecords(now)

	return true
}

// checkGlobalRateLimit checks if global rate limit allows new learning
func (rl *RateLimiter) checkGlobalRateLimit(now time.Time) bool {
	// 1時間前の時刻を計算
	oneHourAgo := now.Add(-time.Hour)

	// 1時間以内の学習数をカウント
	recentCount := 0
	for _, learningTime := range rl.globalLearnings {
		if learningTime.After(oneHourAgo) {
			recentCount++
		}
	}

	return recentCount < rl.globalLimit
}

// cleanupOldRecords removes old records to prevent memory leak
func (rl *RateLimiter) cleanupOldRecords(now time.Time) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	// 24時間以上前の記録を削除（ドメイン学習履歴）
	oneDayAgo := now.Add(-24 * time.Hour)
	for domain, learningTime := range rl.domainLearning {
		if learningTime.Before(oneDayAgo) {
			delete(rl.domainLearning, domain)
		}
	}

	// 1時間以上前の記録を削除（全体学習履歴）
	oneHourAgo := now.Add(-time.Hour)
	recentLearnings := make([]time.Time, 0)
	for _, learningTime := range rl.globalLearnings {
		if learningTime.After(oneHourAgo) {
			recentLearnings = append(recentLearnings, learningTime)
		}
	}
	rl.globalLearnings = recentLearnings
}

// GetRateLimitStatus returns current rate limit status
func (rl *RateLimiter) GetRateLimitStatus(domain string) *RateLimitStatus {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	now := time.Now()
	status := &RateLimitStatus{
		Domain:           domain,
		CheckTime:        now,
		GlobalLimit:      rl.globalLimit,
		DomainCooldown:   rl.domainCooldown,
	}

	// ドメイン固有の状態
	if lastLearning, exists := rl.domainLearning[domain]; exists {
		status.DomainLastLearning = lastLearning
		status.DomainCooldownRemaining = rl.domainCooldown - now.Sub(lastLearning)
		if status.DomainCooldownRemaining < 0 {
			status.DomainCooldownRemaining = 0
		}
		status.DomainAllowed = status.DomainCooldownRemaining == 0
	} else {
		status.DomainAllowed = true
	}

	// 全体レート制限の状態
	oneHourAgo := now.Add(-time.Hour)
	recentCount := 0
	for _, learningTime := range rl.globalLearnings {
		if learningTime.After(oneHourAgo) {
			recentCount++
		}
	}
	status.GlobalRecentCount = recentCount
	status.GlobalAllowed = recentCount < rl.globalLimit

	// 総合判定
	status.LearningAllowed = status.DomainAllowed && status.GlobalAllowed

	return status
}

// RateLimitStatus represents the current rate limiting status
type RateLimitStatus struct {
	Domain                   string        `json:"domain"`
	CheckTime                time.Time     `json:"check_time"`
	LearningAllowed          bool          `json:"learning_allowed"`
	
	// ドメイン固有制限
	DomainAllowed            bool          `json:"domain_allowed"`
	DomainLastLearning       time.Time     `json:"domain_last_learning,omitempty"`
	DomainCooldown           time.Duration `json:"domain_cooldown"`
	DomainCooldownRemaining  time.Duration `json:"domain_cooldown_remaining"`
	
	// 全体制限
	GlobalAllowed            bool          `json:"global_allowed"`
	GlobalLimit              int           `json:"global_limit"`
	GlobalRecentCount        int           `json:"global_recent_count"`
}

// GetGlobalStats returns global rate limiting statistics
func (rl *RateLimiter) GetGlobalStats() *GlobalRateLimitStats {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	now := time.Now()
	stats := &GlobalRateLimitStats{
		CheckTime:             now,
		GlobalLimit:           rl.globalLimit,
		TotalDomainsTracked:   len(rl.domainLearning),
		DomainCooldownPeriod:  rl.domainCooldown,
	}

	// 時間帯別の学習数を計算
	oneHourAgo := now.Add(-time.Hour)
	sixHoursAgo := now.Add(-6 * time.Hour)
	oneDayAgo := now.Add(-24 * time.Hour)

	for _, learningTime := range rl.globalLearnings {
		if learningTime.After(oneHourAgo) {
			stats.LearningsLast1Hour++
		}
		if learningTime.After(sixHoursAgo) {
			stats.LearningsLast6Hours++
		}
		if learningTime.After(oneDayAgo) {
			stats.LearningsLast24Hours++
		}
	}

	// クールダウン中のドメイン数
	for _, lastLearning := range rl.domainLearning {
		if now.Sub(lastLearning) < rl.domainCooldown {
			stats.DomainsInCooldown++
		}
	}

	return stats
}

// GlobalRateLimitStats represents global rate limiting statistics
type GlobalRateLimitStats struct {
	CheckTime             time.Time     `json:"check_time"`
	GlobalLimit           int           `json:"global_limit"`
	TotalDomainsTracked   int           `json:"total_domains_tracked"`
	DomainsInCooldown     int           `json:"domains_in_cooldown"`
	DomainCooldownPeriod  time.Duration `json:"domain_cooldown_period"`
	LearningsLast1Hour    int           `json:"learnings_last_1_hour"`
	LearningsLast6Hours   int           `json:"learnings_last_6_hours"`
	LearningsLast24Hours  int           `json:"learnings_last_24_hours"`
}

// ResetDomainCooldown manually resets cooldown for a specific domain (admin function)
func (rl *RateLimiter) ResetDomainCooldown(domain string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	if _, exists := rl.domainLearning[domain]; exists {
		delete(rl.domainLearning, domain)
		return true
	}

	return false
}

// AdjustLimits allows runtime adjustment of rate limits (admin function)
func (rl *RateLimiter) AdjustLimits(newGlobalLimit int, newDomainCooldown time.Duration) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.globalLimit = newGlobalLimit
	rl.domainCooldown = newDomainCooldown
}

// GetTopDomainsByLearningFrequency returns domains with most frequent learning attempts
func (rl *RateLimiter) GetTopDomainsByLearningFrequency(limit int) []DomainFrequency {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	now := time.Now()
	oneDayAgo := now.Add(-24 * time.Hour)

	// 実際の実装では、学習履歴をより詳細に追跡する必要がある
	// 今回は簡易実装として、現在追跡中のドメインを返す
	result := make([]DomainFrequency, 0)
	
	for domain, lastLearning := range rl.domainLearning {
		if lastLearning.After(oneDayAgo) {
			result = append(result, DomainFrequency{
				Domain:       domain,
				LastLearning: lastLearning,
				// 簡易実装：頻度カウントは省略
				Count:        1,
			})
		}
		
		if len(result) >= limit {
			break
		}
	}

	return result
}

// DomainFrequency represents domain learning frequency data
type DomainFrequency struct {
	Domain       string    `json:"domain"`
	Count        int       `json:"count"`
	LastLearning time.Time `json:"last_learning"`
}
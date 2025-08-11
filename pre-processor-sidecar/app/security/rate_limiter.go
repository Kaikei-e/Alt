// ABOUTME: メモリベースレート制限機能 - Admin API保護用
// ABOUTME: IPベース制限、エンドポイント別制限、スライディングウィンドウ実装

package security

import (
	"log/slog"
	"sync"
	"time"
)

// MemoryRateLimiter はメモリベースのレート制限器
type MemoryRateLimiter struct {
	// 設定
	maxRequestsPerHour int
	cleanupInterval    time.Duration

	// レート制限データ
	mutex    sync.RWMutex
	clients  map[string]*ClientRateLimit
	
	// ログ
	logger *slog.Logger
	
	// クリーンアップ制御
	stopChan chan struct{}
	isRunning bool
}

// ClientRateLimit はクライアント毎のレート制限情報
type ClientRateLimit struct {
	requests    []RequestRecord
	lastCleanup time.Time
}

// RequestRecord は個別のリクエスト記録
type RequestRecord struct {
	timestamp time.Time
	endpoint  string
}

// NewMemoryRateLimiter は新しいメモリレート制限器を作成
func NewMemoryRateLimiter(maxRequestsPerHour int, logger *slog.Logger) *MemoryRateLimiter {
	if logger == nil {
		logger = slog.Default()
	}

	limiter := &MemoryRateLimiter{
		maxRequestsPerHour: maxRequestsPerHour,
		cleanupInterval:    5 * time.Minute,
		clients:           make(map[string]*ClientRateLimit),
		logger:            logger,
		stopChan:          make(chan struct{}),
	}

	// 定期クリーンアップ開始
	limiter.startCleanupRoutine()

	logger.Info("Memory rate limiter created",
		"max_requests_per_hour", maxRequestsPerHour,
		"cleanup_interval_minutes", limiter.cleanupInterval.Minutes())

	return limiter
}

// IsAllowed はリクエストが許可されているか確認
func (rl *MemoryRateLimiter) IsAllowed(clientIP string, endpoint string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)

	// クライアントのレート制限情報取得または作成
	client, exists := rl.clients[clientIP]
	if !exists {
		client = &ClientRateLimit{
			requests:    make([]RequestRecord, 0),
			lastCleanup: now,
		}
		rl.clients[clientIP] = client
	}

	// 古いレコードをクリーンアップ
	client.requests = rl.filterValidRequests(client.requests, oneHourAgo)
	client.lastCleanup = now

	// 現在のリクエスト数確認
	currentRequests := len(client.requests)
	
	if currentRequests >= rl.maxRequestsPerHour {
		rl.logger.Warn("Rate limit exceeded",
			"client_ip", clientIP,
			"endpoint", endpoint,
			"current_requests", currentRequests,
			"limit", rl.maxRequestsPerHour)
		return false
	}

	return true
}

// RecordRequest はリクエストを記録
func (rl *MemoryRateLimiter) RecordRequest(clientIP string, endpoint string) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()

	// クライアントのレート制限情報取得または作成
	client, exists := rl.clients[clientIP]
	if !exists {
		client = &ClientRateLimit{
			requests:    make([]RequestRecord, 0),
			lastCleanup: now,
		}
		rl.clients[clientIP] = client
	}

	// 新しいリクエストを記録
	client.requests = append(client.requests, RequestRecord{
		timestamp: now,
		endpoint:  endpoint,
	})

	rl.logger.Debug("Request recorded",
		"client_ip", clientIP,
		"endpoint", endpoint,
		"total_requests_last_hour", len(client.requests))
}

// GetClientStats はクライアントの統計情報を取得
func (rl *MemoryRateLimiter) GetClientStats(clientIP string) ClientStats {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	client, exists := rl.clients[clientIP]
	if !exists {
		return ClientStats{
			ClientIP:                clientIP,
			RequestsInLastHour:      0,
			RemainingRequests:       rl.maxRequestsPerHour,
			NextResetTime:           time.Now().Add(time.Hour),
		}
	}

	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)
	validRequests := rl.filterValidRequests(client.requests, oneHourAgo)
	
	requestsCount := len(validRequests)
	remaining := rl.maxRequestsPerHour - requestsCount
	if remaining < 0 {
		remaining = 0
	}

	// 次のリセット時間を計算（最も古いリクエストの1時間後）
	nextReset := now.Add(time.Hour)
	if len(validRequests) > 0 {
		oldestRequest := validRequests[0].timestamp
		nextReset = oldestRequest.Add(time.Hour)
	}

	return ClientStats{
		ClientIP:                clientIP,
		RequestsInLastHour:      requestsCount,
		RemainingRequests:       remaining,
		NextResetTime:           nextReset,
		EndpointBreakdown:       rl.getEndpointBreakdown(validRequests),
	}
}

// ClientStats はクライアント統計情報
type ClientStats struct {
	ClientIP           string            `json:"client_ip"`
	RequestsInLastHour int               `json:"requests_in_last_hour"`
	RemainingRequests  int               `json:"remaining_requests"`
	NextResetTime      time.Time         `json:"next_reset_time"`
	EndpointBreakdown  map[string]int    `json:"endpoint_breakdown"`
}

// GetAllClientsStats は全クライアントの統計情報を取得
func (rl *MemoryRateLimiter) GetAllClientsStats() []ClientStats {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	stats := make([]ClientStats, 0, len(rl.clients))
	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)

	for clientIP, client := range rl.clients {
		validRequests := rl.filterValidRequests(client.requests, oneHourAgo)
		
		if len(validRequests) == 0 {
			continue // 古いクライアントはスキップ
		}

		requestsCount := len(validRequests)
		remaining := rl.maxRequestsPerHour - requestsCount
		if remaining < 0 {
			remaining = 0
		}

		nextReset := validRequests[0].timestamp.Add(time.Hour)

		stats = append(stats, ClientStats{
			ClientIP:           clientIP,
			RequestsInLastHour: requestsCount,
			RemainingRequests:  remaining,
			NextResetTime:      nextReset,
			EndpointBreakdown:  rl.getEndpointBreakdown(validRequests),
		})
	}

	return stats
}

// startCleanupRoutine は定期クリーンアップルーチンを開始
func (rl *MemoryRateLimiter) startCleanupRoutine() {
	if rl.isRunning {
		return
	}

	rl.isRunning = true
	go rl.cleanupLoop()
}

// cleanupLoop はクリーンアップループ
func (rl *MemoryRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.performCleanup()
		case <-rl.stopChan:
			rl.logger.Info("Rate limiter cleanup routine stopped")
			return
		}
	}
}

// performCleanup は古いデータのクリーンアップを実行
func (rl *MemoryRateLimiter) performCleanup() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	clientsToDelete := make([]string, 0)
	totalCleaned := 0

	for clientIP, client := range rl.clients {
		// 古いリクエストをフィルタリング
		oldCount := len(client.requests)
		client.requests = rl.filterValidRequests(client.requests, oneHourAgo)
		newCount := len(client.requests)
		totalCleaned += oldCount - newCount

		// 2時間以上活動がないクライアントは削除
		if len(client.requests) == 0 && client.lastCleanup.Before(twoHoursAgo) {
			clientsToDelete = append(clientsToDelete, clientIP)
		} else {
			client.lastCleanup = now
		}
	}

	// 古いクライアントを削除
	for _, clientIP := range clientsToDelete {
		delete(rl.clients, clientIP)
	}

	if totalCleaned > 0 || len(clientsToDelete) > 0 {
		rl.logger.Debug("Rate limiter cleanup completed",
			"requests_cleaned", totalCleaned,
			"clients_removed", len(clientsToDelete),
			"active_clients", len(rl.clients))
	}
}

// filterValidRequests は有効なリクエスト（指定時刻以降）をフィルタリング
func (rl *MemoryRateLimiter) filterValidRequests(requests []RequestRecord, cutoff time.Time) []RequestRecord {
	validRequests := make([]RequestRecord, 0, len(requests))
	for _, req := range requests {
		if req.timestamp.After(cutoff) {
			validRequests = append(validRequests, req)
		}
	}
	return validRequests
}

// getEndpointBreakdown はエンドポイント別のリクエスト内訳を取得
func (rl *MemoryRateLimiter) getEndpointBreakdown(requests []RequestRecord) map[string]int {
	breakdown := make(map[string]int)
	for _, req := range requests {
		breakdown[req.endpoint]++
	}
	return breakdown
}

// Stop はレート制限器を停止
func (rl *MemoryRateLimiter) Stop() {
	if !rl.isRunning {
		return
	}

	close(rl.stopChan)
	rl.isRunning = false
	
	// メモリクリア
	rl.mutex.Lock()
	rl.clients = make(map[string]*ClientRateLimit)
	rl.mutex.Unlock()

	rl.logger.Info("Memory rate limiter stopped")
}

// GetGlobalStats はグローバル統計情報を取得
func (rl *MemoryRateLimiter) GetGlobalStats() GlobalStats {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	totalClients := len(rl.clients)
	totalRequests := 0
	endpointStats := make(map[string]int)

	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)

	for _, client := range rl.clients {
		validRequests := rl.filterValidRequests(client.requests, oneHourAgo)
		totalRequests += len(validRequests)
		
		for _, req := range validRequests {
			endpointStats[req.endpoint]++
		}
	}

	return GlobalStats{
		TotalActiveClients:   totalClients,
		TotalRequestsLastHour: totalRequests,
		MaxRequestsPerHour:   rl.maxRequestsPerHour,
		EndpointBreakdown:    endpointStats,
		LastCleanup:          time.Now(),
	}
}

// GlobalStats はグローバル統計情報
type GlobalStats struct {
	TotalActiveClients    int            `json:"total_active_clients"`
	TotalRequestsLastHour int            `json:"total_requests_last_hour"`
	MaxRequestsPerHour    int            `json:"max_requests_per_hour"`
	EndpointBreakdown     map[string]int `json:"endpoint_breakdown"`
	LastCleanup           time.Time      `json:"last_cleanup"`
}
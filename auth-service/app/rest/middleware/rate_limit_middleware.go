package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

type RateLimiter struct {
	visitors map[string]*Visitor
	mutex    sync.RWMutex
}

type Visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*Visitor),
	}

	// クリーンアップGoルーチン
	go rl.cleanupVisitors()
	return rl
}

func (rl *RateLimiter) RateLimit() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// IPアドレスベースの制限
			ip := c.RealIP()

			// 認証エンドポイント別の制限設定
			var limit rate.Limit
			var burst int

			path := c.Request().URL.Path
			switch {
			case strings.Contains(path, "/login"):
				limit = rate.Every(1 * time.Minute) // 1分間に1回
				burst = 5                          // バースト5回
			case strings.Contains(path, "/register"):
				limit = rate.Every(5 * time.Minute) // 5分間に1回
				burst = 3                           // バースト3回
			case strings.Contains(path, "/csrf"):
				limit = rate.Every(10 * time.Second) // 10秒間に1回
				burst = 10
			default:
				limit = rate.Every(1 * time.Second) // 通常API
				burst = 20
			}

			if !rl.allow(ip, limit, burst) {
				return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
					"error":       "Rate limit exceeded",
					"code":        "RATE_LIMIT_EXCEEDED",
					"retry_after": rl.getRetryAfter(ip),
				})
			}

			return next(c)
		}
	}
}

func (rl *RateLimiter) allow(ip string, limit rate.Limit, burst int) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	visitor, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &Visitor{
			limiter:  rate.NewLimiter(limit, burst),
			lastSeen: time.Now(),
		}
		return true
	}

	visitor.lastSeen = time.Now()
	return visitor.limiter.Allow()
}

func (rl *RateLimiter) getRetryAfter(ip string) int {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	visitor, exists := rl.visitors[ip]
	if !exists {
		return 0
	}

	// 次回許可されるまでの時間を秒で返す
	reservation := visitor.limiter.Reserve()
	if !reservation.OK() {
		return 60 // デフォルト60秒
	}

	delay := reservation.Delay()
	reservation.Cancel() // 実際には使わないのでキャンセル

	return int(delay.Seconds())
}

func (rl *RateLimiter) cleanupVisitors() {
	for {
		time.Sleep(time.Minute)

		rl.mutex.Lock()
		for ip, visitor := range rl.visitors {
			if time.Since(visitor.lastSeen) > 3*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mutex.Unlock()
	}
}

// contains はstrings.Containsの代替
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
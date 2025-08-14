package security

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
)

type IntrusionDetectionSystem struct {
	logger         *slog.Logger
	alertThreshold int
	suspiciousIPs  map[string]*SuspiciousActivity
	mutex          sync.RWMutex
}

type SuspiciousActivity struct {
	IP             string
	Attempts       int
	LastAttempt    time.Time
	AttackPatterns []string
	ThreatLevel    ThreatLevel
}

type ThreatLevel int

const (
	ThreatLevelLow ThreatLevel = iota
	ThreatLevelMedium
	ThreatLevelHigh
	ThreatLevelCritical
)

func NewIDS(logger *slog.Logger) *IntrusionDetectionSystem {
	ids := &IntrusionDetectionSystem{
		logger:         logger.With("component", "ids"),
		alertThreshold: 10,
		suspiciousIPs:  make(map[string]*SuspiciousActivity),
	}

	// クリーンアップGoルーチン
	go ids.cleanupSuspiciousIPs()
	return ids
}

func (ids *IntrusionDetectionSystem) AnalyzeRequest(ctx context.Context, ip, userAgent, path, body string) bool {
	suspicious := false
	patterns := []string{}

	// SQLインジェクション検知
	if ids.detectSQLInjection(body) {
		suspicious = true
		patterns = append(patterns, "SQL_INJECTION")
	}

	// XSS検知
	if ids.detectXSS(body) {
		suspicious = true
		patterns = append(patterns, "XSS_ATTACK")
	}

	// パストラバーサル検知
	if ids.detectPathTraversal(path) {
		suspicious = true
		patterns = append(patterns, "PATH_TRAVERSAL")
	}

	// 異常なUser-Agent検知
	if ids.detectSuspiciousUserAgent(userAgent) {
		suspicious = true
		patterns = append(patterns, "SUSPICIOUS_UA")
	}

	// ブルートフォース検知
	if ids.detectBruteForce(ip, path) {
		suspicious = true
		patterns = append(patterns, "BRUTE_FORCE")
	}

	if suspicious {
		ids.recordSuspiciousActivity(ip, patterns)
		return false // リクエストをブロック
	}

	return true // リクエストを許可
}

func (ids *IntrusionDetectionSystem) detectSQLInjection(input string) bool {
	sqlPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)union\s+select`),
		regexp.MustCompile(`(?i)or\s+1\s*=\s*1`),
		regexp.MustCompile(`(?i)drop\s+table`),
		regexp.MustCompile(`(?i)insert\s+into`),
		regexp.MustCompile(`(?i)delete\s+from`),
		regexp.MustCompile(`(?i)'.*or.*'.*'`),
	}

	for _, pattern := range sqlPatterns {
		if pattern.MatchString(input) {
			return true
		}
	}
	return false
}

func (ids *IntrusionDetectionSystem) detectXSS(input string) bool {
	xssPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<script[^>]*>`),
		regexp.MustCompile(`(?i)javascript:`),
		regexp.MustCompile(`(?i)on\w+\s*=`),
		regexp.MustCompile(`(?i)<iframe[^>]*>`),
		regexp.MustCompile(`(?i)eval\s*\(`),
	}

	for _, pattern := range xssPatterns {
		if pattern.MatchString(input) {
			return true
		}
	}
	return false
}

func (ids *IntrusionDetectionSystem) detectPathTraversal(path string) bool {
	traversalPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\.\./`),
		regexp.MustCompile(`\.\.\\`),
		regexp.MustCompile(`%2e%2e%2f`),
		regexp.MustCompile(`%252e%252e%252f`),
	}

	for _, pattern := range traversalPatterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}

func (ids *IntrusionDetectionSystem) detectSuspiciousUserAgent(userAgent string) bool {
	suspiciousPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)sqlmap`),
		regexp.MustCompile(`(?i)nmap`),
		regexp.MustCompile(`(?i)nikto`),
		regexp.MustCompile(`(?i)burp`),
		regexp.MustCompile(`(?i)wget`),
		regexp.MustCompile(`(?i)curl.*bot`),
	}

	for _, pattern := range suspiciousPatterns {
		if pattern.MatchString(userAgent) {
			return true
		}
	}
	return false
}

func (ids *IntrusionDetectionSystem) detectBruteForce(ip, path string) bool {
	ids.mutex.RLock()
	activity, exists := ids.suspiciousIPs[ip]
	ids.mutex.RUnlock()

	if !exists {
		return false
	}

	// 5分以内に10回以上のログイン試行
	if strings.Contains(path, "/login") &&
		activity.Attempts > 10 &&
		time.Since(activity.LastAttempt) < 5*time.Minute {
		return true
	}

	return false
}

func (ids *IntrusionDetectionSystem) recordSuspiciousActivity(ip string, patterns []string) {
	ids.mutex.Lock()
	defer ids.mutex.Unlock()

	activity, exists := ids.suspiciousIPs[ip]
	if !exists {
		activity = &SuspiciousActivity{
			IP:             ip,
			Attempts:       0,
			AttackPatterns: []string{},
			ThreatLevel:    ThreatLevelLow,
		}
		ids.suspiciousIPs[ip] = activity
	}

	activity.Attempts++
	activity.LastAttempt = time.Now()
	activity.AttackPatterns = append(activity.AttackPatterns, patterns...)

	// 脅威レベル判定
	if activity.Attempts > 50 {
		activity.ThreatLevel = ThreatLevelCritical
	} else if activity.Attempts > 20 {
		activity.ThreatLevel = ThreatLevelHigh
	} else if activity.Attempts > 10 {
		activity.ThreatLevel = ThreatLevelMedium
	}

	// アラート送信
	ids.sendSecurityAlert(activity)
}

func (ids *IntrusionDetectionSystem) sendSecurityAlert(activity *SuspiciousActivity) {
	ids.logger.Error("Security threat detected",
		"ip", activity.IP,
		"attempts", activity.Attempts,
		"threat_level", activity.ThreatLevel,
		"patterns", strings.Join(activity.AttackPatterns, ","),
		"last_attempt", activity.LastAttempt)

	// TODO: 外部アラートシステムとの統合
	// - Slack通知
	// - メール通知
	// - PagerDutyアラート
}

func (ids *IntrusionDetectionSystem) cleanupSuspiciousIPs() {
	for {
		time.Sleep(30 * time.Minute)

		ids.mutex.Lock()
		for ip, activity := range ids.suspiciousIPs {
			// 1時間以上古い記録は削除
			if time.Since(activity.LastAttempt) > time.Hour {
				delete(ids.suspiciousIPs, ip)
			}
		}
		ids.mutex.Unlock()
	}
}

func (ids *IntrusionDetectionSystem) GetThreatLevel(ip string) ThreatLevel {
	ids.mutex.RLock()
	defer ids.mutex.RUnlock()

	activity, exists := ids.suspiciousIPs[ip]
	if !exists {
		return ThreatLevelLow
	}

	return activity.ThreatLevel
}

func (ids *IntrusionDetectionSystem) IsBlocked(ip string) bool {
	threatLevel := ids.GetThreatLevel(ip)
	return threatLevel >= ThreatLevelHigh
}
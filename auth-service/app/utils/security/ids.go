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
	ThreatLevelSafe ThreatLevel = iota      // 0: 許可
	ThreatLevelSuspect                      // 1: ログのみ
	ThreatLevelDangerous                    // 2: レート制限
	ThreatLevelMalicious                    // 3: 完全ブロック
	// 旧レベル維持（後方互換）
	ThreatLevelLow      = ThreatLevelSafe
	ThreatLevelMedium   = ThreatLevelSuspect
	ThreatLevelHigh     = ThreatLevelDangerous
	ThreatLevelCritical = ThreatLevelMalicious
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

// Phase 6.0.1: 段階的脅威レベル判定に変更
func (ids *IntrusionDetectionSystem) AnalyzeRequest(ctx context.Context, ip, userAgent, path, body string) ThreatLevel {
	threatLevel := ThreatLevelSafe
	patterns := []string{}

	// SQLインジェクション検知
	if ids.detectSQLInjection(body) {
		threatLevel = ThreatLevelMalicious
		patterns = append(patterns, "SQL_INJECTION")
	}

	// XSS検知
	if ids.detectXSS(body) {
		threatLevel = ThreatLevelMalicious
		patterns = append(patterns, "XSS_ATTACK")
	}

	// パストラバーサル検知
	if ids.detectPathTraversal(path) {
		threatLevel = ThreatLevelMalicious
		patterns = append(patterns, "PATH_TRAVERSAL")
	}

	// User-Agent検知（段階的判定）
	if ids.detectSuspiciousUserAgent(userAgent) {
		// 実際の攻撃ツールの場合のみMalicious
		threatLevel = ThreatLevelMalicious
		patterns = append(patterns, "MALICIOUS_UA")
	} else if !ids.isAllowedUserAgent(userAgent) && userAgent != "" {
		// 未知のUser-Agentは疑わしいが、ログのみ
		threatLevel = ThreatLevelSuspect
		patterns = append(patterns, "UNKNOWN_UA")
	}

	// ブルートフォース検知
	if ids.detectBruteForce(ip, path) {
		if threatLevel < ThreatLevelDangerous {
			threatLevel = ThreatLevelDangerous
		}
		patterns = append(patterns, "BRUTE_FORCE")
	}

	// 疑わしい活動を記録（Safe以外）
	if threatLevel > ThreatLevelSafe {
		ids.recordSuspiciousActivity(ip, patterns)
	}

	return threatLevel
}

// Phase 6.0.1: 後方互換のためのLegacy function
func (ids *IntrusionDetectionSystem) AnalyzeRequestLegacy(ctx context.Context, ip, userAgent, path, body string) bool {
	threatLevel := ids.AnalyzeRequest(ctx, ip, userAgent, path, body)
	return threatLevel <= ThreatLevelSuspect // Suspect以下は許可
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
	// Phase 6.0.1: 許可リスト優先チェック
	if ids.isAllowedUserAgent(userAgent) {
		return false // 許可リストにある場合は安全
	}

	// 実際の攻撃ツールのみを検知（過敏検知修正）
	maliciousPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)sqlmap`),
		regexp.MustCompile(`(?i)nikto`),
		regexp.MustCompile(`(?i)burpsuite`),
		regexp.MustCompile(`(?i)masscan`),
		regexp.MustCompile(`(?i)nmap.*scanner`),
		regexp.MustCompile(`(?i)zap.*proxy`),
		regexp.MustCompile(`(?i)w3af`),
		regexp.MustCompile(`(?i)havij`),
		regexp.MustCompile(`(?i)acunetix`),
		// 明確な攻撃パターンのみ（curl, wgetは除外）
		regexp.MustCompile(`(?i)attack.*bot`),
		regexp.MustCompile(`(?i)exploit.*scanner`),
	}

	for _, pattern := range maliciousPatterns {
		if pattern.MatchString(userAgent) {
			return true
		}
	}
	return false
}

// Phase 6.0.1: 許可リスト機能追加
func (ids *IntrusionDetectionSystem) isAllowedUserAgent(userAgent string) bool {
	// 正当なブラウザとツール
	allowedPatterns := []*regexp.Regexp{
		// ブラウザ
		regexp.MustCompile(`(?i)mozilla/.*firefox`),
		regexp.MustCompile(`(?i)mozilla/.*chrome`),
		regexp.MustCompile(`(?i)mozilla/.*safari`),
		regexp.MustCompile(`(?i)mozilla/.*edge`),
		// 正当なAPI client
		regexp.MustCompile(`(?i)curl/[0-9]`),        // curl/7.x など
		regexp.MustCompile(`(?i)wget/[0-9]`),        // wget/1.x など
		regexp.MustCompile(`(?i)postmanruntime/`),
		regexp.MustCompile(`(?i)insomnia/`),
		// Kubernetes系
		regexp.MustCompile(`(?i)kube-probe/`),
		regexp.MustCompile(`(?i)go-http-client/`),
		// 内部サービス
		regexp.MustCompile(`(?i)linkerd-proxy/`),
		regexp.MustCompile(`(?i)envoy/`),
		// Node.js内部通信 - Phase 6.4.3 Fix
		regexp.MustCompile(`^node$`),                    // user-agent: "node"
		regexp.MustCompile(`(?i)node/[0-9]`),           // node/18.x など
		regexp.MustCompile(`(?i)node\.js/[0-9]`),       // node.js/18.x など
		regexp.MustCompile(`(?i)next\.js/`),            // Next.js internal
		regexp.MustCompile(`(?i)npm/[0-9]`),            // npm client
		regexp.MustCompile(`(?i)yarn/[0-9]`),           // yarn client
		// テスト・開発ツール
		regexp.MustCompile(`(?i).*compatible.*browser`),
	}

	for _, pattern := range allowedPatterns {
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
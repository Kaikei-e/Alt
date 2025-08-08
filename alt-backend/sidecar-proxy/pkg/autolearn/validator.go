package autolearn

import (
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
)

// DomainValidator handles security validation for auto-learned domains
type DomainValidator struct {
	blacklist map[string]bool // 危険ドメインブラックリスト
	whitelist map[string]bool // 事前承認ドメインホワイトリスト
	logger    *log.Logger     // セキュリティログ
}

// NewDomainValidator creates a new domain validator with security rules
func NewDomainValidator(logger *log.Logger) *DomainValidator {
	dv := &DomainValidator{
		blacklist: make(map[string]bool),
		whitelist: make(map[string]bool),
		logger:    logger,
	}

	// Initialize default blacklist
	dv.initializeBlacklist()

	// Initialize default whitelist
	dv.initializeWhitelist()

	return dv
}

// ValidateNewDomain performs comprehensive security validation
func (dv *DomainValidator) ValidateNewDomain(domain string) error {
	// 1. ブラックリスト優先チェック
	if dv.isBlacklisted(domain) {
		dv.logger.Printf("[Security] 🚫 Blocked blacklisted domain: %s", domain)
		return fmt.Errorf("domain is blacklisted: %s", domain)
	}

	// 2. ホワイトリスト自動承認
	if dv.isWhitelisted(domain) {
		dv.logger.Printf("[Security] ✅ Whitelisted domain auto-approved: %s", domain)
		return nil
	}

	// 3. 基本形式検証
	if err := dv.validateDomainFormat(domain); err != nil {
		return fmt.Errorf("invalid domain format: %w", err)
	}

	// 4. 危険パターン検出
	if err := dv.checkDangerousPatterns(domain); err != nil {
		return fmt.Errorf("dangerous pattern detected: %w", err)
	}

	// 5. プライベートネットワーク除外
	if dv.isPrivateNetwork(domain) {
		return fmt.Errorf("private network domain not allowed: %s", domain)
	}

	// 6. IP アドレス直接指定の除外
	if dv.isIPAddress(domain) {
		return fmt.Errorf("direct IP address not allowed: %s", domain)
	}

	// 7. 長さ制限
	if len(domain) > 253 {
		return fmt.Errorf("domain too long: %d characters (max 253)", len(domain))
	}

	// 8. サブドメイン深度チェック
	if dv.isExcessiveSubdomains(domain) {
		return fmt.Errorf("excessive subdomains detected: %s", domain)
	}

	dv.logger.Printf("[Security] ✅ Domain validation passed: %s", domain)
	return nil
}

// initializeBlacklist sets up default blacklisted domains
func (dv *DomainValidator) initializeBlacklist() {
	// 危険・悪意のあるドメインパターン
	blacklistedDomains := []string{
		// ローカル・内部アドレス
		"localhost",
		"127.0.0.1",
		"::1",
		"0.0.0.0",

		// 既知の悪意のあるドメイン（例）
		"malware.com",
		"phishing.net",
		"suspicious.org",
		"badactor.io",

		// テスト・開発ドメイン
		"test.local",
		"dev.internal",
		"staging.private",

		// 一般的な危険パターン
		"bit.do",       // 短縮URL（セキュリティリスク）
		"grabify.link", // IPロガー
		"iplogger.org", // IPロガー
	}

	for _, domain := range blacklistedDomains {
		dv.blacklist[strings.ToLower(domain)] = true
	}

	dv.logger.Printf("[DomainValidator] Initialized with %d blacklisted domains", len(dv.blacklist))
}

// initializeWhitelist sets up trusted domains for auto-approval
func (dv *DomainValidator) initializeWhitelist() {
	// 信頼できるメジャーなRSSプロバイダー
	trustedDomains := []string{
		// 主要ニュースサイト
		"feeds.bbci.co.uk",
		"rss.cnn.com",
		"feeds.reuters.com",
		"feeds.feedburner.com",

		// 技術サイト
		"github.com",
		"qiita.com",
		"zenn.dev",
		"wired.com",
		"techcrunch.com",
		"hacker-news.firebaseio.com",

		// ブログプラットフォーム
		"medium.com",
		"dev.to",
		"hashnode.com",

		// テストドメイン（開発用）
		"httpbin.org",
		"jsonplaceholder.typicode.com",
	}

	for _, domain := range trustedDomains {
		dv.whitelist[strings.ToLower(domain)] = true
	}

	dv.logger.Printf("[DomainValidator] Initialized with %d whitelisted domains", len(dv.whitelist))
}

// validateDomainFormat validates basic domain format using regex
func (dv *DomainValidator) validateDomainFormat(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	// RFC 1035 準拠のドメイン形式検証（簡易版）
	// 完全な実装ではないが、基本的な攻撃を防ぐには十分
	domainRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain format: %s", domain)
	}

	// 連続ハイフンチェック
	if strings.Contains(domain, "--") {
		// Punycode（国際化ドメイン名）は許可するが、その他の連続ハイフンは拒否
		if !strings.HasPrefix(domain, "xn--") {
			parts := strings.Split(domain, ".")
			for _, part := range parts {
				if strings.Contains(part, "--") && !strings.HasPrefix(part, "xn--") {
					return fmt.Errorf("invalid consecutive hyphens in domain: %s", domain)
				}
			}
		}
	}

	return nil
}

// checkDangerousPatterns detects dangerous patterns in domain names
func (dv *DomainValidator) checkDangerousPatterns(domain string) error {
	lowerDomain := strings.ToLower(domain)

	// 危険キーワードパターン
	dangerousPatterns := []struct {
		pattern string
		reason  string
	}{
		// 内部・プライベートドメイン
		{".local", "local domain"},
		{".internal", "internal domain"},
		{".private", "private domain"},
		{".test", "test domain"},
		{".example", "example domain"},
		{".invalid", "invalid domain"},

		// セキュリティ脅威
		{"malware", "malware-related"},
		{"phishing", "phishing-related"},
		{"suspicious", "suspicious activity"},
		{"badactor", "known bad actor"},
		{"c2server", "command and control"},

		// 一般的な攻撃パターン
		{"admin", "administrative interface"},
		{"login", "login interface"},
		{"secure", "fake security"},
		{"bank", "banking impersonation"},
		{"paypal", "payment impersonation"},

		// URL短縮・リダイレクト
		{"bit.ly", "URL shortener"},
		{"tinyurl", "URL shortener"},
		{"t.co", "URL shortener"},
		{"goo.gl", "URL shortener"},

		// 一時・使い捨てサービス
		{"temp", "temporary service"},
		{"disposable", "disposable service"},
		{"fake", "fake service"},
		{"throw", "throwaway service"},
	}

	for _, dp := range dangerousPatterns {
		if strings.Contains(lowerDomain, dp.pattern) {
			return fmt.Errorf("dangerous pattern '%s' detected (%s)", dp.pattern, dp.reason)
		}
	}

	return nil
}

// isPrivateNetwork checks if domain resolves to private network
func (dv *DomainValidator) isPrivateNetwork(domain string) bool {
	// RFC 1918 プライベートIPレンジ
	privateRanges := []string{
		"10.0.0.0/8",     // Class A
		"172.16.0.0/12",  // Class B
		"192.168.0.0/16", // Class C
	}

	// 特別用途IPレンジ
	specialRanges := []string{
		"127.0.0.0/8",    // Loopback
		"169.254.0.0/16", // Link-local
		"224.0.0.0/4",    // Multicast
		"240.0.0.0/4",    // Reserved
	}

	allRanges := append(privateRanges, specialRanges...)

	// DNS解決してIPチェック（簡易実装）
	ips, err := net.LookupIP(domain)
	if err != nil {
		// DNS解決失敗は警告だが、ブロックはしない（一時的な問題の可能性）
		dv.logger.Printf("[DomainValidator] Warning: DNS lookup failed for %s: %v", domain, err)
		return false
	}

	for _, ip := range ips {
		for _, rangeStr := range allRanges {
			_, cidr, err := net.ParseCIDR(rangeStr)
			if err != nil {
				continue
			}
			if cidr.Contains(ip) {
				dv.logger.Printf("[DomainValidator] 🚫 Private network IP detected: %s -> %s", domain, ip)
				return true
			}
		}
	}

	return false
}

// isIPAddress checks if domain is actually an IP address
func (dv *DomainValidator) isIPAddress(domain string) bool {
	ip := net.ParseIP(domain)
	return ip != nil
}

// isExcessiveSubdomains checks for suspicious subdomain depth
func (dv *DomainValidator) isExcessiveSubdomains(domain string) bool {
	parts := strings.Split(domain, ".")

	// 5つ以上のサブドメインは疑わしい（例: a.b.c.d.e.com）
	if len(parts) > 5 {
		dv.logger.Printf("[DomainValidator] ⚠️  Excessive subdomains detected: %s (%d levels)", domain, len(parts))
		return true
	}

	return false
}

// isBlacklisted checks if domain is in blacklist
func (dv *DomainValidator) isBlacklisted(domain string) bool {
	lowerDomain := strings.ToLower(domain)

	// 完全一致チェック
	if dv.blacklist[lowerDomain] {
		return true
	}

	// サブドメインチェック（例: evil.example.com が example.com がブラックリストにある場合）
	parts := strings.Split(lowerDomain, ".")
	for i := 1; i < len(parts); i++ {
		parentDomain := strings.Join(parts[i:], ".")
		if dv.blacklist[parentDomain] {
			return true
		}
	}

	return false
}

// isWhitelisted checks if domain is in whitelist
func (dv *DomainValidator) isWhitelisted(domain string) bool {
	lowerDomain := strings.ToLower(domain)

	// 完全一致チェック
	if dv.whitelist[lowerDomain] {
		return true
	}

	// サブドメインチェック（例: api.github.com が github.com がホワイトリストにある場合）
	parts := strings.Split(lowerDomain, ".")
	for i := 1; i < len(parts); i++ {
		parentDomain := strings.Join(parts[i:], ".")
		if dv.whitelist[parentDomain] {
			return true
		}
	}

	return false
}

// AddToBlacklist manually adds domain to blacklist
func (dv *DomainValidator) AddToBlacklist(domain, reason string) {
	lowerDomain := strings.ToLower(domain)
	dv.blacklist[lowerDomain] = true
	dv.logger.Printf("[DomainValidator] 🚫 Added to blacklist: %s (reason: %s)", domain, reason)
}

// AddToWhitelist manually adds domain to whitelist
func (dv *DomainValidator) AddToWhitelist(domain, reason string) {
	lowerDomain := strings.ToLower(domain)
	dv.whitelist[lowerDomain] = true
	dv.logger.Printf("[DomainValidator] ✅ Added to whitelist: %s (reason: %s)", domain, reason)
}

// GetBlacklist returns current blacklist
func (dv *DomainValidator) GetBlacklist() []string {
	result := make([]string, 0, len(dv.blacklist))
	for domain := range dv.blacklist {
		result = append(result, domain)
	}
	return result
}

// GetWhitelist returns current whitelist
func (dv *DomainValidator) GetWhitelist() []string {
	result := make([]string, 0, len(dv.whitelist))
	for domain := range dv.whitelist {
		result = append(result, domain)
	}
	return result
}

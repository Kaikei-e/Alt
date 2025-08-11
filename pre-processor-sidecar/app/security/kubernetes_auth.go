// ABOUTME: Kubernetes ServiceAccount認証機能
// ABOUTME: JWT検証、RBAC権限確認、Pod内認証対応

package security

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"pre-processor-sidecar/handler"
)

// KubernetesAuthenticator はKubernetes認証機能を提供
type KubernetesAuthenticator struct {
	logger           *slog.Logger
	publicKey        *rsa.PublicKey
	serviceAccountCA []byte
	namespace        string
	
	// 設定
	tokenPath        string
	caPath           string
	namespacePath    string
}

// ServiceAccountClaims はServiceAccountトークンのクレーム
type ServiceAccountClaims struct {
	jwt.RegisteredClaims
	Kubernetes KubernetesClaims `json:"kubernetes.io,omitempty"`
}

// KubernetesClaims はKubernetes固有のクレーム
type KubernetesClaims struct {
	Namespace      string                    `json:"namespace"`
	ServiceAccount ServiceAccountReference   `json:"serviceaccount"`
	Pod            *PodReference            `json:"pod,omitempty"`
}

// ServiceAccountReference はServiceAccount参照
type ServiceAccountReference struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

// PodReference はPod参照
type PodReference struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

// NewKubernetesAuthenticator は新しいKubernetes認証器を作成
func NewKubernetesAuthenticator(logger *slog.Logger) *KubernetesAuthenticator {
	if logger == nil {
		logger = slog.Default()
	}

	auth := &KubernetesAuthenticator{
		logger:        logger,
		tokenPath:     "/var/run/secrets/kubernetes.io/serviceaccount/token",
		caPath:        "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		namespacePath: "/var/run/secrets/kubernetes.io/serviceaccount/namespace",
	}

	// 環境変数からのオーバーライド
	if path := os.Getenv("SERVICE_ACCOUNT_TOKEN_PATH"); path != "" {
		auth.tokenPath = path
	}
	if path := os.Getenv("SERVICE_ACCOUNT_CA_PATH"); path != "" {
		auth.caPath = path
	}
	if path := os.Getenv("SERVICE_ACCOUNT_NAMESPACE_PATH"); path != "" {
		auth.namespacePath = path
	}

	// 初期化
	if err := auth.initialize(); err != nil {
		logger.Warn("Kubernetes authenticator initialization failed, running in fallback mode", "error", err)
	} else {
		logger.Info("Kubernetes authenticator initialized successfully", "namespace", auth.namespace)
	}

	return auth
}

// initialize は認証器を初期化
func (ka *KubernetesAuthenticator) initialize() error {
	// 名前空間を読み取り
	if namespaceBytes, err := ioutil.ReadFile(ka.namespacePath); err == nil {
		ka.namespace = strings.TrimSpace(string(namespaceBytes))
	} else {
		ka.logger.Warn("Could not read namespace file", "path", ka.namespacePath, "error", err)
		ka.namespace = "default"
	}

	// CA証明書を読み取り
	caBytes, err := ioutil.ReadFile(ka.caPath)
	if err != nil {
		return fmt.Errorf("failed to read CA certificate: %w", err)
	}
	ka.serviceAccountCA = caBytes

	// CA証明書から公開鍵を抽出
	if err := ka.extractPublicKey(caBytes); err != nil {
		return fmt.Errorf("failed to extract public key from CA: %w", err)
	}

	return nil
}

// extractPublicKey はCA証明書から公開鍵を抽出
func (ka *KubernetesAuthenticator) extractPublicKey(caBytes []byte) error {
	block, _ := pem.Decode(caBytes)
	if block == nil {
		return fmt.Errorf("failed to parse PEM block containing the CA certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("CA certificate does not contain RSA public key")
	}

	ka.publicKey = publicKey
	return nil
}

// ValidateKubernetesServiceAccountToken はServiceAccountトークンを検証
func (ka *KubernetesAuthenticator) ValidateKubernetesServiceAccountToken(tokenString string) (*handler.ServiceAccountInfo, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("empty token")
	}

	// JWTトークンをパース
	token, err := jwt.ParseWithClaims(tokenString, &ServiceAccountClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 署名方法の確認
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// 公開鍵が利用可能であれば使用
		if ka.publicKey != nil {
			return ka.publicKey, nil
		}

		// フォールバック: トークンの基本検証のみ
		ka.logger.Warn("Public key not available, performing basic token validation only")
		return nil, nil
	})

	if err != nil {
		// 公開鍵が利用できない場合の基本検証
		if ka.publicKey == nil {
			return ka.validateTokenBasic(tokenString)
		}
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	// クレームの抽出
	claims, ok := token.Claims.(*ServiceAccountClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// トークンの有効性確認
	if !token.Valid {
		return nil, fmt.Errorf("token is invalid")
	}

	// 期限切れ確認
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return nil, fmt.Errorf("token has expired")
	}

	// ServiceAccount情報の構築
	info := &handler.ServiceAccountInfo{
		Subject:   claims.Subject,
		Namespace: claims.Kubernetes.Namespace,
		Name:      claims.Kubernetes.ServiceAccount.Name,
		UID:       claims.Kubernetes.ServiceAccount.UID,
	}

	// オーディエンス情報をグループとして追加
	if len(claims.Audience) > 0 {
		info.Groups = claims.Audience
	}

	ka.logger.Debug("ServiceAccount token validated successfully",
		"subject", info.Subject,
		"namespace", info.Namespace,
		"service_account", info.Name,
		"expires_at", claims.ExpiresAt)

	return info, nil
}

// validateTokenBasic は基本的なトークン検証（公開鍵なし）
func (ka *KubernetesAuthenticator) validateTokenBasic(tokenString string) (*handler.ServiceAccountInfo, error) {
	// JWT構造の基本確認
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	// ペイロード部分をデコード（検証なし）
	payload := parts[1]
	
	// Base64 URLデコード
	decoded, err := jwt.DecodeSegment(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	// JSONパース
	var claims ServiceAccountClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	// 基本的な妥当性確認
	if claims.Subject == "" {
		return nil, fmt.Errorf("missing subject in token")
	}

	if claims.Kubernetes.ServiceAccount.Name == "" {
		return nil, fmt.Errorf("missing service account name in token")
	}

	// 期限切れ確認（あれば）
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return nil, fmt.Errorf("token has expired")
	}

	info := &handler.ServiceAccountInfo{
		Subject:   claims.Subject,
		Namespace: claims.Kubernetes.Namespace,
		Name:      claims.Kubernetes.ServiceAccount.Name,
		UID:       claims.Kubernetes.ServiceAccount.UID,
	}

	if len(claims.Audience) > 0 {
		info.Groups = claims.Audience
	}

	ka.logger.Warn("ServiceAccount token validated with basic method (signature not verified)",
		"subject", info.Subject,
		"namespace", info.Namespace,
		"service_account", info.Name)

	return info, nil
}

// HasAdminPermissions は管理者権限を確認
func (ka *KubernetesAuthenticator) HasAdminPermissions(info *handler.ServiceAccountInfo) bool {
	if info == nil {
		return false
	}

	// 管理者権限の判定ロジック
	
	// 1. 特定のServiceAccount名による判定
	adminServiceAccounts := []string{
		"pre-processor-admin",
		"pre-processor-sidecar-admin", 
		"system:serviceaccount:" + ka.namespace + ":pre-processor-admin",
		"default", // 開発環境用（本番では削除推奨）
	}

	for _, adminSA := range adminServiceAccounts {
		if info.Name == adminSA || info.Subject == adminSA {
			ka.logger.Info("Admin access granted via service account name",
				"service_account", info.Name,
				"subject", info.Subject)
			return true
		}
	}

	// 2. 名前空間による判定
	if info.Namespace == ka.namespace || info.Namespace == "alt-processing" {
		// 同じ名前空間のServiceAccountには基本的な権限を付与
		ka.logger.Debug("Admin access granted via namespace",
			"namespace", info.Namespace,
			"service_account", info.Name)
		return true
	}

	// 3. グループによる判定
	for _, group := range info.Groups {
		if strings.Contains(group, "admin") || strings.Contains(group, "system:masters") {
			ka.logger.Info("Admin access granted via group membership",
				"group", group,
				"service_account", info.Name)
			return true
		}
	}

	// 4. 開発環境での特別扱い
	if ka.isDevelopmentEnvironment() {
		ka.logger.Warn("Admin access granted in development environment",
			"service_account", info.Name,
			"namespace", info.Namespace)
		return true
	}

	ka.logger.Warn("Admin access denied",
		"service_account", info.Name,
		"namespace", info.Namespace,
		"subject", info.Subject,
		"groups", info.Groups)

	return false
}

// isDevelopmentEnvironment は開発環境かどうか判定
func (ka *KubernetesAuthenticator) isDevelopmentEnvironment() bool {
	// 環境変数による判定
	env := os.Getenv("ENVIRONMENT")
	if env == "development" || env == "dev" || env == "local" {
		return true
	}

	// 名前空間による判定
	devNamespaces := []string{"default", "development", "dev"}
	for _, ns := range devNamespaces {
		if ka.namespace == ns {
			return true
		}
	}

	return false
}

// GetCurrentServiceAccount は現在のServiceAccount情報を取得
func (ka *KubernetesAuthenticator) GetCurrentServiceAccount() (*handler.ServiceAccountInfo, error) {
	tokenBytes, err := ioutil.ReadFile(ka.tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service account token: %w", err)
	}

	tokenString := strings.TrimSpace(string(tokenBytes))
	return ka.ValidateKubernetesServiceAccountToken(tokenString)
}

// GetAuthenticationInfo は認証情報の詳細を取得
func (ka *KubernetesAuthenticator) GetAuthenticationInfo() AuthenticationInfo {
	return AuthenticationInfo{
		HasPublicKey:     ka.publicKey != nil,
		HasCA:           len(ka.serviceAccountCA) > 0,
		Namespace:       ka.namespace,
		TokenPath:       ka.tokenPath,
		CAPath:          ka.caPath,
		IsDevelopment:   ka.isDevelopmentEnvironment(),
	}
}

// AuthenticationInfo は認証情報
type AuthenticationInfo struct {
	HasPublicKey  bool   `json:"has_public_key"`
	HasCA         bool   `json:"has_ca"`
	Namespace     string `json:"namespace"`
	TokenPath     string `json:"token_path"`
	CAPath        string `json:"ca_path"`
	IsDevelopment bool   `json:"is_development"`
}
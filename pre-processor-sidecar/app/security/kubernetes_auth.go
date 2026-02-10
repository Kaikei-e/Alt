// ABOUTME: Kubernetes ServiceAccount認証機能
// ABOUTME: JWT検証、RBAC権限確認、Pod内認証対応

package security

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// KubernetesAuthenticator はKubernetes認証機能を提供
type KubernetesAuthenticator struct {
	logger           *slog.Logger
	publicKey        *rsa.PublicKey
	serviceAccountCA []byte
	namespace        string

	// 設定
	tokenPath     string
	caPath        string
	namespacePath string
}

// ServiceAccountClaims はServiceAccountトークンのクレーム
type ServiceAccountClaims struct {
	jwt.RegisteredClaims
	Kubernetes KubernetesClaims `json:"kubernetes.io,omitempty"`
}

// KubernetesClaims はKubernetes固有のクレーム
type KubernetesClaims struct {
	Namespace      string                  `json:"namespace"`
	ServiceAccount ServiceAccountReference `json:"serviceaccount"`
	Pod            *PodReference           `json:"pod,omitempty"`
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
		logger.Error("Kubernetes authenticator initialization failed; rejecting admin authentication until fixed", "error", err)
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

// ServiceAccountInfo represents service account information
type ServiceAccountInfo struct {
	Subject   string   `json:"subject"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	UID       string   `json:"uid"`
	Groups    []string `json:"groups,omitempty"`
}

// ValidateKubernetesServiceAccountToken はServiceAccountトークンを検証
func (ka *KubernetesAuthenticator) ValidateKubernetesServiceAccountToken(tokenString string) (*ServiceAccountInfo, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("empty token")
	}

	if ka.publicKey == nil {
		return nil, fmt.Errorf("public key is not initialized")
	}

	// JWTトークンをパース
	token, err := jwt.ParseWithClaims(tokenString, &ServiceAccountClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 署名方法の確認
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return ka.publicKey, nil
	})

	if err != nil {
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

	// 必須クレーム確認
	if claims.Subject == "" {
		return nil, fmt.Errorf("missing subject in token")
	}
	if claims.Kubernetes.Namespace == "" {
		return nil, fmt.Errorf("missing namespace in token")
	}
	if claims.Kubernetes.ServiceAccount.Name == "" {
		return nil, fmt.Errorf("missing service account name in token")
	}

	// Subjectの一貫性確認: system:serviceaccount:<namespace>:<name>
	expectedSubject := fmt.Sprintf("system:serviceaccount:%s:%s", claims.Kubernetes.Namespace, claims.Kubernetes.ServiceAccount.Name)
	if claims.Subject != expectedSubject {
		return nil, fmt.Errorf("invalid service account subject")
	}

	// ServiceAccount情報の構築
	info := &ServiceAccountInfo{
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

// HasAdminPermissions は管理者権限を確認
func (ka *KubernetesAuthenticator) HasAdminPermissions(info *ServiceAccountInfo) bool {
	if info == nil {
		return false
	}

	if info.Namespace == "" || info.Name == "" || info.Subject == "" {
		ka.logger.Warn("Admin access denied due to missing required fields",
			"namespace", info.Namespace,
			"service_account", info.Name)
		return false
	}

	// Strict namespace boundary
	if info.Namespace != ka.namespace {
		ka.logger.Warn("Admin access denied due to namespace mismatch",
			"token_namespace", info.Namespace,
			"expected_namespace", ka.namespace,
			"service_account", info.Name)
		return false
	}

	expectedSubject := fmt.Sprintf("system:serviceaccount:%s:%s", info.Namespace, info.Name)
	if info.Subject != expectedSubject {
		ka.logger.Warn("Admin access denied due to subject mismatch",
			"subject", info.Subject,
			"expected_subject", expectedSubject)
		return false
	}

	if _, ok := ka.allowedAdminServiceAccounts()[info.Name]; ok {
		ka.logger.Info("Admin access granted via service account allowlist",
			"service_account", info.Name,
			"namespace", info.Namespace)
		return true
	}

	if _, ok := ka.allowedAdminSubjects()[info.Subject]; ok {
		ka.logger.Info("Admin access granted via subject allowlist",
			"subject", info.Subject)
		return true
	}

	ka.logger.Warn("Admin access denied",
		"service_account", info.Name,
		"namespace", info.Namespace,
		"subject", info.Subject,
		"groups", info.Groups)

	return false
}

func (ka *KubernetesAuthenticator) allowedAdminServiceAccounts() map[string]struct{} {
	allowed := map[string]struct{}{
		"pre-processor-admin":         {},
		"pre-processor-sidecar-admin": {},
	}

	if raw := strings.TrimSpace(os.Getenv("PRE_PROCESSOR_ADMIN_SERVICE_ACCOUNTS")); raw != "" {
		for _, v := range strings.Split(raw, ",") {
			name := strings.TrimSpace(v)
			if name != "" {
				allowed[name] = struct{}{}
			}
		}
	}

	return allowed
}

func (ka *KubernetesAuthenticator) allowedAdminSubjects() map[string]struct{} {
	allowed := map[string]struct{}{
		fmt.Sprintf("system:serviceaccount:%s:pre-processor-admin", ka.namespace):         {},
		fmt.Sprintf("system:serviceaccount:%s:pre-processor-sidecar-admin", ka.namespace): {},
	}

	if raw := strings.TrimSpace(os.Getenv("PRE_PROCESSOR_ADMIN_SUBJECTS")); raw != "" {
		for _, v := range strings.Split(raw, ",") {
			subject := strings.TrimSpace(v)
			if subject != "" {
				allowed[subject] = struct{}{}
			}
		}
	}

	return allowed
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
func (ka *KubernetesAuthenticator) GetCurrentServiceAccount() (*ServiceAccountInfo, error) {
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
		HasPublicKey:  ka.publicKey != nil,
		HasCA:         len(ka.serviceAccountCA) > 0,
		Namespace:     ka.namespace,
		TokenPath:     ka.tokenPath,
		CAPath:        ka.caPath,
		IsDevelopment: ka.isDevelopmentEnvironment(),
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

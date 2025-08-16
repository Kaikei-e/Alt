// ABOUTME: OAuth2 Secret読み込みサービス - auth-token-manager連携
// ABOUTME: Kubernetes Secretからauth-token-managerが管理するOAuth2トークンを読み込み

package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"pre-processor-sidecar/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// OAuth2SecretService はOAuth2 Secretを管理するサービス
type OAuth2SecretService struct {
	logger      *slog.Logger
	namespace   string
	secretName  string
	
	// Kubernetes API設定
	tokenPath   string
	apiEndpoint string
	httpClient  *http.Client
	
	// Watch機能追加 (恒久対応: 自律的Secret再読み込み)
	clientset      kubernetes.Interface
	watcherMutex   sync.RWMutex
	isWatching     bool
	stopWatchCh    chan struct{}
	secretUpdateCh chan *models.OAuth2Token
}

// OAuth2SecretConfig はOAuth2 Secret設定
type OAuth2SecretConfig struct {
	Namespace  string
	SecretName string
	Logger     *slog.Logger
	// Watch機能用コールバック (恒久対応: 自律的Secret再読み込み)
	OnSecretUpdate func(*models.OAuth2Token) error
}

// OAuth2SecretData はKubernetes SecretのOAuth2トークンデータ構造
type OAuth2SecretData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    string `json:"expires_at"`
	Scope        string `json:"scope"`
}

// NewOAuth2SecretService は新しいOAuth2 Secretサービスを作成
func NewOAuth2SecretService(config OAuth2SecretConfig) (*OAuth2SecretService, error) {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	
	// デフォルト値設定
	if config.Namespace == "" {
		config.Namespace = os.Getenv("KUBERNETES_NAMESPACE")
		if config.Namespace == "" {
			config.Namespace = "alt-processing"
		}
	}
	
	if config.SecretName == "" {
		config.SecretName = os.Getenv("OAUTH2_TOKEN_SECRET_NAME")
		if config.SecretName == "" {
			config.SecretName = "pre-processor-sidecar-oauth2-token"
		}
	}

	// CA証明書を読み込み（Kubernetes内での証明書検証用）
	caCertPath := "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	caCert, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}
	
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	// TLS設定（Kubernetes API用）
	tlsConfig := &tls.Config{
		RootCAs: caCertPool,
	}

	service := &OAuth2SecretService{
		logger:      config.Logger,
		namespace:   config.Namespace,
		secretName:  config.SecretName,
		tokenPath:   "/var/run/secrets/kubernetes.io/serviceaccount/token",
		apiEndpoint: "https://kubernetes.default.svc.cluster.local",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:       tlsConfig,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 20 * time.Second,
			},
		},
	}

	// Kubernetes clientset初期化 (恒久対応: Secret Watch機能)
	if kubeConfig, err := rest.InClusterConfig(); err == nil {
		if clientset, err := kubernetes.NewForConfig(kubeConfig); err == nil {
			service.clientset = clientset
			service.stopWatchCh = make(chan struct{})
			service.secretUpdateCh = make(chan *models.OAuth2Token, 1)
			config.Logger.Info("Kubernetes clientset initialized for Secret watching")
		} else {
			config.Logger.Warn("Failed to create Kubernetes clientset, watch functionality disabled", "error", err)
		}
	} else {
		config.Logger.Warn("Not running in cluster, watch functionality disabled", "error", err)
	}

	config.Logger.Info("OAuth2SecretService initialized",
		"namespace", config.Namespace,
		"secret_name", config.SecretName,
		"watch_enabled", service.clientset != nil)

	return service, nil
}

// LoadOAuth2Token はKubernetes SecretからOAuth2トークンを読み込み
func (s *OAuth2SecretService) LoadOAuth2Token(ctx context.Context) (*models.OAuth2Token, error) {
	// Kubernetes ServiceAccount tokenを読み込み
	tokenBytes, err := ioutil.ReadFile(s.tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service account token: %w", err)
	}
	
	serviceAccountToken := strings.TrimSpace(string(tokenBytes))
	
	// Kubernetes API経由でSecretを取得
	secretData, err := s.getSecretFromKubernetesAPI(ctx, serviceAccountToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret from Kubernetes API: %w", err)
	}
	
	// Secretデータを解析してOAuth2Tokenに変換
	oauth2Token, err := s.parseOAuth2SecretData(secretData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OAuth2 secret data: %w", err)
	}

	s.logger.Info("OAuth2 token loaded successfully from Kubernetes Secret",
		"secret_name", s.secretName,
		"namespace", s.namespace,
		"expires_at", oauth2Token.ExpiresAt,
		"token_type", oauth2Token.TokenType,
		"scope", oauth2Token.Scope)

	return oauth2Token, nil
}

// getSecretFromKubernetesAPI はKubernetes APIからSecretを取得
func (s *OAuth2SecretService) getSecretFromKubernetesAPI(ctx context.Context, serviceAccountToken string) (map[string][]byte, error) {
	// Kubernetes API endpoint URL構築
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/secrets/%s", 
		s.apiEndpoint, s.namespace, s.secretName)
	
	// HTTPリクエスト作成
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	
	// ServiceAccount tokenを認証ヘッダーに設定
	req.Header.Set("Authorization", "Bearer "+serviceAccountToken)
	req.Header.Set("Accept", "application/json")
	
	// HTTPリクエスト実行
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request to Kubernetes API: %w", err)
	}
	defer resp.Body.Close()
	
	// レスポンス確認
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("Kubernetes API request failed with status %d: %s", 
			resp.StatusCode, string(bodyBytes))
	}
	
	// レスポンスボディ読み込み
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Kubernetes Secret構造をパース
	var secretResponse struct {
		Data map[string]string `json:"data"`
	}
	
	if err := json.Unmarshal(bodyBytes, &secretResponse); err != nil {
		return nil, fmt.Errorf("failed to parse Kubernetes Secret response: %w", err)
	}
	
	// Base64デコード
	decodedData := make(map[string][]byte)
	for key, value := range secretResponse.Data {
		decoded, err := base64Decode(value)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 data for key %s: %w", key, err)
		}
		decodedData[key] = decoded
	}
	
	return decodedData, nil
}

// parseOAuth2SecretData はSecretデータをOAuth2Tokenに変換
func (s *OAuth2SecretService) parseOAuth2SecretData(secretData map[string][]byte) (*models.OAuth2Token, error) {
	// auth-token-managerが使用する 'token_data' キーからJSONデータを取得
	tokenDataBytes, exists := secretData["token_data"]
	if !exists {
		return nil, fmt.Errorf("token_data key not found in secret")
	}
	
	// JSONデータを解析
	var oauth2Data OAuth2SecretData
	if err := json.Unmarshal(tokenDataBytes, &oauth2Data); err != nil {
		return nil, fmt.Errorf("failed to parse OAuth2 token JSON: %w", err)
	}
	
	// 必須フィールドの検証
	if oauth2Data.AccessToken == "" {
		return nil, fmt.Errorf("access_token is missing or empty")
	}
	if oauth2Data.RefreshToken == "" {
		return nil, fmt.Errorf("refresh_token is missing or empty")
	}
	
	// 有効期限の解析
	var expiresAt time.Time
	if oauth2Data.ExpiresAt != "" {
		parsedTime, err := time.Parse(time.RFC3339, oauth2Data.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("failed to parse expires_at time %s: %w", oauth2Data.ExpiresAt, err)
		}
		expiresAt = parsedTime
	} else {
		// デフォルトの有効期限（24時間後）
		expiresAt = time.Now().Add(24 * time.Hour)
	}
	
	// OAuth2Token構造体に変換
	token := &models.OAuth2Token{
		AccessToken:  oauth2Data.AccessToken,
		RefreshToken: oauth2Data.RefreshToken,
		TokenType:    oauth2Data.TokenType,
		ExpiresAt:    expiresAt,
		Scope:        oauth2Data.Scope,
	}
	
	// デフォルト値設定
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	if token.Scope == "" {
		token.Scope = "read"
	}
	
	return token, nil
}

// IsTokenExpired はトークンが有効期限切れかどうか確認
func (s *OAuth2SecretService) IsTokenExpired(token *models.OAuth2Token, bufferMinutes int) bool {
	if token == nil {
		return true
	}
	
	buffer := time.Duration(bufferMinutes) * time.Minute
	return time.Now().Add(buffer).After(token.ExpiresAt)
}

// base64Decode はbase64文字列をデコード
func base64Decode(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encoded)
}

// GetSecretInfo はSecret情報を取得（デバッグ用）
func (s *OAuth2SecretService) GetSecretInfo() map[string]interface{} {
	s.watcherMutex.RLock()
	defer s.watcherMutex.RUnlock()
	
	return map[string]interface{}{
		"namespace":     s.namespace,
		"secret_name":   s.secretName,
		"api_endpoint":  s.apiEndpoint,
		"token_path":    s.tokenPath,
		"watch_enabled": s.clientset != nil,
		"is_watching":   s.isWatching,
	}
}

// StartWatching はSecret監視を開始 (恒久対応: 自律的Secret再読み込み)
func (s *OAuth2SecretService) StartWatching(onUpdate func(*models.OAuth2Token) error) error {
	if s.clientset == nil {
		return fmt.Errorf("kubernetes clientset not available")
	}

	s.watcherMutex.Lock()
	if s.isWatching {
		s.watcherMutex.Unlock()
		return fmt.Errorf("already watching secrets")
	}
	s.isWatching = true
	s.watcherMutex.Unlock()

	go s.watchSecretChanges(onUpdate)
	s.logger.Info("Started watching Secret changes",
		"namespace", s.namespace,
		"secret_name", s.secretName)

	return nil
}

// StopWatching はSecret監視を停止
func (s *OAuth2SecretService) StopWatching() {
	s.watcherMutex.Lock()
	defer s.watcherMutex.Unlock()

	if !s.isWatching {
		return
	}

	s.isWatching = false
	close(s.stopWatchCh)
	s.logger.Info("Stopped watching Secret changes")
}

// watchSecretChanges はSecret変更を監視する内部関数
func (s *OAuth2SecretService) watchSecretChanges(onUpdate func(*models.OAuth2Token) error) {
	for {
		select {
		case <-s.stopWatchCh:
			s.logger.Info("Secret watcher stopped")
			return
		default:
		}

		// Secret変更監視開始
		watcher, err := s.clientset.CoreV1().Secrets(s.namespace).Watch(context.Background(), metav1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", s.secretName),
		})
		if err != nil {
			s.logger.Error("Failed to start secret watcher", "error", err)
			time.Sleep(30 * time.Second) // 30秒後にリトライ
			continue
		}

		s.logger.Info("Secret watcher started successfully")

		// イベント処理ループ
		for event := range watcher.ResultChan() {
			switch event.Type {
			case watch.Modified:
				s.logger.Info("Secret modification detected", "event_type", event.Type)
				if err := s.handleSecretUpdate(onUpdate); err != nil {
					s.logger.Error("Failed to handle secret update", "error", err)
				}
			case watch.Error:
				s.logger.Error("Secret watch error", "event", event.Object)
			}
		}

		watcher.Stop()
		s.logger.Warn("Secret watcher connection lost, reconnecting in 10 seconds")
		time.Sleep(10 * time.Second)
	}
}

// handleSecretUpdate はSecret更新を処理
func (s *OAuth2SecretService) handleSecretUpdate(onUpdate func(*models.OAuth2Token) error) error {
	// 更新されたSecretからトークンを読み込み
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := s.LoadOAuth2Token(ctx)
	if err != nil {
		return fmt.Errorf("failed to load updated OAuth2 token: %w", err)
	}

	// コールバック関数でトークン更新を通知
	if onUpdate != nil {
		if err := onUpdate(token); err != nil {
			s.logger.Error("Secret update callback failed", "error", err)
			return err
		}
	}

	s.logger.Info("Secret update processed successfully",
		"expires_at", token.ExpiresAt,
		"scope", token.Scope)

	return nil
}

// GetSecretUpdateChannel はSecret更新チャンネルを取得
func (s *OAuth2SecretService) GetSecretUpdateChannel() <-chan *models.OAuth2Token {
	return s.secretUpdateCh
}
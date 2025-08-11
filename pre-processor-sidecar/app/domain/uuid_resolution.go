// ABOUTME: ドメインサービス - UUID解決の純粋な業務ロジック
// ABOUTME: インフラストラクチャに依存しないピュアなビジネスロジック

package domain

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// UUIDResolutionResult はUUID解決処理の結果を表す不変オブジェクト
type UUIDResolutionResult struct {
	ResolvedCount     int                 `json:"resolved_count"`
	AutoCreatedCount  int                 `json:"auto_created_count"`
	UnknownCount      int                 `json:"unknown_count"`
	TotalProcessed    int                 `json:"total_processed"`
	Errors           []ResolutionError   `json:"errors,omitempty"`
}

// ResolutionError はUUID解決処理中のエラー詳細
type ResolutionError struct {
	ArticleInoreaderID string `json:"article_inoreader_id"`
	OriginStreamID     string `json:"origin_stream_id"`
	ErrorMessage       string `json:"error_message"`
	ErrorCode          string `json:"error_code"`
}

// SubscriptionMapping はスレッドセーフなサブスクリプションマッピング
type SubscriptionMapping struct {
	InoreaderIDToUUID map[string]uuid.UUID
	UUIDToInoreaderID map[uuid.UUID]string
	mu                sync.RWMutex
}

// GetUUID はスレッドセーフにUUIDを取得
func (m *SubscriptionMapping) GetUUID(inoreaderID string) (uuid.UUID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	subscriptionUUID, exists := m.InoreaderIDToUUID[inoreaderID]
	return subscriptionUUID, exists
}

// SetMapping はスレッドセーフにマッピングを設定
func (m *SubscriptionMapping) SetMapping(inoreaderID string, subscriptionUUID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InoreaderIDToUUID[inoreaderID] = subscriptionUUID
	m.UUIDToInoreaderID[subscriptionUUID] = inoreaderID
}

// Size はマッピングのサイズを取得
func (m *SubscriptionMapping) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.InoreaderIDToUUID)
}

// NewSubscriptionMapping は新しいSubscriptionMappingを作成
func NewSubscriptionMapping() *SubscriptionMapping {
	return &SubscriptionMapping{
		InoreaderIDToUUID: make(map[string]uuid.UUID),
		UUIDToInoreaderID: make(map[uuid.UUID]string),
	}
}

// エラー定義
var (
	ErrEmptyOriginStreamID = fmt.Errorf("origin stream ID is empty")
	ErrAutoCreationFailed  = fmt.Errorf("auto creation failed")
	ErrInvalidArticle      = fmt.Errorf("invalid article data")
)

// SubscriptionAutoCreator はサブスクリプション自動作成の抽象化
type SubscriptionAutoCreator interface {
	AutoCreateSubscription(ctx context.Context, originStreamID string) (uuid.UUID, error)
}

// LoggerInterface はログ出力の抽象化
type LoggerInterface interface {
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
}
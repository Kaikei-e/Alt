package fetch_feed_port

import (
	"context"
	"reflect"
	"testing"
	"time"

	"alt/domain"
)

// インターフェース契約のテスト
func TestFetchFeedsPortContract(t *testing.T) {
	tests := []struct {
		name           string
		methodName     string
		expectedParams []reflect.Type
		expectedReturn []reflect.Type
	}{
		{
			name:       "FetchReadFeedsListCursor method exists",
			methodName: "FetchReadFeedsListCursor",
			expectedParams: []reflect.Type{
				reflect.TypeOf((*context.Context)(nil)).Elem(),
				reflect.TypeOf((*time.Time)(nil)),
				reflect.TypeOf(int(0)),
			},
			expectedReturn: []reflect.Type{
				reflect.TypeOf([]*domain.FeedItem(nil)),
				reflect.TypeOf((*error)(nil)).Elem(),
			},
		},
		{
			name:       "FetchFavoriteFeedsListCursor method exists",
			methodName: "FetchFavoriteFeedsListCursor",
			expectedParams: []reflect.Type{
				reflect.TypeOf((*context.Context)(nil)).Elem(),
				reflect.TypeOf((*time.Time)(nil)),
				reflect.TypeOf(int(0)),
			},
			expectedReturn: []reflect.Type{
				reflect.TypeOf([]*domain.FeedItem(nil)),
				reflect.TypeOf((*error)(nil)).Elem(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// FetchFeedsPortインターフェースの型を取得
			portType := reflect.TypeOf((*FetchFeedsPort)(nil)).Elem()

			// メソッドが存在するかチェック
			method, exists := portType.MethodByName(tt.methodName)
			if !exists {
				t.Errorf("Method %s does not exist in FetchFeedsPort interface", tt.methodName)
				return
			}

			// パラメータ数のチェック（インターフェースメソッドはレシーバーなし）
			expectedParamCount := len(tt.expectedParams)
			actualParamCount := method.Type.NumIn()

			if actualParamCount != expectedParamCount {
				t.Errorf("Method %s has %d parameters, expected %d", tt.methodName, actualParamCount, expectedParamCount)
				return
			}

			// パラメータの型チェック
			for i, expectedType := range tt.expectedParams {
				actualType := method.Type.In(i) // インターフェースメソッドなのでインデックス0から
				if actualType != expectedType {
					t.Errorf("Method %s parameter %d has type %v, expected %v", tt.methodName, i, actualType, expectedType)
				}
			}

			// 戻り値数のチェック
			expectedReturnCount := len(tt.expectedReturn)
			actualReturnCount := method.Type.NumOut()

			if actualReturnCount != expectedReturnCount {
				t.Errorf("Method %s has %d return values, expected %d", tt.methodName, actualReturnCount, expectedReturnCount)
				return
			}

			// 戻り値の型チェック
			for i, expectedType := range tt.expectedReturn {
				actualType := method.Type.Out(i)
				if actualType != expectedType {
					t.Errorf("Method %s return value %d has type %v, expected %v", tt.methodName, i, actualType, expectedType)
				}
			}
		})
	}
}

// インターフェース実装の確認テスト
func TestFetchFeedsPortImplementation(t *testing.T) {
	// モックの実装があることを確認
	var _ FetchFeedsPort = (*mockFetchFeedsPort)(nil)
}

// テスト用の簡単なモック実装
type mockFetchFeedsPort struct{}

func (m *mockFetchFeedsPort) FetchFeeds(ctx context.Context, link string) ([]*domain.FeedItem, error) {
	return nil, nil
}

func (m *mockFetchFeedsPort) FetchFeedsList(ctx context.Context) ([]*domain.FeedItem, error) {
	return nil, nil
}

func (m *mockFetchFeedsPort) FetchFeedsListLimit(ctx context.Context, offset int) ([]*domain.FeedItem, error) {
	return nil, nil
}

func (m *mockFetchFeedsPort) FetchFeedsListPage(ctx context.Context, page int) ([]*domain.FeedItem, error) {
	return nil, nil
}

func (m *mockFetchFeedsPort) FetchFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return nil, nil
}

func (m *mockFetchFeedsPort) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return nil, nil
}

func (m *mockFetchFeedsPort) FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return nil, nil
}

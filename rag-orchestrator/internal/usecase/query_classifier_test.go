package usecase

import (
	"testing"
)

func TestClassify_ComparisonKeywords_JP(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"RustとGoの違いは？"},
		{"AIと機械学習の比較"},
		{"React vs Vue"},
		{"量子コンピュータ対古典コンピュータ"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent := c.Classify(nil, tt.query)
			if intent != IntentComparison {
				t.Errorf("expected IntentComparison for %q, got %s", tt.query, intent)
			}
		})
	}
}

func TestClassify_ComparisonKeywords_EN(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"compare Rust and Go"},
		{"difference between AI and ML"},
		{"React vs Angular"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent := c.Classify(nil, tt.query)
			if intent != IntentComparison {
				t.Errorf("expected IntentComparison for %q, got %s", tt.query, intent)
			}
		})
	}
}

func TestClassify_TemporalKeywords_JP(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"最近のAIニュースは？"},
		{"今週のサイバーセキュリティ"},
		{"今日のテクノロジートレンド"},
		{"最新の量子コンピュータ研究"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent := c.Classify(nil, tt.query)
			if intent != IntentTemporal {
				t.Errorf("expected IntentTemporal for %q, got %s", tt.query, intent)
			}
		})
	}
}

func TestClassify_TemporalKeywords_EN(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"latest AI research"},
		{"recent cybersecurity news"},
		{"this week in tech"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent := c.Classify(nil, tt.query)
			if intent != IntentTemporal {
				t.Errorf("expected IntentTemporal for %q, got %s", tt.query, intent)
			}
		})
	}
}

func TestClassify_DeepDiveKeywords_JP(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"量子コンピューティングについて詳しく教えて"},
		{"Rustの所有権システムを深掘りして"},
		{"ブロックチェーンについて教えて"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent := c.Classify(nil, tt.query)
			if intent != IntentTopicDeepDive {
				t.Errorf("expected IntentTopicDeepDive for %q, got %s", tt.query, intent)
			}
		})
	}
}

func TestClassify_DeepDiveKeywords_EN(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"explain transformers in detail"},
		{"tell me about quantum computing"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent := c.Classify(nil, tt.query)
			if intent != IntentTopicDeepDive {
				t.Errorf("expected IntentTopicDeepDive for %q, got %s", tt.query, intent)
			}
		})
	}
}

func TestClassify_FactCheckKeywords_JP(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"量子コンピュータは暗号を解けるって本当？"},
		{"AIが人間を超えるのは事実？"},
		{"Rustはメモリ安全って正しい？"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent := c.Classify(nil, tt.query)
			if intent != IntentFactCheck {
				t.Errorf("expected IntentFactCheck for %q, got %s", tt.query, intent)
			}
		})
	}
}

func TestClassify_FactCheckKeywords_EN(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"is it true that quantum computers can break encryption"},
		{"fact check: AI surpasses humans"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent := c.Classify(nil, tt.query)
			if intent != IntentFactCheck {
				t.Errorf("expected IntentFactCheck for %q, got %s", tt.query, intent)
			}
		})
	}
}

func TestClassify_ArticleScoped_UsesExistingParser(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	query := "Regarding the article: Test Title [articleId: 123e4567-e89b-12d3-a456-426614174000]\n\nQuestion:\nWhat is this about?"
	intent := c.Classify(nil, query)
	if intent != IntentArticleScoped {
		t.Errorf("expected IntentArticleScoped, got %s", intent)
	}
}

func TestClassify_General_FallsThrough(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"AIとは何か"},
		{"Rustのエラーハンドリング"},
		{"hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent := c.Classify(nil, tt.query)
			if intent != IntentGeneral {
				t.Errorf("expected IntentGeneral for %q, got %s", tt.query, intent)
			}
		})
	}
}

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

// --- SubIntent classification tests ---

func TestClassifySubIntent_CritiqueKeywords_JP(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"反論はある？"},
		{"この記事の弱点は？"},
		{"批判的な意見は？"},
		{"問題点を教えて"},
		{"リスクは何？"},
		{"デメリットはある？"},
		{"懸念点は？"},
		{"この手法の課題は？"},
		{"限界はある？"},
		{"欠点を挙げて"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			subIntent := c.ClassifySubIntent(tt.query)
			if subIntent != SubIntentCritique {
				t.Errorf("expected SubIntentCritique for %q, got %s", tt.query, subIntent)
			}
		})
	}
}

func TestClassifySubIntent_CritiqueKeywords_EN(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"are there counterarguments?"},
		{"what are the weaknesses?"},
		{"any limitations?"},
		{"what are the risks?"},
		{"what are the drawbacks?"},
		{"any concerns?"},
		{"criticism of this approach?"},
		{"flaw in this argument?"},
		{"downside of this approach?"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			subIntent := c.ClassifySubIntent(tt.query)
			if subIntent != SubIntentCritique {
				t.Errorf("expected SubIntentCritique for %q, got %s", tt.query, subIntent)
			}
		})
	}
}

func TestClassifySubIntent_OpinionKeywords_JP(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"どう思う？"},
		{"この記事の評価は？"},
		{"意見を教えて"},
		{"見解を聞かせて"},
		{"感想は？"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			subIntent := c.ClassifySubIntent(tt.query)
			if subIntent != SubIntentOpinion {
				t.Errorf("expected SubIntentOpinion for %q, got %s", tt.query, subIntent)
			}
		})
	}
}

func TestClassifySubIntent_OpinionKeywords_EN(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"what do you think about this?"},
		{"your opinion on this approach?"},
		{"assessment of this technology?"},
		{"evaluation of the claims?"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			subIntent := c.ClassifySubIntent(tt.query)
			if subIntent != SubIntentOpinion {
				t.Errorf("expected SubIntentOpinion for %q, got %s", tt.query, subIntent)
			}
		})
	}
}

func TestClassifySubIntent_ImplicationKeywords_JP(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"影響はある？"},
		{"どういう意味？"},
		{"今後どうなる？"},
		{"将来性は？"},
		{"結果はどうなる？"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			subIntent := c.ClassifySubIntent(tt.query)
			if subIntent != SubIntentImplication {
				t.Errorf("expected SubIntentImplication for %q, got %s", tt.query, subIntent)
			}
		})
	}
}

func TestClassifySubIntent_ImplicationKeywords_EN(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"what are the implications?"},
		{"what does this mean for the industry?"},
		{"what is the impact?"},
		{"what are the consequences?"},
		{"going forward, what changes?"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			subIntent := c.ClassifySubIntent(tt.query)
			if subIntent != SubIntentImplication {
				t.Errorf("expected SubIntentImplication for %q, got %s", tt.query, subIntent)
			}
		})
	}
}

func TestClassifySubIntent_NoMatch_ReturnsNone(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	tests := []struct {
		query string
	}{
		{"この記事の要点は？"},
		{"要約して"},
		{"何が書いてある？"},
		{"3行でまとめて"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			subIntent := c.ClassifySubIntent(tt.query)
			if subIntent != SubIntentNone {
				t.Errorf("expected SubIntentNone for %q, got %s", tt.query, subIntent)
			}
		})
	}
}

func TestClassifySubIntent_PriorityConflicts(t *testing.T) {
	c := NewQueryClassifier(nil, 0)
	// Critique > Opinion > Implication
	tests := []struct {
		query    string
		expected SubIntentType
		reason   string
	}{
		{"どういう意味？問題点は？", SubIntentCritique, "Critique wins over Implication"},
		{"影響とリスクは？", SubIntentCritique, "リスク triggers Critique before 影響"},
		{"評価して。弱点はある？", SubIntentCritique, "弱点 triggers Critique before 評価"},
		{"影響は？意見を教えて", SubIntentOpinion, "Opinion wins over Implication"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			subIntent := c.ClassifySubIntent(tt.query)
			if subIntent != tt.expected {
				t.Errorf("%s: expected %s for %q, got %s", tt.reason, tt.expected, tt.query, subIntent)
			}
		})
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

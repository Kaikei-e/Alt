package service

import "testing"

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"empty string is undetermined", "", "und"},
		{"short whitespace only is undetermined", "   ", "und"},
		{"Japanese title with hiragana", "山梨 山林火災 延焼範囲が西側に拡大", "ja"},
		{"Japanese with katakana", "バッテリー技術の最前線", "ja"},
		{"Japanese with CJK only", "電気自動車市場動向", "ja"},
		{"plain English title", "GPU shortage impacts AI training", "en"},
		{"English with numerals", "OpenAI releases o3 in Q1 2026", "en"},
		{"single digit / url is undetermined", "12345", "und"},
		{"mostly English with a loaned Japanese word", "The word 寿司 is popular", "en"},
		{"majority CJK beats ASCII noise", "東京オリンピック 2028 開催地決定", "ja"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectLanguage(tc.text)
			if got != tc.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

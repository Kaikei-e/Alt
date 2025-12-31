package html_parser

import (
	"compress/gzip"
	"context"
	"io"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

func newUTF8Reader(ctx context.Context, r io.Reader, ctype string) (io.Reader, error) {
	// ctx でタイムアウトさせたい場合は http.Request 側で
	if r2, err := charset.NewReaderLabel(ctype, r); err == nil {
		return r2, nil
	} else {
		return nil, err
	}
}

func fallbackText(r io.Reader) string {
	var b strings.Builder
	z := html.NewTokenizer(r)
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.TextToken {
			b.Write(z.Text())
		}
	}
	return b.String()
}

// extractPTagsFromReader extracts p tags from a non-gzip reader.
func extractPTagsFromReader(_ context.Context, r io.Reader) ([]string, error) {
	policy := bluemonday.NewPolicy()
	policy.AllowElements("p", "a", "strong", "em")
	cleaned := policy.SanitizeReader(r)

	doc, err := goquery.NewDocumentFromReader(cleaned)
	if err != nil {
		return []string{fallbackText(cleaned)}, nil
	}
	return doc.Find("p").
		Map(func(_ int, s *goquery.Selection) string {
			return strings.TrimSpace(s.Text())
		}), nil
}

func ExtractPTags(ctx context.Context, htmlR io.Reader, ctype string) ([]string, error) {
	// ① エンコーディング変換
	r, err := newUTF8Reader(ctx, htmlR, ctype)
	if err != nil {
		return nil, err
	}

	// ② gzip 展開 & ③ サイズ制限
	r = io.LimitReader(r, 1<<20)
	gzr, gzErr := gzip.NewReader(r)
	if gzErr != nil {
		// Not gzip encoded, use original reader
		return extractPTagsFromReader(ctx, r)
	}
	defer func() {
		if closeErr := gzr.Close(); closeErr != nil {
			// Log but don't fail - data has been read
			_ = closeErr
		}
	}()

	// ④ Sanitize
	policy := bluemonday.NewPolicy()
	policy.AllowElements("p", "a", "strong", "em")
	cleaned := policy.SanitizeReader(gzr)

	// ⑤ goquery 本体
	doc, err := goquery.NewDocumentFromReader(cleaned)
	if err != nil {
		// goquery が死んだら最弱フォールバック
		return []string{fallbackText(cleaned)}, nil
	}
	return doc.Find("p").
		Map(func(_ int, s *goquery.Selection) string {
			return strings.TrimSpace(s.Text())
		}), nil
}

// StripTags は HTML 文字列からタグを除去し、
// プレーンテキストだけを返すシンプルな関数。
// ・script/style も自動的にスキップ
// ・改行と連続空白を 1 つの空白に正規化する
func StripTags(raw string) string {
	// strings.NewReader を直接渡すだけなのでヒープ圧は低め
	return stripCore(strings.NewReader(raw))
}

// --- 内部実装 -----------------------------------------------------

func stripCore(r io.Reader) string {
	var b strings.Builder
	z := html.NewTokenizer(r)

	depthSkip := 0 // <script> や <style> ブロックを無視するための深さカウンタ

	for {
		switch tt := z.Next(); tt {
		case html.ErrorToken:
			return normalizeWS(b.String())

		case html.StartTagToken:
			name, _ := z.TagName()
			if skipTag(name) {
				depthSkip++
			}

		case html.EndTagToken:
			name, _ := z.TagName()
			if skipTag(name) && depthSkip > 0 {
				depthSkip--
			}

		case html.TextToken:
			if depthSkip == 0 { // script/style 内はスキップ
				b.Write(z.Text())
			}
		}
	}
}

// script,style は “本文” ではないので除外
func skipTag(name []byte) bool {
	switch string(name) {
	case "script", "style", "noscript":
		return true
	default:
		return false
	}
}

// 長い改行・タブ・連続空白を単一スペースにまとめる
func normalizeWS(s string) string {
	// strings.Fields は空白類文字をまとめて扱い便利
	return strings.Join(strings.Fields(s), " ")
}

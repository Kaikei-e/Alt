package domain_test

import (
	"strings"
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeHTML(t *testing.T) {
	t.Run("deeply nested HTML extracts article text and strips tags", func(t *testing.T) {
		// Simulates a news site with deeply nested divs, figure tags, and navigation links
		input := `<div><div><div><div><div><div><div><div><figure><p></p></figure></div>` +
			`<h2>太陽光発電の新技術が実用化へ</h2>` +
			`<div><div><p>2026年4月1日午前10時00分</p>` +
			`<p>(2026年4月1日午後2時00分更新)</p></div>` +
			`<p><a href="/topics/energy">エネルギー</a></p></div></div>` +
			`<div><p>国内の研究チームが開発した次世代太陽光パネルが、従来比2倍の発電効率を達成した。</p>` +
			`<p>研究チームは3年間の実証実験を経て、商用化の目処が立ったと発表した。</p></div></div>` +
			`<div><h3>注目ワード</h3><div><p>` +
			`<a href="/topics/energy">エネルギー</a>` +
			`<a href="/topics/tech">テクノロジー</a></p></div></div></div>` +
			`<div><p><h2>あわせて読みたい</h2></p><ul><li><a href="/articles/12345">` +
			`<div><div><figure><p></p></figure></div><div><p>電力会社の新戦略</p></div></div></a></li></ul></div>`

		result := domain.SanitizeHTML(input)

		// Article headline and body must be present
		assert.Contains(t, result, "太陽光発電の新技術が実用化へ")
		assert.Contains(t, result, "国内の研究チームが開発した次世代太陽光パネルが、従来比2倍の発電効率を達成した")
		assert.Contains(t, result, "研究チームは3年間の実証実験を経て")

		// No HTML tags
		assert.NotContains(t, result, "<div>")
		assert.NotContains(t, result, "<h2>")
		assert.NotContains(t, result, "<p>")
		assert.NotContains(t, result, "<a href")
		assert.NotContains(t, result, "</div>")
		assert.NotContains(t, result, "<figure>")

		// Boilerplate removed
		assert.NotContains(t, result, "注目ワード")
		assert.NotContains(t, result, "あわせて読みたい")
	})

	t.Run("news article HTML preserves link text and strips tags", func(t *testing.T) {
		input := `<p>大規模な太陽フレアが観測され、通信障害の懸念が高まっている。</p>` +
			`<p>専門家は、` +
			`<a href="https://example.com/space/solar-impact">地球への影響が数日中に現れる可能性</a>がある` +
			`と指摘している。</p>`

		result := domain.SanitizeHTML(input)

		assert.Contains(t, result, "大規模な太陽フレアが観測され")
		assert.Contains(t, result, "地球への影響が数日中に現れる可能性")
		assert.NotContains(t, result, "<p>")
		assert.NotContains(t, result, "<a href")
		assert.NotContains(t, result, "example.com")
	})

	t.Run("p tags preserve paragraph structure as newlines", func(t *testing.T) {
		input := `<p>最初の段落です。</p><p>2番目の段落です。</p><p>3番目の段落です。</p>`

		result := domain.SanitizeHTML(input)

		assert.Contains(t, result, "最初の段落です。")
		assert.Contains(t, result, "2番目の段落です。")
		assert.Contains(t, result, "3番目の段落です。")
		// Block elements should produce separate lines
		lines := strings.Split(result, "\n")
		nonEmpty := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				nonEmpty++
			}
		}
		assert.GreaterOrEqual(t, nonEmpty, 3)
	})

	t.Run("plain text passes through unchanged", func(t *testing.T) {
		input := "This is already plain text.\n\nSecond paragraph here."

		result := domain.SanitizeHTML(input)

		assert.Equal(t, input, result)
	})

	t.Run("boilerplate sections removed", func(t *testing.T) {
		input := `<p>本文テキストです。重要な記事内容が書かれています。</p>` +
			`<h3>注目ワード</h3><p>ワード1</p>` +
			`<h2>あわせて読みたい</h2><ul><li>記事1</li></ul>` +
			`<h3>深掘りコンテンツ</h3><p>コンテンツ</p>` +
			`<h3>最新・注目の動画</h3><p>動画</p>` +
			`<h3>天気予報・防災情報</h3><p>天気</p>` +
			`<h3>新着ニュース</h3><p>ニュース</p>` +
			`<h3>各地のニュース</h3><p>地方</p>` +
			`<p>受信契約について詳しく確認する</p>`

		result := domain.SanitizeHTML(input)

		assert.Contains(t, result, "本文テキストです")
		assert.NotContains(t, result, "注目ワード")
		assert.NotContains(t, result, "あわせて読みたい")
		assert.NotContains(t, result, "深掘りコンテンツ")
		assert.NotContains(t, result, "最新・注目の動画")
		assert.NotContains(t, result, "天気予報・防災情報")
		assert.NotContains(t, result, "新着ニュース")
		assert.NotContains(t, result, "各地のニュース")
		assert.NotContains(t, result, "受信契約")
	})

	t.Run("empty string returns empty", func(t *testing.T) {
		assert.Equal(t, "", domain.SanitizeHTML(""))
	})

	t.Run("script and style tags are dropped entirely", func(t *testing.T) {
		input := `<p>Visible text.</p><script>alert('xss')</script>` +
			`<style>.hidden{display:none}</style><noscript>Enable JS</noscript>` +
			`<p>More visible text.</p>`

		result := domain.SanitizeHTML(input)

		assert.Contains(t, result, "Visible text.")
		assert.Contains(t, result, "More visible text.")
		assert.NotContains(t, result, "alert")
		assert.NotContains(t, result, "display:none")
		assert.NotContains(t, result, "Enable JS")
	})

	t.Run("block elements produce line breaks", func(t *testing.T) {
		input := `<h1>Title</h1><p>Paragraph</p><div>Block</div><br/><li>Item</li>`

		result := domain.SanitizeHTML(input)

		assert.Contains(t, result, "Title")
		assert.Contains(t, result, "Paragraph")
		assert.Contains(t, result, "Block")
		assert.Contains(t, result, "Item")
		// Should not be all on one line
		assert.Contains(t, result, "\n")
	})

	t.Run("HTML entities are decoded", func(t *testing.T) {
		input := `<p>Tom &amp; Jerry &lt;3 &quot;cartoons&quot;</p>`

		result := domain.SanitizeHTML(input)

		assert.Contains(t, result, `Tom & Jerry <3 "cartoons"`)
	})

	t.Run("consecutive whitespace is normalized", func(t *testing.T) {
		input := `<p>Text</p>


<p>More</p>



<p>End</p>`

		result := domain.SanitizeHTML(input)

		// No more than 2 consecutive newlines
		assert.NotContains(t, result, "\n\n\n")
	})
}

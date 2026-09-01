package domain_test

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// overlapBudget mirrors the chunker's documented budget rule so the tests pin
// the formula independently of the implementation.
func overlapBudget() int {
	return int(math.Ceil(domain.DefaultOverlapRatio * float64(domain.MaxChunkLength)))
}

// overlapOf returns the text chunk c carried over from its predecessor.
func overlapOf(c domain.Chunk) string {
	return string([]rune(c.Content)[:c.OverlapRunes])
}

// coreOf returns chunk c's own text, without the carried-over overlap.
func coreOf(c domain.Chunk) string {
	return strings.TrimSpace(string([]rune(c.Content)[c.OverlapRunes:]))
}

func TestChunker_Chunk(t *testing.T) {
	chunker := domain.NewChunker()

	t.Run("Splits by paragraphs and merges short ones", func(t *testing.T) {
		// Short paragraphs will be merged due to MinChunkLength
		body := "Paragraph 1.\n\nParagraph 2.\n\nParagraph 3."
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)
		// All three short paragraphs will be merged into one chunk
		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "Paragraph 1.")
		assert.Contains(t, chunks[0].Content, "Paragraph 2.")
		assert.Contains(t, chunks[0].Content, "Paragraph 3.")
		assert.Equal(t, 0, chunks[0].Ordinal)
	})

	t.Run("Ignores empty paragraphs and merges short ones", func(t *testing.T) {
		body := "Para 1\n\n\n\nPara 2"
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)
		// Short paragraphs will be merged
		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "Para 1")
		assert.Contains(t, chunks[0].Content, "Para 2")
	})

	t.Run("Computes stable hash", func(t *testing.T) {
		body := "Content"
		chunks1, _ := chunker.Chunk(body)
		chunks2, _ := chunker.Chunk(body)

		assert.NotEmpty(t, chunks1[0].Hash)
		assert.Equal(t, chunks1[0].Hash, chunks2[0].Hash)
	})

	t.Run("Handles single line", func(t *testing.T) {
		body := "Single line."
		chunks, _ := chunker.Chunk(body)
		assert.Len(t, chunks, 1)
		assert.Equal(t, "Single line.", chunks[0].Content)
	})

	t.Run("Trims whitespace and merges short chunks", func(t *testing.T) {
		body := "  Para 1  \n\n  Para 2  "
		chunks, _ := chunker.Chunk(body)
		// Short paragraphs will be merged
		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "Para 1")
		assert.Contains(t, chunks[0].Content, "Para 2")
	})

	t.Run("Merges short chunks", func(t *testing.T) {
		// Create short paragraphs that should be merged with the following long paragraph
		body := "Short.\n\nAlso short.\n\nThis is a longer paragraph that exceeds the minimum chunk length and should stand alone."
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)

		// Short paragraphs merge forward into the following long paragraph
		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "Short.")
		assert.Contains(t, chunks[0].Content, "Also short.")
		assert.Contains(t, chunks[0].Content, "This is a longer paragraph")
	})

	t.Run("Splits long chunks at sentence boundaries", func(t *testing.T) {
		// Create a very long paragraph
		longText := ""
		for i := 0; i < 20; i++ {
			longText += "This is sentence number " + string(rune('0'+i)) + " in a very long paragraph. "
		}

		chunks, err := chunker.Chunk(longText)
		assert.NoError(t, err)

		// Should split into multiple chunks
		assert.Greater(t, len(chunks), 1)

		// A chunk carries at most one full core plus the overlap budget.
		maxContent := domain.MaxChunkLength + overlapBudget()
		for _, chunk := range chunks {
			assert.LessOrEqual(t, utf8.RuneCountInString(chunk.Content), maxContent)
		}
	})

	t.Run("Handles mixed short and long paragraphs", func(t *testing.T) {
		shortPara := "Tag 1"
		longPara := "This is a very long paragraph that contains enough text to exceed the minimum chunk length requirement. "
		longPara += "It has multiple sentences to ensure proper handling. "
		longPara += "This should remain as a separate chunk because it's long enough."

		body := shortPara + "\n\n" + "Tag 2" + "\n\n" + longPara

		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)

		// Should have merged short tags and kept long paragraph separate
		assert.GreaterOrEqual(t, len(chunks), 1)

		// At least one chunk should be >= MinChunkLength
		hasLongChunk := false
		for _, chunk := range chunks {
			if len(chunk.Content) >= domain.MinChunkLength {
				hasLongChunk = true
				break
			}
		}
		assert.True(t, hasLongChunk)
	})

	t.Run("Returns v10 version", func(t *testing.T) {
		assert.Equal(t, domain.ChunkerVersionV10, chunker.Version())
	})

	t.Run("Handles Japanese sentence boundaries", func(t *testing.T) {
		// Japanese text with 。 as sentence terminator
		body := "これは最初の文です。これは二番目の文です。これは三番目の文です。"
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)
		assert.NotEmpty(t, chunks)
	})

	t.Run("Keeps long article content separate from navigation fragments", func(t *testing.T) {
		// Simulate NHK-style navigation fragments
		body := "注目ワード\n\nあわせて読みたい\n\n" +
			"This is the actual article content with sufficient length. " +
			"It contains multiple sentences and provides meaningful context. " +
			"This should be preserved in the chunks."

		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)

		// Plain-text input: the fragments are not headings, so they merge
		// forward into the article content instead of being discarded.
		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "注目ワード")
		assert.Contains(t, chunks[0].Content, "あわせて読みたい")
		assert.Contains(t, chunks[0].Content, "This is the actual article content")
	})

	t.Run("Merges leading short title with following navigation items", func(t *testing.T) {
		// Simulates title (27 chars) + navigation menu (multiple short items) + first long paragraph
		body := "Short Title Here 1234567890\n\nMenu1\n\nMenu2\n\nMenu3\n\nThis is a long enough paragraph that exceeds the minimum chunk length requirement of 80 characters. It needs to be a bit longer now."
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)

		// All chunks should be >= MinChunkLength
		for i, chunk := range chunks {
			if utf8.RuneCountInString(chunk.Content) < domain.MinChunkLength {
				t.Errorf("Chunk %d is too short (%d chars): %q", i, len(chunk.Content), chunk.Content)
			}
		}

		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "Short Title")
		assert.Contains(t, chunks[0].Content, "Menu1")
		assert.Contains(t, chunks[0].Content, "This is a long enough paragraph")
	})

	t.Run("MinChunkLength is 80", func(t *testing.T) {
		assert.Equal(t, 80, domain.MinChunkLength)
	})

	t.Run("HTML input produces chunks without tags", func(t *testing.T) {
		htmlBody := `<div><h2>見出しテキスト</h2>` +
			`<p>これは記事の本文です。十分な長さのテキストが必要です。このテキストはチャンクの最小文字数を超える長さにしています。</p>` +
			`<p>2番目の段落です。こちらも十分な長さを持つテキストで、チャンク分割のテストに使用します。複数の文を含みます。</p>` +
			`</div>`

		chunks, err := chunker.Chunk(htmlBody)
		assert.NoError(t, err)
		assert.NotEmpty(t, chunks)

		for _, chunk := range chunks {
			assert.NotContains(t, chunk.Content, "<div>")
			assert.NotContains(t, chunk.Content, "<h2>")
			assert.NotContains(t, chunk.Content, "<p>")
			assert.NotContains(t, chunk.Content, "</div>")
			assert.Contains(t, chunk.Content, "見出しテキスト")
		}
	})
}

// --- v10: version identity -------------------------------------------------

func TestChunkerV10_VersionIdentity(t *testing.T) {
	t.Run("default chunker reports v10", func(t *testing.T) {
		assert.Equal(t, domain.ChunkerVersionV10, domain.NewChunker().Version())
	})

	t.Run("v10 differs from v9 so a rebuild is triggered", func(t *testing.T) {
		assert.NotEqual(t, domain.ChunkerVersionV9, domain.ChunkerVersionV10)
	})

	t.Run("non-default configuration reports a distinct version", func(t *testing.T) {
		// A tuned chunker produces different chunk text, so it must not read
		// back as "already up to date" against a default-config corpus.
		tuned, err := domain.NewChunkerWithConfig(domain.ChunkerConfig{OverlapRatio: 0.25})
		require.NoError(t, err)

		assert.NotEqual(t, domain.ChunkerVersionV10, tuned.Version())
		assert.Contains(t, string(tuned.Version()), string(domain.ChunkerVersionV10))

		same, err := domain.NewChunkerWithConfig(domain.ChunkerConfig{OverlapRatio: 0.25})
		require.NoError(t, err)
		assert.Equal(t, tuned.Version(), same.Version(), "version must be a pure function of the config")
	})

	t.Run("explicit default configuration reports the plain v10 version", func(t *testing.T) {
		c, err := domain.NewChunkerWithConfig(domain.DefaultChunkerConfig())
		require.NoError(t, err)
		assert.Equal(t, domain.ChunkerVersionV10, c.Version())
	})

	t.Run("disabled overlap is an explicit, recorded configuration", func(t *testing.T) {
		c, err := domain.NewChunkerWithConfig(domain.ChunkerConfig{OverlapRatio: 0})
		require.NoError(t, err)
		assert.NotEqual(t, domain.ChunkerVersionV10, c.Version())
	})
}

func TestChunkerV10_InvalidConfigFailsFast(t *testing.T) {
	tests := []struct {
		name string
		cfg  domain.ChunkerConfig
	}{
		{"negative overlap", domain.ChunkerConfig{OverlapRatio: -0.1}},
		{"overlap of one", domain.ChunkerConfig{OverlapRatio: 1.0}},
		{"overlap above one", domain.ChunkerConfig{OverlapRatio: 1.5}},
		{"overlap NaN", domain.ChunkerConfig{OverlapRatio: math.NaN()}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := domain.NewChunkerWithConfig(tt.cfg)
			assert.Error(t, err, "invalid config must fail loudly, not be silently clamped")
			assert.Nil(t, c)
		})
	}
}

// --- v10: sentence-boundary overlap ---------------------------------------

func TestChunkerV10_Overlap(t *testing.T) {
	longJapanese := strings.Repeat("これは記事の本文を構成する十分に長い文章です。", 60)

	t.Run("adjacent chunks share a sentence-boundary overlap", func(t *testing.T) {
		chunks, err := domain.NewChunker().Chunk(longJapanese)
		require.NoError(t, err)
		require.Greater(t, len(chunks), 1, "input must be long enough to split")

		for i := 1; i < len(chunks); i++ {
			overlap := overlapOf(chunks[i])
			assert.NotEmpty(t, overlap,
				"chunk %d must start with text carried over from chunk %d", i, i-1)
			assert.True(t, strings.HasSuffix(strings.TrimSpace(chunks[i-1].Content), overlap),
				"the overlap must come from the tail of the previous chunk")
		}
	})

	t.Run("overlap stays within the configured budget", func(t *testing.T) {
		chunks, err := domain.NewChunker().Chunk(longJapanese)
		require.NoError(t, err)
		require.Greater(t, len(chunks), 1)

		for i := 1; i < len(chunks); i++ {
			assert.LessOrEqual(t, chunks[i].OverlapRunes, overlapBudget(),
				"chunk %d overlap exceeds the budget", i)
		}
	})

	t.Run("overlap ends on a sentence boundary when one fits", func(t *testing.T) {
		chunks, err := domain.NewChunker().Chunk(longJapanese)
		require.NoError(t, err)
		require.Greater(t, len(chunks), 1)

		overlap := overlapOf(chunks[1])
		require.NotEmpty(t, overlap)
		assert.True(t, strings.HasSuffix(overlap, "。"),
			"expected whole trailing sentences, got %q", overlap)
	})

	t.Run("zero ratio disables overlap", func(t *testing.T) {
		noOverlap, err := domain.NewChunkerWithConfig(domain.ChunkerConfig{OverlapRatio: 0})
		require.NoError(t, err)

		chunks, err := noOverlap.Chunk(longJapanese)
		require.NoError(t, err)
		require.Greater(t, len(chunks), 1)

		for i := 1; i < len(chunks); i++ {
			assert.Zero(t, chunks[i].OverlapRunes)
		}
	})

	t.Run("a single chunk has no overlap prefix", func(t *testing.T) {
		chunks, err := domain.NewChunker().Chunk("This one paragraph is comfortably longer than the minimum chunk length but far below the maximum.")
		require.NoError(t, err)
		require.Len(t, chunks, 1)
		assert.Zero(t, chunks[0].OverlapRunes)
		assert.Equal(t, "This one paragraph is comfortably longer than the minimum chunk length but far below the maximum.", chunks[0].Content)
	})

	t.Run("overlap never duplicates more than half of a short previous chunk", func(t *testing.T) {
		// "AAAA..." has no sentence terminator, so the fallback tail applies.
		body := strings.Repeat("A", 85) + "\n\n" + strings.Repeat("E", 85)
		chunks, err := domain.NewChunker().Chunk(body)
		require.NoError(t, err)
		require.Len(t, chunks, 2)

		assert.NotZero(t, chunks[1].OverlapRunes)
		assert.LessOrEqual(t, chunks[1].OverlapRunes, 85/2)
	})
}

// --- v10: merge-forward ----------------------------------------------------

func TestChunkerV10_MergeForward(t *testing.T) {
	t.Run("short fragments attach to the following section, not the preceding one", func(t *testing.T) {
		longA := "Alpha " + strings.Repeat("a", 85)
		longE := "Epsilon " + strings.Repeat("e", 85)
		body := longA + "\n\nShort C\n\nShort D\n\n" + longE

		chunks, err := domain.NewChunker().Chunk(body)
		require.NoError(t, err)
		require.Len(t, chunks, 2)

		core := coreOf(chunks[1])
		assert.NotContains(t, chunks[0].Content, "Short C")
		assert.Contains(t, core, "Short C")
		assert.Contains(t, core, "Short D")
		assert.Contains(t, core, "Epsilon")
	})

	t.Run("no source paragraph is dropped", func(t *testing.T) {
		paragraphs := []string{
			"Lead fragment",
			strings.Repeat("b", 120),
			"tiny",
			"another tiny",
			strings.Repeat("c", 200),
			"trailing fragment",
		}
		chunks, err := domain.NewChunker().Chunk(strings.Join(paragraphs, "\n\n"))
		require.NoError(t, err)

		joined := ""
		for _, c := range chunks {
			joined += c.Content + "\n"
		}
		for _, p := range paragraphs {
			assert.Contains(t, joined, p, "paragraph %q vanished from the chunk set", p)
		}
	})

	t.Run("trailing remainder attaches to the previous chunk", func(t *testing.T) {
		body := strings.Repeat("d", 200) + "\n\ntail"
		chunks, err := domain.NewChunker().Chunk(body)
		require.NoError(t, err)
		require.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "tail")
	})

	t.Run("a document shorter than the minimum still yields one chunk", func(t *testing.T) {
		chunks, err := domain.NewChunker().Chunk("tiny")
		require.NoError(t, err)
		require.Len(t, chunks, 1)
		assert.Equal(t, "tiny", chunks[0].Content)
	})

	t.Run("empty body yields no chunks", func(t *testing.T) {
		chunks, err := domain.NewChunker().Chunk("")
		require.NoError(t, err)
		assert.Empty(t, chunks)
	})
}

// --- v10: HTML boilerplate regression -------------------------------------

// broadcasterStyleArticle reproduces the shape of the pages that put raw markup
// and navigation text into ~26% of the v8 embeddings.
const broadcasterStyleArticle = `<article>
<script type="application/json">{"props":{"pageProps":{"tracking":"gtm-XYZ"}}}</script>
<style>.c-nav__list{display:flex}</style>
<h1>台風10号 九州南部に接近</h1>
<figure><img src="/img/typhoon.jpg" alt=""><figcaption>気象衛星の画像</figcaption></figure>
<p>気象庁によりますと、台風10号は1日午前9時、鹿児島県の南の海上を north へゆっくりと進んでいます。中心気圧は960ヘクトパスカル、最大瞬間風速は50メートルに達する見込みです。</p>
<p>気象庁は、暴風や高波、それに土砂災害に警戒するとともに、river の増水にも十分注意するよう呼びかけています。&amp; 交通機関にも影響が出ています。</p>
<h2>注目ワード</h2>
<ul><li>地震速報</li><li>選挙特集</li></ul>
<h2>あわせて読みたい</h2>
<ul><li>去年の台風被害を振り返る</li></ul>
<footer><p>受信契約のご案内</p></footer>
</article>`

func TestChunkerV10_HTMLBoilerplateNeverSurvives(t *testing.T) {
	chunks, err := domain.NewChunker().Chunk(broadcasterStyleArticle)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)

	joined := ""
	for _, c := range chunks {
		joined += c.Content + "\n"
	}

	t.Run("article body survives", func(t *testing.T) {
		assert.Contains(t, joined, "気象庁")
		assert.Contains(t, joined, "960ヘクトパスカル")
	})

	t.Run("markup never survives", func(t *testing.T) {
		for _, frag := range []string{"<p>", "</p>", "<article", "<ul>", "<li>", "<script", "<style", "<img", "class="} {
			assert.NotContains(t, joined, frag)
		}
	})

	t.Run("script and style payloads never survive", func(t *testing.T) {
		assert.NotContains(t, joined, "pageProps")
		assert.NotContains(t, joined, "gtm-XYZ")
		assert.NotContains(t, joined, "c-nav__list")
		assert.NotContains(t, joined, "display:flex")
	})

	t.Run("navigation sections never survive", func(t *testing.T) {
		assert.NotContains(t, joined, "注目ワード")
		assert.NotContains(t, joined, "あわせて読みたい")
		assert.NotContains(t, joined, "地震速報")
		assert.NotContains(t, joined, "選挙特集")
		assert.NotContains(t, joined, "去年の台風被害")
	})

	t.Run("figure captions and licence footers never survive", func(t *testing.T) {
		assert.NotContains(t, joined, "気象衛星の画像")
		assert.NotContains(t, joined, "受信契約")
	})

	t.Run("entities are decoded", func(t *testing.T) {
		assert.NotContains(t, joined, "&amp;")
		assert.Contains(t, joined, "& 交通機関")
	})
}

func TestChunkerV10_LicenceFooterWithoutTriggerHeading(t *testing.T) {
	// The trigger-heading rule cannot help here: the boilerplate line stands
	// on its own after the article body.
	body := `<div>
<p>` + strings.Repeat("本文が十分な長さになるように繰り返します。", 8) + `</p>
<p>受信契約について</p>
</div>`

	chunks, err := domain.NewChunker().Chunk(body)
	require.NoError(t, err)

	joined := ""
	for _, c := range chunks {
		joined += c.Content + "\n"
	}
	assert.Contains(t, joined, "本文が十分な長さ")
	assert.NotContains(t, joined, "受信契約")
}

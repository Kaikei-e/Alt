package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParagraphFlusher_ParagraphBoundary(t *testing.T) {
	f := NewParagraphFlusher(80, 1500*time.Millisecond, 80)

	// Feed text with paragraph break
	text := strings.Repeat("あ", 60) + "\n\n" + strings.Repeat("い", 30)
	flush, ok := f.Feed(text)
	require.True(t, ok, "should flush at paragraph boundary")
	assert.Equal(t, strings.Repeat("あ", 60)+"\n\n", flush)
}

func TestParagraphFlusher_JapaneseSentence(t *testing.T) {
	f := NewParagraphFlusher(80, 1500*time.Millisecond, 80)

	// Feed 80+ runes ending with 。
	text := strings.Repeat("あ", 79) + "。"
	flush, ok := f.Feed(text)
	require.True(t, ok, "should flush at Japanese sentence end with 80+ runes")
	assert.Equal(t, text, flush)
}

func TestParagraphFlusher_MinRunesRespected(t *testing.T) {
	f := NewParagraphFlusher(80, 1500*time.Millisecond, 80)

	// Feed < 80 runes with sentence end — should NOT flush
	text := strings.Repeat("あ", 50) + "。"
	flush, ok := f.Feed(text)
	assert.False(t, ok, "should not flush under minRunes")
	assert.Empty(t, flush)
}

func TestParagraphFlusher_EnglishSentence(t *testing.T) {
	f := NewParagraphFlusher(80, 1500*time.Millisecond, 80)

	text := strings.Repeat("a", 79) + "."
	flush, ok := f.Feed(text)
	require.True(t, ok, "should flush at English sentence end with 80+ runes")
	assert.Equal(t, text, flush)
}

func TestParagraphFlusher_TimeFlush(t *testing.T) {
	f := NewParagraphFlusher(80, 1500*time.Millisecond, 80)

	// Feed 80+ runes without sentence boundary
	text := strings.Repeat("あ", 90)
	_, ok := f.Feed(text)
	assert.False(t, ok, "no boundary, should not flush on Feed")

	// Simulate time passage by backdating lastFlushAt
	f.lastFlushAt = time.Now().Add(-2 * time.Second)

	flush, ok := f.TimeFlush()
	require.True(t, ok, "should flush after timeout with 80+ runes")
	assert.Equal(t, text, flush)
}

func TestParagraphFlusher_TimeFlush_NotEnoughRunes(t *testing.T) {
	f := NewParagraphFlusher(80, 1500*time.Millisecond, 80)

	// Feed < 80 runes
	text := strings.Repeat("あ", 30)
	f.Feed(text)
	f.lastFlushAt = time.Now().Add(-2 * time.Second)

	_, ok := f.TimeFlush()
	assert.False(t, ok, "should not time-flush with < 80 runes")
}

func TestParagraphFlusher_Drain(t *testing.T) {
	f := NewParagraphFlusher(80, 1500*time.Millisecond, 80)

	text := strings.Repeat("あ", 50)
	f.Feed(text)

	drained := f.Drain()
	assert.Equal(t, text, drained)

	// After drain, pending should be empty
	assert.Empty(t, f.Drain())
}

func TestParagraphFlusher_MultipleFeedCalls(t *testing.T) {
	f := NewParagraphFlusher(80, 1500*time.Millisecond, 80)

	// Feed in small chunks, accumulating to 80+ runes
	for i := 0; i < 8; i++ {
		f.Feed(strings.Repeat("あ", 10))
	}
	// Now feed a sentence end
	flush, ok := f.Feed("。")
	require.True(t, ok, "should flush after accumulating 80+ runes + sentence end")
	assert.Len(t, []rune(flush), 81) // 80 あ + 。
}

func TestParagraphFlusher_ParagraphBoundary_FlushesEvenUnderMinRunes(t *testing.T) {
	f := NewParagraphFlusher(80, 1500*time.Millisecond, 80)

	// Paragraph boundary should flush regardless of minRunes
	text := "Short.\n\nNext."
	flush, ok := f.Feed(text)
	require.True(t, ok, "paragraph boundary should flush even under minRunes")
	assert.Equal(t, "Short.\n\n", flush)
}

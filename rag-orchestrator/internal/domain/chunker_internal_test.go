package domain

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitSentences_IsLossless(t *testing.T) {
	// Losslessness is what lets the chunker promise that no source text
	// disappears: every core is a concatenation of these segments.
	inputs := []string{
		"",
		"one sentence with no terminator",
		"First. Second! Third?",
		"これは最初の文です。これは二番目の文です。",
		"「引用です。」と述べた。次の文。",
		"Pi is 3.14 and the U.S. rate stayed flat. Then it moved.\n\nNew paragraph.",
		"trailing whitespace   ",
	}

	for _, in := range inputs {
		assert.Equal(t, in, strings.Join(splitSentences(in), ""), "input %q", in)
	}
}

func TestSplitSentences_JapaneseNeedsNoTrailingSpace(t *testing.T) {
	got := splitSentences("これは最初の文です。これは二番目の文です。")
	require.Len(t, got, 2, "a Japanese terminator ends a sentence on its own")
	assert.Equal(t, "これは最初の文です。", got[0])
}

func TestSplitSentences_KeepsDecimalsAndAbbreviations(t *testing.T) {
	got := splitSentences("Pi is 3.14 exactly. Next.")
	require.Len(t, got, 2)
	assert.Equal(t, "Pi is 3.14 exactly.", strings.TrimSpace(got[0]))
}

func TestSplitSentences_KeepsClosingPunctuationWithItsSentence(t *testing.T) {
	got := splitSentences("「もう限界です。」と話した。")
	require.Len(t, got, 2)
	assert.Equal(t, "「もう限界です。」", got[0])
}

func TestOverlapTail_TakesWholeTrailingSentences(t *testing.T) {
	prev := strings.Repeat("十分に長い文章がここに続きます。", 12) // 16 runes each
	tail := overlapTail(prev, 50)

	assert.True(t, strings.HasSuffix(prev, tail), "the tail must be a suffix of the previous chunk")
	assert.True(t, strings.HasSuffix(tail, "。"), "expected whole sentences, got %q", tail)
	assert.LessOrEqual(t, utf8.RuneCountInString(tail), 50)
	assert.NotEmpty(t, tail)
}

func TestOverlapTail_FallsBackToWordBoundaryWhenNoSentenceFits(t *testing.T) {
	prev := "a single unterminated clause that is far longer than the overlap budget allows"
	tail := overlapTail(prev, 20)

	require.NotEmpty(t, tail)
	assert.True(t, strings.HasSuffix(prev, tail))
	assert.LessOrEqual(t, utf8.RuneCountInString(tail), 20)
	assert.False(t, strings.HasPrefix(tail, " "))
	assert.NotContains(t, strings.Fields(prev)[0], tail)
}

func TestOverlapTail_ZeroBudgetYieldsNothing(t *testing.T) {
	assert.Empty(t, overlapTail("some previous chunk text.", 0))
}

func TestOverlapBudget_NeverExceedsHalfOfThePreviousChunk(t *testing.T) {
	assert.Equal(t, 0, overlapBudget("anything", 0))
	assert.Equal(t, 5, overlapBudget(strings.Repeat("x", 10), DefaultOverlapRatio))
	assert.Equal(t, 150, overlapBudget(strings.Repeat("x", MaxChunkLength), DefaultOverlapRatio))
}

func TestMergeForward_KeepsEveryParagraph(t *testing.T) {
	paragraphs := []string{"tiny", "also tiny", strings.Repeat("x", 100), "trailer"}
	groups := mergeForward(paragraphs)

	joined := strings.Join(groups, "\n\n")
	for _, p := range paragraphs {
		assert.Contains(t, joined, p)
	}
	assert.NotEmpty(t, groups)
}

func TestMergeForward_EmptyInputYieldsNoGroups(t *testing.T) {
	assert.Empty(t, mergeForward(nil))
}

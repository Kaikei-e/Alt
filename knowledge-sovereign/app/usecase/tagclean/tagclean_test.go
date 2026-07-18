package tagclean

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalize_LowercasesAndTrims(t *testing.T) {
	assert.Equal(t, "rust", Normalize(" Rust "))
	assert.Equal(t, "io_uring", Normalize("io_uring"))
}

func TestNormalize_DropsEnglishStopwords(t *testing.T) {
	for _, junk := range []string{"also", "could", "might", "said", "wrote", "would", "becomes", "without", "even", "says", "great"} {
		assert.Empty(t, Normalize(junk), "stopword %q must normalize to empty", junk)
	}
}

func TestNormalize_DropsDigitOnlyAndShortTags(t *testing.T) {
	for _, junk := range []string{"5", "2025", "12", "a", "ー"} {
		assert.Empty(t, Normalize(junk), "%q must normalize to empty", junk)
	}
}

func TestNormalize_DropsURLDebris(t *testing.T) {
	for _, junk := range []string{"https", "http", "www", "com", "gt", "amp"} {
		assert.Empty(t, Normalize(junk), "URL debris %q must normalize to empty", junk)
	}
}

func TestNormalize_DropsJapaneseFunctionWords(t *testing.T) {
	for _, junk := range []string{"こと", "もの", "ため", "よう"} {
		assert.Empty(t, Normalize(junk), "JA function word %q must normalize to empty", junk)
	}
}

func TestNormalize_KeepsSubstantiveTags(t *testing.T) {
	for _, keep := range []string{"military", "submarine", "postgresql", "機械学習"} {
		assert.NotEmpty(t, Normalize(keep), "substantive tag %q must survive", keep)
	}
}

func TestCleanDisplay_DropsJunkAndDeduplicatesCaseVariants(t *testing.T) {
	got := CleanDisplay([]string{"Rust", "also", "5", "rust", "https", "military"})
	assert.Equal(t, []string{"rust", "military"}, got)
}

func TestCleanDisplay_MergesNaiveSingularPluralPairs(t *testing.T) {
	got := CleanDisplay([]string{"agents", "agent", "submarines"})
	// When both forms are present the singular wins; a lone plural stays.
	assert.Equal(t, []string{"agent", "submarines"}, got)
}

func TestCleanDisplay_PreservesFirstSeenOrder(t *testing.T) {
	got := CleanDisplay([]string{"korea", "military", "korea", "crypto"})
	assert.Equal(t, []string{"korea", "military", "crypto"}, got)
}

func TestCleanDisplay_EmptyInputYieldsEmpty(t *testing.T) {
	assert.Empty(t, CleanDisplay(nil))
	assert.Empty(t, CleanDisplay([]string{"also", "5"}))
}

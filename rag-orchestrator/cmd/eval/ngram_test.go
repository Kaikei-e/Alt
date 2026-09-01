package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueryNGrams_CJKTrigrams(t *testing.T) {
	grams := queryNGrams("量子計算")
	assert.Equal(t, []string{"量子計", "子計算"}, grams)
}

func TestQueryNGrams_ShortCJKRunKeptWhole(t *testing.T) {
	assert.Equal(t, []string{"暗号"}, queryNGrams("暗号"))
}

func TestQueryNGrams_LatinWordsKeptWhole(t *testing.T) {
	grams := queryNGrams("How does JWT work?")
	assert.Equal(t, []string{"How", "does", "JWT", "work"}, grams)
}

func TestQueryNGrams_MixedScriptSplitsAtBoundary(t *testing.T) {
	grams := queryNGrams("JWT認証の実装")
	assert.Contains(t, grams, "JWT")
	assert.Contains(t, grams, "認証の")
	assert.NotContains(t, grams, "JWT認証")
}

func TestQueryNGrams_DropsPunctuationAndDuplicates(t *testing.T) {
	grams := queryNGrams("abc, abc; abc")
	assert.Equal(t, []string{"abc"}, grams)
	assert.Empty(t, queryNGrams("?? ,, --"))
}

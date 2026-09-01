package eval

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// The committed golden file is fully synthetic: this repository is public, so
// no production query, title or article id may live in it. Cases mined from the
// real corpus are generated into an untracked local file and reached through
// GoldenCasesPathEnv.
const syntheticGoldenPath = "testdata/golden_cases.json"

func loadSyntheticCases(t *testing.T) []GoldenCase {
	t.Helper()
	cases, err := LoadGoldenCases(syntheticGoldenPath)
	require.NoError(t, err)
	return cases
}

func TestGoldenSet_SizeAndUniqueness(t *testing.T) {
	cases := loadSyntheticCases(t)
	assert.GreaterOrEqual(t, len(cases), 45, "golden set must be large enough to separate stacks")

	seen := make(map[string]bool, len(cases))
	for _, c := range cases {
		assert.False(t, seen[c.ID], "duplicate case id %q", c.ID)
		seen[c.ID] = true
		assert.NotEmpty(t, c.Query, "case %q has no query", c.ID)
	}
}

func TestGoldenSet_CategoriesAreKnown(t *testing.T) {
	cases := loadSyntheticCases(t)
	for _, c := range cases {
		assert.Contains(t, KnownCategories, c.Category, "case %q has unknown category %q", c.ID, c.Category)
	}
}

func TestGoldenSet_EveryCategoryIsExercised(t *testing.T) {
	cases := loadSyntheticCases(t)
	present := make(map[string]int, len(KnownCategories))
	for _, c := range cases {
		present[c.Category]++
	}
	for _, category := range KnownCategories {
		assert.Greater(t, present[category], 0, "category %q has no case", category)
	}
}

func TestGoldenSet_CrossLingualCoverage(t *testing.T) {
	cases := loadSyntheticCases(t)

	crossLingual := 0
	for _, c := range cases {
		if !c.IsCrossLingual() {
			continue
		}
		crossLingual++
		assert.NotEmpty(t, c.Expected.RelevantArticleIDs,
			"cross-lingual case %q needs article-level ground truth to measure recall", c.ID)
	}
	assert.GreaterOrEqual(t, crossLingual, 8, "cross-lingual recall decides the embedder choice")
}

func TestGoldenSet_ArticleIDsAreWellFormed(t *testing.T) {
	cases := loadSyntheticCases(t)
	for _, c := range cases {
		ids := append([]string{}, c.Expected.RelevantArticleIDs...)
		ids = append(ids, c.Expected.ExpectedCitationArticleIDs...)
		ids = append(ids, c.Expected.ForbiddenArticleIDs...)
		if c.ArticleScope != nil {
			ids = append(ids, c.ArticleScope.ArticleID)
			assert.NotEmpty(t, c.ArticleScope.Title, "article-scoped case %q needs a title", c.ID)
		}
		for _, id := range ids {
			assert.Regexp(t, uuidPattern, id, "case %q references a malformed article id", c.ID)
		}
	}
}

func TestGoldenSet_CitationTargetsAreAlsoRelevant(t *testing.T) {
	cases := loadSyntheticCases(t)
	for _, c := range cases {
		relevant := make(map[string]bool, len(c.Expected.RelevantArticleIDs))
		for _, id := range c.Expected.RelevantArticleIDs {
			relevant[id] = true
		}
		for _, id := range c.Expected.ExpectedCitationArticleIDs {
			assert.True(t, relevant[id],
				"case %q expects citation of %s but does not list it as relevant", c.ID, id)
		}
	}
}

func TestGoldenSet_RelevantAndForbiddenAreDisjoint(t *testing.T) {
	cases := loadSyntheticCases(t)
	for _, c := range cases {
		overlap := ForbiddenHits(c.Expected.RelevantArticleIDs, c.Expected.ForbiddenArticleIDs)
		assert.Empty(t, overlap, "case %q lists the same article as relevant and forbidden", c.ID)
	}
}

func TestGoldenSet_NoAnswerCasesForbidCitations(t *testing.T) {
	cases := loadSyntheticCases(t)

	noAnswer := 0
	for _, c := range cases {
		if !c.Expected.ExpectNoAnswer {
			continue
		}
		noAnswer++
		assert.False(t, c.Expected.RequiresCitations,
			"case %q cannot both expect no answer and require citations", c.ID)
		assert.Empty(t, c.Expected.RelevantArticleIDs,
			"case %q expects no answer, so it must not name relevant articles", c.ID)
	}
	assert.GreaterOrEqual(t, noAnswer, 3, "the set needs negative controls against fabricated citations")
}

func TestResolveGoldenCasesPath(t *testing.T) {
	assert.Equal(t, syntheticGoldenPath, ResolveGoldenCasesPath(syntheticGoldenPath))

	local := filepath.Join(t.TempDir(), "golden_cases.local.json")
	t.Setenv(GoldenCasesPathEnv, local)
	assert.Equal(t, local, ResolveGoldenCasesPath(syntheticGoldenPath))
}

func TestLoadGoldenCases_EmptyFileIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	require.NoError(t, os.WriteFile(path, []byte("[]"), 0o600))
	_, err := LoadGoldenCases(path)
	assert.Error(t, err)
}

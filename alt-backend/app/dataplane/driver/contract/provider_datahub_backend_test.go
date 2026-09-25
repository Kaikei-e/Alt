//go:build contract

package contract

import (
	"path/filepath"
	"testing"
)

// TestVerifyAltBackendDataHubContract and TestVerifyAltHarvesterDataHubContract
// verify the data-hub in-family capabilities (ADR-000954 D3, catalog §2.A /
// §2.D / §2.E / §2.L / §2.O).
//
// These two consumers are unusual only in that they are built from this same
// Go module. That is the reason to pin them, not a reason to skip them: a
// shared module lets a message and a handler change together and still
// compile, while the three binaries ship as three containers and roll
// independently. "It builds" and "the deployed provider answers what the
// deployed consumer sends" are different claims, and only the second one is
// what breaks in production.
//
// Two pacticipants rather than one because the two binaries call disjoint
// halves of the surface — the harvester drives the outbox and the retention
// jobs, the backend drives the article-serving reads. Publishing both under
// one name would let a harvester-only break verify green.
func TestVerifyAltBackendDataHubContract(t *testing.T) {
	verifyConsumer(t, "alt-backend", dataHubProviderName,
		filepath.Join(altBackendPactDir, altBackendDataHubPactFile),
		withStates(noopStates(
			"alt-data-hub has a scraped article head",
			"alt-data-hub has no article head for the article",
			"alt-data-hub has og images for the articles",
			"alt-data-hub has a live image proxy cache entry",
			"alt-data-hub has no live image proxy cache entry",
			"alt-data-hub accepts image proxy cache writes",
			"alt-data-hub has a scraping domain",
			"alt-data-hub has no scraping domain for the host",
			"alt-data-hub accepts declined domain writes",
			"alt-data-hub has a declined domain for the user",
			"alt-data-hub has subscribers for the feed link",
			"alt-data-hub has the article for the user",

			// Article catalog, archive and knowledge backfill capabilities (catalog §2.B / §2.C / §2.N).
			"alt-data-hub accepts article archives",
			"alt-data-hub accepts article head writes",
			"alt-data-hub has no article for the url",
			"alt-data-hub has articles for the user",
			"alt-data-hub has articles for the feed",
			"alt-data-hub has historic articles to replay",
			"alt-data-hub has summary versions to replay",

			// Feed and feed link capabilities (catalog §2.F / §2.G / §2.H).
			"alt-data-hub accepts feed link registrations",
			"alt-data-hub accepts bulk feed link registrations",
			"alt-data-hub has feed links",
			"alt-data-hub has a feed link that has never been polled",
			"alt-data-hub has no feed link for the url",
			"alt-data-hub has unread feeds for the user",
			"alt-data-hub has favorite feeds past the og image retention window",
			"alt-data-hub has feeds",
			"alt-data-hub has no feeds",
			"alt-data-hub has feeds for the feed link",
			"alt-data-hub has no summary for the article",
			"alt-data-hub has a summary for the article url",
			"alt-data-hub has feeds matching the title query for the user",
			"alt-data-hub has no tagged feeds",
			"alt-data-hub has feeds for the articles",
			"alt-data-hub has imported inoreader articles for the urls",

			// Read state and tag read capabilities (catalog §2.I / §2.J).
			"alt-data-hub has a feed at the url",
			"alt-data-hub has no feed at the url",
			"alt-data-hub has a feed for the article url",
			"alt-data-hub has read marks for the user",
			"alt-data-hub has subscriptions for the user",
			"alt-data-hub accepts subscription writes",
			"alt-data-hub has no tags for the article",
			"alt-data-hub has tags for the feed",
			"alt-data-hub accepts article tag writes",
			"alt-data-hub has tag cooccurrences",
			"alt-data-hub has tags matching the prefix",
			"alt-data-hub has tagged articles for the user in the window",

			// Versioned artifact and stats capabilities (catalog §2.K / §2.M).
			"alt-data-hub accepts summary version appends",
			"alt-data-hub has an earlier summary version for the article",
			"alt-data-hub has no earlier summary version for the article",
			"alt-data-hub has a superseded summary version",
			"alt-data-hub has a current summary version for the article",
			"alt-data-hub accepts tag set version appends",
			"alt-data-hub has an earlier tag set version for the article",
			"alt-data-hub has a tag set version",
			"alt-data-hub accepts article summary writes",
			// "has feeds" and "has articles for the user" are already declared
			// by the article and feed capabilities above; the §2.M counts reuse them rather than
			// inventing a second name for the same precondition.
			"alt-data-hub has summarized articles for the user",
			"alt-data-hub has unsummarized articles for the user",
			"alt-data-hub has unread feeds for the user since the bound",
			"alt-data-hub has trend data for the user",
			"alt-data-hub has read state for the user",

			// Tag Trail and article reference capabilities (catalog §2.J / §2.C) — the last two.
			"alt-data-hub has articles carrying the feed tag",
			"alt-data-hub has articles carrying the tag name across feeds",
			"alt-data-hub has the article",
			"alt-data-hub has no article with that id",

			// Web Push: the subscription registry and the dispatcher's half of
			// the delivery queue.
			"alt-data-hub accepts push subscription writes",
			"alt-data-hub has a push subscription for the user",
			"alt-data-hub has no push subscription for the endpoint",
			"alt-data-hub has push subscriptions for the user",
			"alt-data-hub accepts notification enqueues",
			"alt-data-hub has due push deliveries",
			"alt-data-hub has a claimed push delivery",

			// On-demand OG image resolution, which replaced the batch backfill.
			"alt-data-hub holds one unresolved feed and one whose origin already refused",
			"alt-data-hub holds no feed_og_images row for the requested feed",
			"alt-data-hub accepts feed og image resolutions",
			"alt-data-hub accepts feed og image refusals",
			"alt-data-hub holds feed og images past the retention window",
		), withStates(backlogStates(), withStates(deepHealthStates(), readStateStates()))), true)
}

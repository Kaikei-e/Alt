// Package trail_episodes derives the trail spine's default display unit
// (D24/D30): the episode. An episode is a PURE derivation over collapsed
// footprints — never a stored entity, never a new event. Deriving twice from
// the same input always yields the same episodes (reproject-safe).
//
// Two stages fold footprints into episodes:
//
//  1. Same-article contacts (any verb, any day) always join one episode —
//     unconditionally, no threshold.
//  2. Cross-article contacts chain into one episode when their cleaned tags
//     (usecase/tagclean) overlap by at least minSharedTags AND their nearest
//     contacts fall within tagChainWindow. Precision over recall: a single
//     shared tag, or a gap outside the window, keeps articles in separate
//     episodes.
package trail_episodes

import (
	"sort"
	"time"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/tagclean"
)

// tagChainWindow bounds how far apart (by nearest contact) two articles'
// footprint groups may be and still chain via tag overlap (stage 2).
// Same-article contacts (stage 1) are exempt — they always join regardless
// of elapsed time.
const tagChainWindow = 14 * 24 * time.Hour

// minSharedTags is the number of cleaned tags two articles' footprint groups
// must share, within tagChainWindow, to chain into one episode (stage 2).
const minSharedTags = 2

// wearRank orders path-wear bands from shallowest to deepest. An unrecognized
// or empty band ranks with "thin" (unknown treats as thin).
var wearRank = map[string]int{"thin": 1, "worn": 2, "deep": 3}

// Episode is one derived line of inquiry: a set of footprints that belong
// together because they touch the same article, or because they chain
// across articles via cleaned-tag overlap. It carries no state of its own —
// every field is computed from its member footprints.
type Episode struct {
	// EpisodeKey is deterministic: "ep:" + the oldest member's FootprintKey.
	EpisodeKey string
	// Wear is the deepest wear band among the member footprints.
	Wear string
	// Footprints are the member rows, newest contact first.
	Footprints []sovereign_db.TrailFootprint
}

// Derive groups footprints (as GetTrailFootprints returns them: collapsed,
// newest-first) into episodes, newest latest-contact first.
func Derive(footprints []sovereign_db.TrailFootprint) []Episode {
	if len(footprints) == 0 {
		return nil
	}

	groups := groupByItem(footprints)

	// Union-find over the per-article groups: stage 1 (same item_key) is
	// already folded by groupByItem; stage 2 chains articles pairwise by
	// cleaned-tag overlap within the window. Transitive chains (A-B, B-C)
	// merge into one episode even when A and C alone would not qualify —
	// that is the "chaining" a union-find naturally gives us.
	parent := make([]int, len(groups))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := 0; i < len(groups); i++ {
		for j := i + 1; j < len(groups); j++ {
			ri, rj := find(i), find(j)
			if ri == rj {
				continue
			}
			if chains(groups[i], groups[j]) {
				parent[ri] = rj
			}
		}
	}

	clusters := make(map[int][]*itemGroup, len(groups))
	for i, g := range groups {
		root := find(i)
		clusters[root] = append(clusters[root], g)
	}

	episodes := make([]Episode, 0, len(clusters))
	for _, members := range clusters {
		episodes = append(episodes, buildEpisode(members))
	}

	sort.Slice(episodes, func(a, b int) bool {
		la, lb := episodes[a].Footprints[0], episodes[b].Footprints[0]
		if !la.OccurredAt.Equal(lb.OccurredAt) {
			return la.OccurredAt.After(lb.OccurredAt)
		}
		return la.FootprintKey > lb.FootprintKey
	})
	return episodes
}

// itemGroup is stage 1: every footprint for one item_key, unconditionally
// joined. earliest/latest span all of the group's contacts (a collapsed row
// with ContactCount > 1 already spans FirstOccurredAt..OccurredAt).
type itemGroup struct {
	footprints []sovereign_db.TrailFootprint // newest first, preserved from input order
	tags       map[string]struct{}           // cleaned, deduplicated
	earliest   time.Time
	latest     time.Time
}

// groupByItem folds footprints sharing item_key into one group each (stage
// 1), preserving first-seen order for deterministic downstream processing.
func groupByItem(footprints []sovereign_db.TrailFootprint) []*itemGroup {
	byItem := make(map[string]*itemGroup, len(footprints))
	order := make([]string, 0, len(footprints))
	for _, f := range footprints {
		g, ok := byItem[f.ItemKey]
		if !ok {
			g = &itemGroup{tags: make(map[string]struct{})}
			byItem[f.ItemKey] = g
			order = append(order, f.ItemKey)
		}
		g.footprints = append(g.footprints, f)
		for _, tag := range f.Tags {
			if cleaned := tagclean.Normalize(tag); cleaned != "" {
				g.tags[cleaned] = struct{}{}
			}
		}
		first := effectiveFirst(f)
		if g.earliest.IsZero() || first.Before(g.earliest) {
			g.earliest = first
		}
		if g.latest.IsZero() || f.OccurredAt.After(g.latest) {
			g.latest = f.OccurredAt
		}
	}

	groups := make([]*itemGroup, len(order))
	for i, key := range order {
		groups[i] = byItem[key]
	}
	return groups
}

// effectiveFirst is a footprint's earliest known contact, falling back to
// its latest when FirstOccurredAt is unset (a single-contact row).
func effectiveFirst(f sovereign_db.TrailFootprint) time.Time {
	if f.FirstOccurredAt.IsZero() {
		return f.OccurredAt
	}
	return f.FirstOccurredAt
}

// chains reports whether two articles' footprint groups chain into one
// episode (stage 2): their cleaned tags overlap by at least minSharedTags,
// AND their nearest contacts fall within tagChainWindow.
func chains(a, b *itemGroup) bool {
	if sharedTagCount(a.tags, b.tags) < minSharedTags {
		return false
	}
	return gapBetween(a, b) <= tagChainWindow
}

func sharedTagCount(a, b map[string]struct{}) int {
	n := 0
	for tag := range a {
		if _, ok := b[tag]; ok {
			n++
		}
	}
	return n
}

// gapBetween is the elapsed time between two groups' nearest contacts: zero
// when their contact spans overlap, otherwise the distance between the
// earlier group's latest contact and the later group's earliest contact.
func gapBetween(a, b *itemGroup) time.Duration {
	if a.latest.Before(b.earliest) {
		return b.earliest.Sub(a.latest)
	}
	if b.latest.Before(a.earliest) {
		return a.earliest.Sub(b.latest)
	}
	return 0
}

// buildEpisode merges member groups' footprints into one episode: newest
// contact first overall, keyed by the oldest member's footprint, wear
// escalated to the deepest band present.
func buildEpisode(members []*itemGroup) Episode {
	var all []sovereign_db.TrailFootprint
	for _, g := range members {
		all = append(all, g.footprints...)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].OccurredAt.Equal(all[j].OccurredAt) {
			return all[i].OccurredAt.After(all[j].OccurredAt)
		}
		return all[i].FootprintKey > all[j].FootprintKey
	})

	oldest := all[0]
	oldestFirst := effectiveFirst(oldest)
	for _, f := range all[1:] {
		first := effectiveFirst(f)
		if first.Before(oldestFirst) || (first.Equal(oldestFirst) && f.FootprintKey < oldest.FootprintKey) {
			oldest = f
			oldestFirst = first
		}
	}

	wear := "thin"
	for _, f := range all {
		band := f.Wear
		if band == "" {
			band = "thin"
		}
		if wearRank[band] > wearRank[wear] {
			wear = band
		}
	}

	return Episode{
		EpisodeKey: "ep:" + oldest.FootprintKey,
		Wear:       wear,
		Footprints: all,
	}
}

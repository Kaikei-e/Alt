package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"fmt"
	"math"
	"testing"
)

func TestComputeEdges_SelectionNormalisationAndOrdering(t *testing.T) {
	items := []*domain.TagCloudItem{
		{TagName: "a", ArticleCount: 100},
		{TagName: "b", ArticleCount: 100},
		{TagName: "c", ArticleCount: 100},
		{TagName: "d", ArticleCount: 100},
		{TagName: "e", ArticleCount: 100},
	}

	// Tag "a" (idx 0) has 4 partners: b(80), c(60), d(40), e(20).
	// Its 3 strongest partners are b, c, d — e(20) is dropped because e has no other edges?
	// Wait: for tag "e" (idx 4), if "a" is one of e's 3 strongest partners, the union keeps (a, e).
	// To test that a 4th partner is dropped when it is in neither endpoint's top 3,
	// give "e" 3 stronger partners (b, c, d) with count 50 so (a, e) with count 20 is 4th for both "a" and "e".
	cooccurrences := []*domain.TagCooccurrence{
		{TagNameA: "a", TagNameB: "b", SharedCount: 80},
		{TagNameA: "a", TagNameB: "c", SharedCount: 60},
		{TagNameA: "a", TagNameB: "d", SharedCount: 40},
		{TagNameA: "a", TagNameB: "e", SharedCount: 20},
		{TagNameA: "b", TagNameB: "e", SharedCount: 50},
		{TagNameA: "c", TagNameB: "e", SharedCount: 50},
		{TagNameA: "d", TagNameB: "e", SharedCount: 50},
		{TagNameA: "a", TagNameB: "missing", SharedCount: 99},
	}

	edges := ComputeEdges(items, cooccurrences)
	if len(edges) != 6 {
		t.Fatalf("want 6 kept edges in union, got %d: %+v", len(edges), edges)
	}

	// Maximum kept ochiai is (a, b) = 80 / 100 = 0.8, so normalised weights are:
	// (0, 1): 80/80 = 1.0
	// (0, 2): 60/80 = 0.75
	// (1, 4): 50/80 = 0.625
	// (2, 4): 50/80 = 0.625
	// (3, 4): 50/80 = 0.625
	// (0, 3): 40/80 = 0.5
	want := []domain.TagCloudEdge{
		{SourceIndex: 0, TargetIndex: 1, Weight: 1.0},
		{SourceIndex: 0, TargetIndex: 2, Weight: 0.75},
		{SourceIndex: 1, TargetIndex: 4, Weight: 0.625},
		{SourceIndex: 2, TargetIndex: 4, Weight: 0.625},
		{SourceIndex: 3, TargetIndex: 4, Weight: 0.625},
		{SourceIndex: 0, TargetIndex: 3, Weight: 0.5},
	}

	for i, w := range want {
		got := edges[i]
		if got.SourceIndex != w.SourceIndex || got.TargetIndex != w.TargetIndex || math.Abs(got.Weight-w.Weight) > 1e-9 {
			t.Errorf("edge[%d] = %+v, want %+v", i, *got, w)
		}
		if got.SourceIndex < 0 || got.SourceIndex >= got.TargetIndex || got.TargetIndex >= len(items) {
			t.Errorf("edge[%d] indices (%d, %d) out of valid range [0, %d)", i, got.SourceIndex, got.TargetIndex, len(items))
		}
	}
}

func TestComputeEdges_CapsAt600Strongest(t *testing.T) {
	const n = 260
	items := make([]*domain.TagCloudItem, n)
	for i := range n {
		items[i] = &domain.TagCloudItem{
			TagName:      fmt.Sprintf("tag-%03d", i),
			ArticleCount: 1000,
		}
	}

	var coocs []*domain.TagCooccurrence
	for i := range n {
		for step := 1; step <= 3 && i+step < n; step++ {
			coocs = append(coocs, &domain.TagCooccurrence{
				TagNameA:    items[i].TagName,
				TagNameB:    items[i+step].TagName,
				SharedCount: 900 - i*2 - step,
			})
		}
	}

	edges := ComputeEdges(items, coocs)
	if len(edges) != 600 {
		t.Fatalf("want 600 edges after cap, got %d", len(edges))
	}
	if math.Abs(edges[0].Weight-1.0) > 1e-9 {
		t.Errorf("strongest edge weight = %f, want 1.0", edges[0].Weight)
	}
	for i, e := range edges {
		if e.SourceIndex < 0 || e.SourceIndex >= e.TargetIndex || e.TargetIndex >= len(items) {
			t.Fatalf("edge[%d] indices (%d, %d) out of range [0, %d)", i, e.SourceIndex, e.TargetIndex, len(items))
		}
		if e.Weight <= 0 || e.Weight > 1.0 {
			t.Fatalf("edge[%d] weight %f out of (0, 1]", i, e.Weight)
		}
		if i > 0 {
			prev := edges[i-1]
			if prev.Weight < e.Weight {
				t.Fatalf("edges not sorted by weight descending at %d: %+v before %+v", i, *prev, *e)
			}
			if prev.Weight == e.Weight && (prev.SourceIndex > e.SourceIndex || (prev.SourceIndex == e.SourceIndex && prev.TargetIndex >= e.TargetIndex)) {
				t.Fatalf("equal-weight edges not sorted by index at %d: %+v before %+v", i, *prev, *e)
			}
		}
	}
}

func TestComputeClusters_DeterminismAcrossInputOrders(t *testing.T) {
	baseItems := []*domain.TagCloudItem{
		{TagName: "rust", ArticleCount: 90},
		{TagName: "tokio", ArticleCount: 80},
		{TagName: "async", ArticleCount: 70},
		{TagName: "wasm", ArticleCount: 60},
		{TagName: "python", ArticleCount: 55},
		{TagName: "pytorch", ArticleCount: 50},
		{TagName: "llm", ArticleCount: 45},
		{TagName: "transformer", ArticleCount: 40},
		{TagName: "orphan", ArticleCount: 10},
	}

	baseCoocs := []*domain.TagCooccurrence{
		{TagNameA: "rust", TagNameB: "tokio", SharedCount: 40},
		{TagNameA: "rust", TagNameB: "async", SharedCount: 35},
		{TagNameA: "tokio", TagNameB: "async", SharedCount: 38},
		{TagNameA: "wasm", TagNameB: "rust", SharedCount: 30},
		{TagNameA: "python", TagNameB: "pytorch", SharedCount: 30},
		{TagNameA: "pytorch", TagNameB: "llm", SharedCount: 28},
		{TagNameA: "llm", TagNameB: "transformer", SharedCount: 25},
		{TagNameA: "python", TagNameB: "llm", SharedCount: 22},
	}

	cloneItems := func(src []*domain.TagCloudItem) []*domain.TagCloudItem {
		out := make([]*domain.TagCloudItem, len(src))
		for i, it := range src {
			cp := *it
			out[i] = &cp
		}
		return out
	}

	itemsForward := cloneItems(baseItems)
	ComputeClusters(itemsForward, baseCoocs)

	wantByTag := make(map[string]int, len(itemsForward))
	for _, it := range itemsForward {
		wantByTag[it.TagName] = it.ClusterID
	}

	if wantByTag["rust"] != 1 || wantByTag["tokio"] != 1 || wantByTag["async"] != 1 || wantByTag["wasm"] != 1 {
		t.Fatalf("expected rust cluster to be cluster 1 (largest total count), got %+v", wantByTag)
	}
	if wantByTag["python"] != 2 || wantByTag["pytorch"] != 2 || wantByTag["llm"] != 2 || wantByTag["transformer"] != 2 {
		t.Fatalf("expected python/ml cluster to be cluster 2, got %+v", wantByTag)
	}
	if wantByTag["orphan"] != 0 {
		t.Fatalf("expected orphan tag to have cluster 0, got %d", wantByTag["orphan"])
	}

	// Reverse both items and co-occurrences (and swap TagNameA/TagNameB).
	itemsReversed := make([]*domain.TagCloudItem, len(baseItems))
	for i := range baseItems {
		cp := *baseItems[len(baseItems)-1-i]
		itemsReversed[i] = &cp
	}
	coocsReversed := make([]*domain.TagCooccurrence, len(baseCoocs))
	for i := range baseCoocs {
		orig := baseCoocs[len(baseCoocs)-1-i]
		coocsReversed[i] = &domain.TagCooccurrence{
			TagNameA:    orig.TagNameB,
			TagNameB:    orig.TagNameA,
			SharedCount: orig.SharedCount,
		}
	}

	ComputeClusters(itemsReversed, coocsReversed)
	for _, it := range itemsReversed {
		if it.ClusterID != wantByTag[it.TagName] {
			t.Errorf("tag %q cluster changed with reversed input: got %d, want %d", it.TagName, it.ClusterID, wantByTag[it.TagName])
		}
	}
}

func TestComputeClusters_SizeThreshold(t *testing.T) {
	items := []*domain.TagCloudItem{
		// 3-tag community (below threshold of 4 -> must get cluster 0)
		{TagName: "small1", ArticleCount: 200},
		{TagName: "small2", ArticleCount: 200},
		{TagName: "small3", ArticleCount: 200},
		// 4-tag community (meets threshold of 4 -> gets cluster 1)
		{TagName: "valid1", ArticleCount: 50},
		{TagName: "valid2", ArticleCount: 50},
		{TagName: "valid3", ArticleCount: 50},
		{TagName: "valid4", ArticleCount: 50},
	}
	coocs := []*domain.TagCooccurrence{
		{TagNameA: "small1", TagNameB: "small2", SharedCount: 150},
		{TagNameA: "small2", TagNameB: "small3", SharedCount: 150},
		{TagNameA: "small1", TagNameB: "small3", SharedCount: 150},
		{TagNameA: "valid1", TagNameB: "valid2", SharedCount: 40},
		{TagNameA: "valid2", TagNameB: "valid3", SharedCount: 40},
		{TagNameA: "valid3", TagNameB: "valid4", SharedCount: 40},
		{TagNameA: "valid1", TagNameB: "valid4", SharedCount: 40},
	}

	ComputeClusters(items, coocs)

	for _, it := range items[:3] {
		if it.ClusterID != 0 {
			t.Errorf("tag %q in 3-tag community got cluster %d, want 0", it.TagName, it.ClusterID)
		}
	}
	for _, it := range items[3:] {
		if it.ClusterID != 1 {
			t.Errorf("tag %q in 4-tag community got cluster %d, want 1", it.TagName, it.ClusterID)
		}
	}
}

func TestComputeClusters_EightClusterCapAndOrdering(t *testing.T) {
	const numCommunities = 10
	const communitySize = 4
	items := make([]*domain.TagCloudItem, 0, numCommunities*communitySize)
	var coocs []*domain.TagCooccurrence

	// Community c has per-tag ArticleCount = (c + 1) * 10, so community 9 has the
	// largest total article count (cluster 1), down to community 2 (cluster 8),
	// and communities 1 and 0 are capped out (cluster 0).
	for c := range numCommunities {
		count := (c + 1) * 10
		names := make([]string, communitySize)
		for m := range communitySize {
			name := fmt.Sprintf("comm-%02d-tag-%d", c, m)
			names[m] = name
			items = append(items, &domain.TagCloudItem{
				TagName:      name,
				ArticleCount: count,
			})
		}
		for i := 0; i < communitySize; i++ {
			for j := i + 1; j < communitySize; j++ {
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    names[i],
					TagNameB:    names[j],
					SharedCount: count / 2,
				})
			}
		}
	}

	ComputeClusters(items, coocs)

	for c := range numCommunities {
		wantCluster := 0
		if c >= 2 {
			wantCluster = numCommunities - c
		}
		for m := range communitySize {
			idx := c*communitySize + m
			if items[idx].ClusterID != wantCluster {
				t.Errorf("community %d tag %q got ClusterID %d, want %d", c, items[idx].TagName, items[idx].ClusterID, wantCluster)
			}
		}
	}
}

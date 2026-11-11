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

	// Give "a" and "e" three stronger partners each so (a, e) is 4th for both and dropped from the union.
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

func TestComputeEdges_PartnerTiesStabilisedByFixedRank(t *testing.T) {
	// "hub" has 4 equal-Ochiai partners ("alpha", "beta", "gamma", "delta") all with ArticleCount=100,
	// and each partner has 3 stronger edges to "p1", "p2", "p3" so only hub's top-3 selection decides
	// which 3 of the 4 hub edges enter the union.
	baseItems := []*domain.TagCloudItem{
		{TagName: "hub", ArticleCount: 100},
		{TagName: "delta", ArticleCount: 100},
		{TagName: "gamma", ArticleCount: 100},
		{TagName: "beta", ArticleCount: 100},
		{TagName: "alpha", ArticleCount: 100},
		{TagName: "p1", ArticleCount: 100},
		{TagName: "p2", ArticleCount: 100},
		{TagName: "p3", ArticleCount: 100},
	}
	coocs := []*domain.TagCooccurrence{
		{TagNameA: "hub", TagNameB: "delta", SharedCount: 30},
		{TagNameA: "hub", TagNameB: "gamma", SharedCount: 30},
		{TagNameA: "hub", TagNameB: "beta", SharedCount: 30},
		{TagNameA: "hub", TagNameB: "alpha", SharedCount: 30},
	}
	for _, partner := range []string{"delta", "gamma", "beta", "alpha"} {
		for _, p := range []string{"p1", "p2", "p3"} {
			coocs = append(coocs, &domain.TagCooccurrence{
				TagNameA:    partner,
				TagNameB:    p,
				SharedCount: 80,
			})
		}
	}

	edgesPerm1 := ComputeEdges(baseItems, coocs)

	reorderedItems := []*domain.TagCloudItem{
		{TagName: "p3", ArticleCount: 100},
		{TagName: "alpha", ArticleCount: 100},
		{TagName: "beta", ArticleCount: 100},
		{TagName: "hub", ArticleCount: 100},
		{TagName: "gamma", ArticleCount: 100},
		{TagName: "delta", ArticleCount: 100},
		{TagName: "p1", ArticleCount: 100},
		{TagName: "p2", ArticleCount: 100},
	}
	edgesPerm2 := ComputeEdges(reorderedItems, coocs)

	hubPartners := func(items []*domain.TagCloudItem, edges []*domain.TagCloudEdge) map[string]bool {
		got := make(map[string]bool)
		for _, e := range edges {
			a := items[e.SourceIndex].TagName
			b := items[e.TargetIndex].TagName
			if a == "hub" {
				got[b] = true
			} else if b == "hub" {
				got[a] = true
			}
		}
		return got
	}

	p1 := hubPartners(baseItems, edgesPerm1)
	p2 := hubPartners(reorderedItems, edgesPerm2)

	// Fixed rank orders equal-count partners by TagName ascending: alpha, beta, delta (dropping gamma).
	wantHubPartners := map[string]bool{"alpha": true, "beta": true, "delta": true}
	for _, got := range []map[string]bool{p1, p2} {
		if len(got) != len(wantHubPartners) {
			t.Fatalf("hub partners = %v, want %v", got, wantHubPartners)
		}
		for k := range wantHubPartners {
			if !got[k] {
				t.Errorf("hub partners = %v, missing %q", got, k)
			}
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

	keptPairs := make(map[indexPair]bool, len(edges))
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
		keptPairs[indexPair{u: e.SourceIndex, v: e.TargetIndex}] = true
	}

	nameToIdx := make(map[string]int, n)
	for i, it := range items {
		nameToIdx[it.TagName] = i
	}
	maxKeptOchiai := 899.0 / 1000.0
	weakestKeptOchiai := edges[len(edges)-1].Weight * maxKeptOchiai
	for _, c := range coocs {
		u, v := nameToIdx[c.TagNameA], nameToIdx[c.TagNameB]
		if u > v {
			u, v = v, u
		}
		if keptPairs[indexPair{u: u, v: v}] {
			continue
		}
		droppedOchiai := float64(c.SharedCount) / 1000.0
		if droppedOchiai > weakestKeptOchiai+1e-12 {
			t.Errorf("dropped edge (%d, %d) has ochiai %f > weakest kept ochiai %f", u, v, droppedOchiai, weakestKeptOchiai)
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
		{TagName: "small1", ArticleCount: 200},
		{TagName: "small2", ArticleCount: 200},
		{TagName: "small3", ArticleCount: 200},
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

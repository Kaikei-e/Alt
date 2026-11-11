package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"cmp"
	"fmt"
	"math"
	"math/rand"
	"slices"
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

func buildHubHeavyFixture() ([]*domain.TagCloudItem, []*domain.TagCooccurrence, [][]string) {
	const (
		numTopics   = 5
		topicSize   = 8
		numHubs     = 5
		numTailTags = 3
	)

	var items []*domain.TagCloudItem
	var coocs []*domain.TagCooccurrence

	hubNames := make([]string, numHubs)
	for h := range numHubs {
		name := fmt.Sprintf("hub-%d", h)
		hubNames[h] = name
		items = append(items, &domain.TagCloudItem{
			TagName:      name,
			ArticleCount: 480 - h*30,
		})
	}

	topicGroups := make([][]string, numTopics)
	for g := range numTopics {
		baseCount := 130 - g*15
		group := make([]string, topicSize)
		for m := range topicSize {
			name := fmt.Sprintf("topic-%c-%d", 'a'+g, m)
			group[m] = name
			items = append(items, &domain.TagCloudItem{
				TagName:      name,
				ArticleCount: baseCount - m*2,
			})
		}
		topicGroups[g] = group

		for i := 0; i < topicSize; i++ {
			countI := baseCount - i*2
			for j := i + 1; j < topicSize; j++ {
				countJ := baseCount - j*2
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    group[i],
					TagNameB:    group[j],
					SharedCount: (countI + countJ) / 4,
				})
			}
		}
	}

	// Generic hubs co-occur strongly with one another and moderately with every topic tag.
	for i := 0; i < numHubs; i++ {
		for j := i + 1; j < numHubs; j++ {
			coocs = append(coocs, &domain.TagCooccurrence{
				TagNameA:    hubNames[i],
				TagNameB:    hubNames[j],
				SharedCount: 140,
			})
		}
		for g := range numTopics {
			for m := range topicSize {
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    hubNames[i],
					TagNameB:    topicGroups[g][m],
					SharedCount: 38,
				})
			}
		}
	}

	// Sparse cross-topic bridges between adjacent topics.
	for g := range numTopics {
		next := (g + 1) % numTopics
		coocs = append(coocs, &domain.TagCooccurrence{
			TagNameA:    topicGroups[g][0],
			TagNameB:    topicGroups[next][0],
			SharedCount: 5,
		})
	}

	// Small 3-tag tail component (< minCommunityTagCount) that must remain unclustered.
	tailNames := make([]string, numTailTags)
	for i := range numTailTags {
		name := fmt.Sprintf("tail-%d", i)
		tailNames[i] = name
		items = append(items, &domain.TagCloudItem{
			TagName:      name,
			ArticleCount: 25 - i*2,
		})
	}
	for i := 0; i < numTailTags; i++ {
		for j := i + 1; j < numTailTags; j++ {
			coocs = append(coocs, &domain.TagCooccurrence{
				TagNameA:    tailNames[i],
				TagNameB:    tailNames[j],
				SharedCount: 12,
			})
		}
	}

	return items, coocs, topicGroups
}

func TestComputeClusters_HubHeavyGraphRecoversBalancedTopics(t *testing.T) {
	baseItems, baseCoocs, topicGroups := buildHubHeavyFixture()

	cloneItems := func(src []*domain.TagCloudItem) []*domain.TagCloudItem {
		out := make([]*domain.TagCloudItem, len(src))
		for i, it := range src {
			cp := *it
			out[i] = &cp
		}
		return out
	}

	items := cloneItems(baseItems)
	ComputeClusters(items, baseCoocs)

	byName := make(map[string]int, len(items))
	clusterSizes := make(map[int]int)
	clusteredCount := 0
	for _, it := range items {
		if it.ClusterID < 0 || it.ClusterID > maxCommunities {
			t.Fatalf("tag %q got out-of-range ClusterID %d", it.TagName, it.ClusterID)
		}
		byName[it.TagName] = it.ClusterID
		clusterSizes[it.ClusterID]++
		if it.ClusterID > 0 {
			clusteredCount++
		}
	}

	t.Logf("cluster distribution: sizes=%v clustered=%d/%d (%.1f%%)",
		clusterSizes, clusteredCount, len(items), 100*float64(clusteredCount)/float64(len(items)))

	// (a) Each genuine topic group is recovered as its own distinct non-zero cluster.
	seenGroupCluster := make(map[int]int, len(topicGroups))
	for g, group := range topicGroups {
		cid := byName[group[0]]
		if cid == 0 {
			t.Errorf("topic group %d (%q) left unclustered (ClusterID=0)", g, group[0])
			continue
		}
		for _, tagName := range group[1:] {
			if byName[tagName] != cid {
				t.Errorf("topic group %d split across clusters: %q=%d, %q=%d",
					g, group[0], cid, tagName, byName[tagName])
			}
		}
		if prevGroup, dup := seenGroupCluster[cid]; dup {
			t.Errorf("topic groups %d and %d collapsed into the same ClusterID %d", prevGroup, g, cid)
		}
		seenGroupCluster[cid] = g
	}

	// (b) No single cluster holds more than 40% of the clustered tags.
	if clusteredCount == 0 {
		t.Fatal("expected non-zero clustered tags")
	}
	for cid, sz := range clusterSizes {
		if cid == 0 {
			continue
		}
		share := float64(sz) / float64(clusteredCount)
		if share > 0.40 {
			t.Errorf("cluster %d holds %d/%d (%.1f%%) of clustered tags, exceeding 40%% cap",
				cid, sz, clusteredCount, share*100)
		}
	}

	// (c) Identical result across repeated runs and input permutations.
	for run := range 5 {
		repeatItems := cloneItems(baseItems)
		ComputeClusters(repeatItems, baseCoocs)
		for _, it := range repeatItems {
			if it.ClusterID != byName[it.TagName] {
				t.Fatalf("repeat run %d changed ClusterID of %q: got %d, want %d",
					run, it.TagName, it.ClusterID, byName[it.TagName])
			}
		}
	}

	for seed := int64(1); seed <= 5; seed++ {
		rng := rand.New(rand.NewSource(seed)) //#nosec G404 -- deterministic test permutation seed
		permItems := cloneItems(baseItems)
		rng.Shuffle(len(permItems), func(i, j int) {
			permItems[i], permItems[j] = permItems[j], permItems[i]
		})
		permCoocs := make([]*domain.TagCooccurrence, len(baseCoocs))
		for i, c := range baseCoocs {
			cp := *c
			if rng.Intn(2) == 1 {
				cp.TagNameA, cp.TagNameB = cp.TagNameB, cp.TagNameA
			}
			permCoocs[i] = &cp
		}
		rng.Shuffle(len(permCoocs), func(i, j int) {
			permCoocs[i], permCoocs[j] = permCoocs[j], permCoocs[i]
		})

		ComputeClusters(permItems, permCoocs)
		for _, it := range permItems {
			if it.ClusterID != byName[it.TagName] {
				t.Fatalf("permutation seed %d changed ClusterID of %q: got %d, want %d",
					seed, it.TagName, it.ClusterID, byName[it.TagName])
			}
		}
	}
}

func buildBenchmarkHubFixture() ([]*domain.TagCloudItem, []*domain.TagCooccurrence) {
	const (
		numHubs   = 20
		numTopics = 7
		topicSize = 35
		numTail   = 35
	)
	items := make([]*domain.TagCloudItem, 0, 300)
	var coocs []*domain.TagCooccurrence

	hubs := make([]string, numHubs)
	for h := range numHubs {
		name := fmt.Sprintf("hub-%02d", h)
		hubs[h] = name
		items = append(items, &domain.TagCloudItem{TagName: name, ArticleCount: 600 - h*15})
	}
	for i := 0; i < numHubs; i++ {
		for j := i + 1; j < numHubs; j++ {
			coocs = append(coocs, &domain.TagCooccurrence{TagNameA: hubs[i], TagNameB: hubs[j], SharedCount: 90})
		}
	}

	for g := range numTopics {
		group := make([]string, topicSize)
		for m := range topicSize {
			name := fmt.Sprintf("topic-%d-%02d", g, m)
			group[m] = name
			items = append(items, &domain.TagCloudItem{TagName: name, ArticleCount: 150 - g*8 - m})
		}
		for i := 0; i < topicSize; i++ {
			for step := 1; step <= 5 && i+step < topicSize; step++ {
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    group[i],
					TagNameB:    group[i+step],
					SharedCount: 42 - step*3,
				})
			}
			for h := (i + g) % 5; h < numHubs; h += 5 {
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    hubs[h],
					TagNameB:    group[i],
					SharedCount: 25,
				})
			}
		}
	}

	for i := range numTail {
		items = append(items, &domain.TagCloudItem{TagName: fmt.Sprintf("tail-%02d", i), ArticleCount: 15})
	}

	return items, coocs
}

func TestRunLeiden_NodeLevelLocalOptimality(t *testing.T) {
	items, coocs := buildBenchmarkHubFixture()
	_, adj := buildAssociationGraph(items, coocs)
	n := len(items)

	strength := make([]float64, n)
	seedRank := make([]int, n)
	twoM := 0.0
	for r := range n {
		seedRank[r] = r
		for _, nb := range adj[r] {
			strength[r] += nb.weight
		}
		twoM += strength[r]
	}

	g := &leidenGraph{
		n:        n,
		adj:      adj,
		strength: strength,
		seedRank: seedRank,
		twoM:     twoM,
	}
	labels := runLeiden(g, leidenResolution)

	sigmaTot := make(map[int]float64)
	commSize := make(map[int]int)
	for u, c := range labels {
		sigmaTot[c] += strength[u]
		commSize[c]++
	}

	scale := leidenResolution / twoM
	moveEps := qualityEps * twoM
	improvingNodes := 0
	maxGain := 0.0
	for u := range n {
		oldComm := labels[u]
		ku := strength[u]
		commWeights := make(map[int]float64)
		for _, nb := range adj[u] {
			commWeights[labels[nb.partner]] += nb.weight
		}
		stayScore := commWeights[oldComm] - scale*ku*(sigmaTot[oldComm]-ku)

		bestGain := 0.0
		if commSize[oldComm] > 1 && -stayScore > bestGain {
			bestGain = -stayScore
		}
		for c, w := range commWeights {
			if c == oldComm {
				continue
			}
			gain := (w - scale*ku*sigmaTot[c]) - stayScore
			if gain > bestGain {
				bestGain = gain
			}
		}
		if bestGain > moveEps {
			improvingNodes++
			if bestGain > maxGain {
				maxGain = bestGain
			}
		}
	}

	if improvingNodes > 0 {
		t.Fatalf("partition is not locally optimal at node level: %d nodes have strictly positive move gain (max delta = %.6g)",
			improvingNodes, maxGain)
	}
}

func BenchmarkComputeClusters_300Nodes_HubHeavy(b *testing.B) {
	items, coocs := buildBenchmarkHubFixture()

	b.ResetTimer()
	for b.Loop() {
		ComputeClusters(items, coocs)
	}
}

func buildLinkedHubs2000CutFixture() ([]*domain.TagCloudItem, []*domain.TagCooccurrence, [][]string) {
	const (
		numHubs    = 12
		numTopics  = 6
		topicSize  = 20
		numTail    = 48
		maxDBPairs = 2000
	)

	items := make([]*domain.TagCloudItem, 0, numHubs+numTopics*topicSize+numTail)
	var coocs []*domain.TagCooccurrence

	hubs := make([]string, numHubs)
	hubCounts := make([]int, numHubs)
	for h := range numHubs {
		name := fmt.Sprintf("hub-%02d", h)
		count := 600 - h*15
		hubs[h] = name
		hubCounts[h] = count
		items = append(items, &domain.TagCloudItem{TagName: name, ArticleCount: count})
	}
	for i := 0; i < numHubs; i++ {
		for j := i + 1; j < numHubs; j++ {
			coocs = append(coocs, &domain.TagCooccurrence{
				TagNameA:    hubs[i],
				TagNameB:    hubs[j],
				SharedCount: 90,
			})
		}
	}

	topicGroups := make([][]string, numTopics)
	for g := range numTopics {
		group := make([]string, topicSize)
		counts := make([]int, topicSize)
		for m := range topicSize {
			name := fmt.Sprintf("topic-%d-%02d", g, m)
			count := 160 - g*16 - m
			group[m] = name
			counts[m] = count
			items = append(items, &domain.TagCloudItem{TagName: name, ArticleCount: count})
		}
		topicGroups[g] = group

		for i := 0; i < topicSize; i++ {
			for j := i + 1; j < topicSize; j++ {
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    group[i],
					TagNameB:    group[j],
					SharedCount: (counts[i]+counts[j])/6 + 8,
				})
			}
			for h := range numHubs {
				// Co-occurrence count scales with both hub and topic-tag frequency;
				// ranking the 2,000-pair SQL cutoff by Ochiai association instead of
				// raw shared_count prevents large-count hub pairs from crowding out
				// higher-association topic pairs.
				shared := (hubCounts[h]*counts[i])/2200 + 6
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    hubs[h],
					TagNameB:    group[i],
					SharedCount: shared,
				})
			}
		}
	}

	for i := range numTail {
		items = append(items, &domain.TagCloudItem{TagName: fmt.Sprintf("tail-%02d", i), ArticleCount: 15})
	}

	countByName := make(map[string]int, len(items))
	for _, it := range items {
		countByName[it.TagName] = it.ArticleCount
	}
	ochiaiOf := func(c *domain.TagCooccurrence) float64 {
		ca := countByName[c.TagNameA]
		cb := countByName[c.TagNameB]
		return float64(c.SharedCount) / math.Sqrt(float64(ca)*float64(cb))
	}

	slices.SortFunc(coocs, func(a, b *domain.TagCooccurrence) int {
		if c := cmp.Compare(ochiaiOf(b), ochiaiOf(a)); c != 0 {
			return c
		}
		if c := cmp.Compare(b.SharedCount, a.SharedCount); c != 0 {
			return c
		}
		if c := cmp.Compare(a.TagNameA, b.TagNameA); c != 0 {
			return c
		}
		return cmp.Compare(a.TagNameB, b.TagNameB)
	})
	if len(coocs) > maxDBPairs {
		coocs = coocs[:maxDBPairs]
	}

	return items, coocs, topicGroups
}

func TestComputeClusters_LinkedHubsStayBalanced(t *testing.T) {
	baseItems, baseCoocs, topicGroups := buildLinkedHubs2000CutFixture()

	cloneItems := func(src []*domain.TagCloudItem) []*domain.TagCloudItem {
		out := make([]*domain.TagCloudItem, len(src))
		for i, it := range src {
			cp := *it
			out[i] = &cp
		}
		return out
	}

	items := cloneItems(baseItems)
	ComputeClusters(items, baseCoocs)

	byName := make(map[string]int, len(items))
	clusterSizes := make(map[int]int)
	clusteredCount := 0
	for _, it := range items {
		if it.ClusterID < 0 || it.ClusterID > maxCommunities {
			t.Fatalf("tag %q got out-of-range ClusterID %d", it.TagName, it.ClusterID)
		}
		byName[it.TagName] = it.ClusterID
		clusterSizes[it.ClusterID]++
		if it.ClusterID > 0 {
			clusteredCount++
		}
	}

	t.Logf("12-hub 2000-cut distribution: sizes=%v clustered=%d/%d (%.1f%%)",
		clusterSizes, clusteredCount, len(items), 100*float64(clusteredCount)/float64(len(items)))

	if clusteredCount == 0 {
		t.Fatal("expected non-zero clustered tags")
	}
	for cid, sz := range clusterSizes {
		if cid == 0 {
			continue
		}
		if share := float64(sz) / float64(clusteredCount); share > 0.40 {
			t.Errorf("cluster %d holds %d/%d (%.1f%%) > 40%% of clustered tags", cid, sz, clusteredCount, share*100)
		}
	}

	seenTopicClusters := make(map[int]int, len(topicGroups))
	intactTopicCount := 0
	for g, group := range topicGroups {
		cid := byName[group[0]]
		if cid == 0 {
			continue
		}
		intact := true
		for _, name := range group[1:] {
			if byName[name] != cid {
				intact = false
				break
			}
		}
		if !intact {
			continue
		}
		intactTopicCount++
		if prev, dup := seenTopicClusters[cid]; dup {
			t.Errorf("topic groups %d and %d share ClusterID %d", prev, g, cid)
		}
		seenTopicClusters[cid] = g
	}
	if len(seenTopicClusters) < 5 {
		t.Errorf("got %d distinct non-zero topic clusters (from %d intact topic groups), want at least 5 of %d",
			len(seenTopicClusters), intactTopicCount, len(topicGroups))
	}

	for seed := int64(1); seed <= 5; seed++ {
		rng := rand.New(rand.NewSource(seed)) //#nosec G404 -- deterministic test permutation seed
		permItems := cloneItems(baseItems)
		rng.Shuffle(len(permItems), func(i, j int) {
			permItems[i], permItems[j] = permItems[j], permItems[i]
		})
		permCoocs := make([]*domain.TagCooccurrence, len(baseCoocs))
		for i, c := range baseCoocs {
			cp := *c
			if rng.Intn(2) == 1 {
				cp.TagNameA, cp.TagNameB = cp.TagNameB, cp.TagNameA
			}
			permCoocs[i] = &cp
		}
		rng.Shuffle(len(permCoocs), func(i, j int) {
			permCoocs[i], permCoocs[j] = permCoocs[j], permCoocs[i]
		})

		ComputeClusters(permItems, permCoocs)
		for _, it := range permItems {
			if it.ClusterID != byName[it.TagName] {
				t.Fatalf("permutation seed %d changed ClusterID of %q: got %d, want %d",
					seed, it.TagName, it.ClusterID, byName[it.TagName])
			}
		}
	}
}

func TestComputeClusters_SingleCommunityGraph(t *testing.T) {
	// Under Reichardt-Bornholdt configuration modularity with γ = 1.15 (> 1), a
	// standalone uniform k-clique stays singletons iff γ >= k/(k-1): lone 4-cycles
	// and k >= 8 cliques split below minCommunityTagCount (ClusterID 0), while a
	// lone 6-clique (6/5 = 1.20 > 1.15) remains a single community (ClusterID 1).
	t.Run("lone 4-cycle splits into pairs and receives cluster 0", func(t *testing.T) {
		items := []*domain.TagCloudItem{
			{TagName: "a", ArticleCount: 100},
			{TagName: "b", ArticleCount: 90},
			{TagName: "c", ArticleCount: 80},
			{TagName: "d", ArticleCount: 70},
		}
		coocs := []*domain.TagCooccurrence{
			{TagNameA: "a", TagNameB: "b", SharedCount: 40},
			{TagNameA: "b", TagNameB: "c", SharedCount: 35},
			{TagNameA: "c", TagNameB: "d", SharedCount: 30},
			{TagNameA: "a", TagNameB: "d", SharedCount: 25},
		}

		ComputeClusters(items, coocs)
		for _, it := range items {
			if it.ClusterID != 0 {
				t.Errorf("tag %q in lone 4-cycle got ClusterID %d, want 0", it.TagName, it.ClusterID)
			}
		}
	})

	t.Run("lone 6-clique stays together under gamma 1.15 and receives cluster 1", func(t *testing.T) {
		items := make([]*domain.TagCloudItem, 6)
		for i := range items {
			items[i] = &domain.TagCloudItem{
				TagName:      fmt.Sprintf("k-%d", i),
				ArticleCount: 100,
			}
		}
		var coocs []*domain.TagCooccurrence
		for i := 0; i < len(items); i++ {
			for j := i + 1; j < len(items); j++ {
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    items[i].TagName,
					TagNameB:    items[j].TagName,
					SharedCount: 50,
				})
			}
		}

		ComputeClusters(items, coocs)
		for _, it := range items {
			if it.ClusterID != 1 {
				t.Errorf("tag %q in lone 6-clique got ClusterID %d, want 1", it.TagName, it.ClusterID)
			}
		}
	})

	t.Run("lone 8-clique stays singletons and receives cluster 0", func(t *testing.T) {
		items := make([]*domain.TagCloudItem, 8)
		for i := range items {
			items[i] = &domain.TagCloudItem{
				TagName:      fmt.Sprintf("k8-%d", i),
				ArticleCount: 100,
			}
		}
		var coocs []*domain.TagCooccurrence
		for i := 0; i < len(items); i++ {
			for j := i + 1; j < len(items); j++ {
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    items[i].TagName,
					TagNameB:    items[j].TagName,
					SharedCount: 50,
				})
			}
		}

		ComputeClusters(items, coocs)
		for _, it := range items {
			if it.ClusterID != 0 {
				t.Errorf("tag %q in lone 8-clique got ClusterID %d, want 0", it.TagName, it.ClusterID)
			}
		}
	})
}

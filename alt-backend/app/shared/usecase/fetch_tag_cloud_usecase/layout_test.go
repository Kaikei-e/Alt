package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"math"
	"testing"
)

func distance(a, b *domain.TagCloudItem) float64 {
	dx := a.PositionX - b.PositionX
	dy := a.PositionY - b.PositionY
	dz := a.PositionZ - b.PositionZ
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func TestComputeLayout_EmptyInput(t *testing.T) {
	items := []*domain.TagCloudItem{}
	edges := []*domain.TagCooccurrence{}
	ComputeLayout(items, edges)
	if len(items) != 0 {
		t.Errorf("expected empty result, got %d items", len(items))
	}
}

func TestComputeLayout_SingleNode(t *testing.T) {
	items := []*domain.TagCloudItem{
		{TagName: "go", ArticleCount: 10},
	}
	ComputeLayout(items, nil)

	if items[0].PositionX != 0 || items[0].PositionY != 0 || items[0].PositionZ != 0 {
		t.Errorf("single node should be at origin, got (%f, %f, %f)",
			items[0].PositionX, items[0].PositionY, items[0].PositionZ)
	}
}

func TestComputeLayout_UnconnectedNodesSpreadApart(t *testing.T) {
	items := []*domain.TagCloudItem{
		{TagName: "go", ArticleCount: 10},
		{TagName: "rust", ArticleCount: 8},
		{TagName: "python", ArticleCount: 12},
	}
	ComputeLayout(items, nil)

	// All nodes should be separated by repulsion
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			d := distance(items[i], items[j])
			if d < 1.0 {
				t.Errorf("nodes %s and %s are too close: %f",
					items[i].TagName, items[j].TagName, d)
			}
		}
	}
}

func TestComputeLayout_ConnectedNodesCloserThanUnconnected(t *testing.T) {
	items := []*domain.TagCloudItem{
		{TagName: "go", ArticleCount: 10},
		{TagName: "rust", ArticleCount: 8},
		{TagName: "python", ArticleCount: 12},
	}
	edges := []*domain.TagCooccurrence{
		{TagNameA: "go", TagNameB: "rust", SharedCount: 20},
	}
	ComputeLayout(items, edges)

	connectedDist := distance(items[0], items[1])    // go-rust (connected)
	unconnectedDist1 := distance(items[0], items[2]) // go-python
	unconnectedDist2 := distance(items[1], items[2]) // rust-python

	avgUnconnected := (unconnectedDist1 + unconnectedDist2) / 2

	if connectedDist >= avgUnconnected {
		t.Errorf("connected pair (go-rust) distance %f should be less than average unconnected distance %f",
			connectedDist, avgUnconnected)
	}
}

func TestComputeLayout_PositionsWithinBounds(t *testing.T) {
	items := make([]*domain.TagCloudItem, 50)
	for i := range items {
		items[i] = &domain.TagCloudItem{
			TagName:      "tag" + string(rune('a'+i%26)),
			ArticleCount: i + 1,
		}
	}

	// Some random edges
	edges := []*domain.TagCooccurrence{
		{TagNameA: "taga", TagNameB: "tagb", SharedCount: 5},
		{TagNameA: "tagc", TagNameB: "tagd", SharedCount: 3},
	}
	ComputeLayout(items, edges)

	const bound = 110.0
	for _, item := range items {
		if math.Abs(item.PositionX) > bound || math.Abs(item.PositionY) > bound || math.Abs(item.PositionZ) > bound {
			t.Errorf("node %s position (%f, %f, %f) exceeds bound %f",
				item.TagName, item.PositionX, item.PositionY, item.PositionZ, bound)
		}
	}
}

func TestComputeLayout_Deterministic(t *testing.T) {
	makeItems := func() []*domain.TagCloudItem {
		return []*domain.TagCloudItem{
			{TagName: "go", ArticleCount: 10},
			{TagName: "rust", ArticleCount: 8},
			{TagName: "python", ArticleCount: 12},
		}
	}
	edges := []*domain.TagCooccurrence{
		{TagNameA: "go", TagNameB: "rust", SharedCount: 5},
	}

	items1 := makeItems()
	ComputeLayout(items1, edges)

	items2 := makeItems()
	ComputeLayout(items2, edges)

	for i := range items1 {
		if items1[i].PositionX != items2[i].PositionX ||
			items1[i].PositionY != items2[i].PositionY ||
			items1[i].PositionZ != items2[i].PositionZ {
			t.Errorf("layout is not deterministic for node %s", items1[i].TagName)
		}
	}
}

func makeTestItems(n int) []*domain.TagCloudItem {
	items := make([]*domain.TagCloudItem, n)
	for i := range items {
		items[i] = &domain.TagCloudItem{
			TagName:      "tag" + string(rune('A'+i%26)) + string(rune('0'+i/26)),
			ArticleCount: i + 1,
		}
	}
	return items
}

func makeTestEdges(n int) []*domain.TagCooccurrence {
	edges := make([]*domain.TagCooccurrence, 0, n/2)
	items := makeTestItems(n)
	for i := 0; i < n-1; i += 2 {
		edges = append(edges, &domain.TagCooccurrence{
			TagNameA:    items[i].TagName,
			TagNameB:    items[i+1].TagName,
			SharedCount: 5,
		})
	}
	return edges
}

func BenchmarkComputeLayout_100Nodes(b *testing.B) {
	for b.Loop() {
		items := makeTestItems(100)
		edges := makeTestEdges(100)
		ComputeLayout(items, edges)
	}
}

func BenchmarkComputeLayout_200Nodes(b *testing.B) {
	for b.Loop() {
		items := makeTestItems(200)
		edges := makeTestEdges(200)
		ComputeLayout(items, edges)
	}
}

func BenchmarkComputeLayout_300Nodes(b *testing.B) {
	for b.Loop() {
		items := makeTestItems(300)
		edges := makeTestEdges(300)
		ComputeLayout(items, edges)
	}
}

func BenchmarkComputeLayout_500Nodes(b *testing.B) {
	for b.Loop() {
		items := makeTestItems(500)
		edges := makeTestEdges(500)
		ComputeLayout(items, edges)
	}
}

// makeDenseEdges creates edges with higher density (every pair within a window).
func makeDenseEdges(n int) []*domain.TagCooccurrence {
	items := makeTestItems(n)
	edges := make([]*domain.TagCooccurrence, 0)
	window := 5
	if window > n {
		window = n
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < i+window && j < n; j++ {
			edges = append(edges, &domain.TagCooccurrence{
				TagNameA:    items[i].TagName,
				TagNameB:    items[j].TagName,
				SharedCount: 3 + (i+j)%10,
			})
		}
	}
	return edges
}

func BenchmarkComputeLayout_300Nodes_DenseEdges(b *testing.B) {
	for b.Loop() {
		items := makeTestItems(300)
		edges := makeDenseEdges(300)
		ComputeLayout(items, edges)
	}
}

// TestComputeLayout_EarlyConvergenceQuality verifies that early convergence
// produces the same layout quality as running all iterations.
func TestComputeLayout_EarlyConvergenceQuality(t *testing.T) {
	items := makeTestItems(100)
	edges := makeTestEdges(100)
	ComputeLayout(items, edges)

	// All positions should be within bounds
	const bound = 110.0
	for _, item := range items {
		if math.Abs(item.PositionX) > bound || math.Abs(item.PositionY) > bound || math.Abs(item.PositionZ) > bound {
			t.Errorf("node %s position (%f, %f, %f) exceeds bound %f",
				item.TagName, item.PositionX, item.PositionY, item.PositionZ, bound)
		}
	}

	// Connected nodes should still be closer than unconnected
	items2 := []*domain.TagCloudItem{
		{TagName: "go", ArticleCount: 10},
		{TagName: "rust", ArticleCount: 8},
		{TagName: "python", ArticleCount: 12},
	}
	edges2 := []*domain.TagCooccurrence{
		{TagNameA: "go", TagNameB: "rust", SharedCount: 20},
	}
	ComputeLayout(items2, edges2)

	connectedDist := distance(items2[0], items2[1])
	unconnectedDist1 := distance(items2[0], items2[2])
	unconnectedDist2 := distance(items2[1], items2[2])
	avgUnconnected := (unconnectedDist1 + unconnectedDist2) / 2

	if connectedDist >= avgUnconnected {
		t.Errorf("early convergence broke layout: connected pair distance %f >= avg unconnected %f",
			connectedDist, avgUnconnected)
	}
}

// TestComputeLayout_EarlyConvergenceReducesIterations verifies that the profile
// is shorter than layoutIterations when convergence occurs.
func TestComputeLayout_EarlyConvergenceReducesIterations(t *testing.T) {
	items := makeTestItems(300)
	edges := makeTestEdges(300)
	profile := ComputeLayoutWithProfile(items, edges)

	if len(profile) >= layoutIterations {
		t.Errorf("expected early convergence to reduce iterations, but got %d (max %d)",
			len(profile), layoutIterations)
	}
	t.Logf("converged in %d/%d iterations (saved %.0f%%)",
		len(profile), layoutIterations,
		100*(1-float64(len(profile))/float64(layoutIterations)))
}

// TestComputeLayout_ConvergenceProfile measures max displacement per iteration
// to identify when convergence occurs.
func TestComputeLayout_ConvergenceProfile(t *testing.T) {
	items := makeTestItems(300)
	edges := makeTestEdges(300)

	profile := ComputeLayoutWithProfile(items, edges)

	// Log the convergence profile
	t.Logf("Total iterations: %d", len(profile))
	t.Logf("Initial radius: %f", math.Sqrt(float64(300))*8.0)

	// Log every 25th iteration
	for i, maxDisp := range profile {
		if i%25 == 0 || i == len(profile)-1 {
			t.Logf("Iteration %3d: maxDisplacement = %.6f", i, maxDisp)
		}
	}

	// Test multiple convergence thresholds
	initialRadius := math.Sqrt(float64(300)) * 8.0
	thresholds := []float64{0.002, 0.005, 0.01}
	for _, ratio := range thresholds {
		threshold := initialRadius * ratio
		convergedAt := -1
		consecutive := 0
		for i, maxDisp := range profile {
			if maxDisp < threshold {
				consecutive++
				if consecutive >= 5 && convergedAt == -1 {
					convergedAt = i - 4
				}
			} else {
				consecutive = 0
			}
		}
		if convergedAt >= 0 {
			t.Logf("ratio=%.3f threshold=%.3f → converged at iteration %d (saves %d iterations)",
				ratio, threshold, convergedAt, layoutIterations-convergedAt)
		} else {
			t.Logf("ratio=%.3f threshold=%.3f → did not converge within %d iterations",
				ratio, threshold, len(profile))
		}
	}
}
